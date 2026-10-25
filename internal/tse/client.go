// Package tse fetches the official TSE result files.
// One URL is one file. The TSE rewrites that file in place.
package tse

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lewtec/lewkit/x/driver/httpclient"
	"github.com/lewtec/lewkit/x/errs"
)

const (
	defaultBase = "https://resultados.tse.jus.br/oficial"
	userAgent   = "SlopApura/0.2 (projeto local, cache >=270s, sem chave, sem cadastro)"

	ttlData   = 270 * time.Second
	ttl404    = 30 * time.Second
	ttlConfig = 300 * time.Second
	maxBody   = 32 << 20
)

// Client caches TSE JSON.
// Now, when set, replaces time.Now for the cache clock.
// Pass a time from time.Now so Sub keeps the monotonic clock.
type Client struct {
	HTTP *http.Client
	Now  func() time.Time
	// Base replaces the official host in tests.
	Base string

	mu    sync.Mutex
	cache map[string]slot
}

type slot struct {
	at      time.Time
	val     any
	present bool
	lm      string
	etag    string
}

type lookup struct {
	slot  slot
	fresh bool
	ok    bool
}

// New uses the process HTTP client.
// Blank-import github.com/lewtec/lewkit/x/driver/httpclient/native first.
func New(ctx context.Context) (*Client, error) {
	hc, err := httpclient.Client(ctx)
	if err != nil {
		return nil, err
	}
	return &Client{HTTP: hc}, nil
}

func (c *Client) now() time.Time {
	if c != nil && c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Client) base() string {
	if c != nil && c.Base != "" {
		return strings.TrimRight(c.Base, "/")
	}
	return defaultBase
}

func (c *Client) httpClient(ctx context.Context) (*http.Client, error) {
	if c != nil && c.HTTP != nil {
		return c.HTTP, nil
	}
	return httpclient.Client(ctx)
}

// ElectionCode selects the 2026 file code.
// President is the federal election. Every other cargo is estadual.
func ElectionCode(cargo, turno int) (string, error) {
	if turno != 1 && turno != 2 {
		return "", errs.Wrap(errs.ErrInvalid, "turno inválido")
	}
	switch {
	case cargo == 1 && turno == 2:
		return "6258", nil
	case cargo == 1:
		return "6257", nil
	case turno == 2:
		return "6260", nil
	default:
		return "6259", nil
	}
}

// ResultURL is the immutable aggregate file for one UF and cargo.
func (c *Client) ResultURL(uf string, cargo, turno int) (string, error) {
	code, err := ElectionCode(cargo, turno)
	if err != nil {
		return "", err
	}
	n, err := strconv.Atoi(code)
	if err != nil {
		return "", err
	}
	uf = strings.ToLower(strings.TrimSpace(uf))
	if uf == "" {
		uf = "br"
	}
	return fmt.Sprintf("%s/ele2026/%s/dados/%s/%s-c%04d-e%06d-u.json", c.base(), code, uf, uf, cargo, n), nil
}

// ConfigURL is the canonical election-code document.
func (c *Client) ConfigURL() string {
	return c.base() + "/comum/config/ele-c.json"
}

// Resultado returns the raw JSON object, or nil when the file is not published yet.
func (c *Client) Resultado(ctx context.Context, uf string, cargo, turno int) (any, error) {
	rawURL, err := c.ResultURL(uf, cargo, turno)
	if err != nil {
		return nil, err
	}
	return c.fetch(ctx, rawURL, ttlData)
}

// Config returns the canonical TSE config document.
// A not-published file is a nil body and a nil error.
func (c *Client) Config(ctx context.Context) (any, error) {
	return c.fetch(ctx, c.ConfigURL(), ttlConfig)
}

func (c *Client) fetch(ctx context.Context, rawURL string, ttl time.Duration) (any, error) {
	now := c.now()
	prev := c.lookup(rawURL, now, ttl)
	if prev.fresh {
		if !prev.slot.present {
			return nil, nil
		}
		return prev.slot.val, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	if prev.ok {
		if prev.slot.lm != "" {
			req.Header.Set("If-Modified-Since", prev.slot.lm)
		}
		if prev.slot.etag != "" {
			req.Header.Set("If-None-Match", prev.slot.etag)
		}
	}

	hc, err := c.httpClient(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotModified:
		if !prev.ok {
			return nil, errs.Wrap(errs.ErrNotFound, "304 without cached body")
		}
		c.store(rawURL, slot{
			at:      now,
			val:     prev.slot.val,
			present: prev.slot.present,
			lm:      prev.slot.lm,
			etag:    prev.slot.etag,
		})
		if !prev.slot.present {
			return nil, nil
		}
		return prev.slot.val, nil
	case http.StatusNotFound:
		// Expire in 30s, not 270s. The offset is fixed to the data TTL,
		// including when the caller asked for the longer config TTL.
		c.store(rawURL, slot{at: now.Add(-(ttlData - ttl404))})
		return nil, nil
	case http.StatusOK:
		body, err := readLimited(resp.Body)
		if err != nil {
			return nil, err
		}
		val, err := decodeJSON(body)
		if err != nil {
			return nil, err
		}
		c.store(rawURL, slot{
			at:      now,
			val:     val,
			present: true,
			lm:      resp.Header.Get("Last-Modified"),
			etag:    resp.Header.Get("ETag"),
		})
		return val, nil
	default:
		return nil, fmt.Errorf("tse http %d", resp.StatusCode)
	}
}

func readLimited(r io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxBody+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBody {
		return nil, fmt.Errorf("tse body exceeds %d bytes", maxBody)
	}
	return body, nil
}

func decodeJSON(body []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var val any
	if err := dec.Decode(&val); err != nil {
		return nil, err
	}
	return val, nil
}

func (c *Client) lookup(rawURL string, now time.Time, ttl time.Duration) lookup {
	c.mu.Lock()
	defer c.mu.Unlock()
	hit, ok := c.cache[rawURL]
	if !ok {
		return lookup{}
	}
	return lookup{slot: hit, ok: true, fresh: now.Sub(hit.at) < ttl}
}

func (c *Client) store(rawURL string, item slot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cache == nil {
		c.cache = make(map[string]slot)
	}
	c.cache[rawURL] = item
}
