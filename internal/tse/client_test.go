package tse

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestResultURL(t *testing.T) {
	c := &Client{}
	cases := []struct {
		uf        string
		cargo     int
		turno     int
		want      string
		wantError bool
	}{
		{"BR", 1, 2, "https://resultados.tse.jus.br/oficial/ele2026/6258/dados/br/br-c0001-e006258-u.json", false},
		{"br", 1, 1, "https://resultados.tse.jus.br/oficial/ele2026/6257/dados/br/br-c0001-e006257-u.json", false},
		{"sp", 6, 1, "https://resultados.tse.jus.br/oficial/ele2026/6259/dados/sp/sp-c0006-e006259-u.json", false},
		{"mg", 3, 2, "https://resultados.tse.jus.br/oficial/ele2026/6260/dados/mg/mg-c0003-e006260-u.json", false},
		{"br", 1, 3, "", true},
	}
	for _, tc := range cases {
		got, err := c.ResultURL(tc.uf, tc.cargo, tc.turno)
		if tc.wantError {
			if err == nil {
				t.Fatalf("ResultURL(%s, %d, %d) ok, want error", tc.uf, tc.cargo, tc.turno)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Fatalf("ResultURL(%s, %d, %d)=%s, want %s", tc.uf, tc.cargo, tc.turno, got, tc.want)
		}
	}
	if c.ConfigURL() != "https://resultados.tse.jus.br/oficial/comum/config/ele-c.json" {
		t.Fatal(c.ConfigURL())
	}
}

func TestNotFoundCache(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	at := time.Now()
	c := &Client{HTTP: srv.Client(), Base: srv.URL, Now: func() time.Time { return at }}
	ctx := context.Background()

	raw, err := c.Resultado(ctx, "br", 1, 2)
	if err != nil || raw != nil {
		t.Fatalf("raw=%v err=%v", raw, err)
	}
	if _, err = c.Resultado(ctx, "br", 1, 2); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits %d, want 1 inside 30s", hits.Load())
	}
	at = at.Add(31 * time.Second)
	if _, err = c.Resultado(ctx, "br", 1, 2); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 2 {
		t.Fatalf("hits %d, want 2 after 31s", hits.Load())
	}
}

func TestNotModified(t *testing.T) {
	var hits atomic.Int32
	const etag = `"v1"`
	const lm = "Mon, 02 Oct 2026 12:00:00 GMT"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("User-Agent") == "" || r.Header.Get("Accept") != "application/json" {
			t.Errorf("headers ua=%q accept=%q", r.Header.Get("User-Agent"), r.Header.Get("Accept"))
		}
		if r.Header.Get("If-None-Match") == etag {
			if r.Header.Get("If-Modified-Since") != lm {
				t.Errorf("If-Modified-Since %q", r.Header.Get("If-Modified-Since"))
			}
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		w.Header().Set("Last-Modified", lm)
		_, _ = w.Write([]byte(`{"cand":[{"nm":"A","vap":"10"}]}`))
	}))
	t.Cleanup(srv.Close)

	at := time.Now()
	c := &Client{HTTP: srv.Client(), Base: srv.URL, Now: func() time.Time { return at }}
	ctx := context.Background()
	if _, err := c.Resultado(ctx, "br", 1, 2); err != nil {
		t.Fatal(err)
	}
	at = at.Add(271 * time.Second)
	raw, err := c.Resultado(ctx, "br", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	obj, _ := raw.(map[string]any)
	cands, _ := obj["cand"].([]any)
	first, _ := cands[0].(map[string]any)
	if first["nm"] != "A" {
		t.Fatalf("body %v", raw)
	}
	at = at.Add(10 * time.Second)
	if _, err = c.Resultado(ctx, "br", 1, 2); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 2 {
		t.Fatalf("hits %d, want 2", hits.Load())
	}
}

func TestNotModifiedWithoutBody(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotModified)
	}))
	t.Cleanup(srv.Close)
	c := &Client{HTTP: srv.Client(), Base: srv.URL}
	if _, err := c.Resultado(context.Background(), "br", 1, 1); err == nil {
		t.Fatal("want error")
	}
	if _, err := c.Resultado(context.Background(), "br", 1, 1); err == nil {
		t.Fatal("304 must not be cached")
	}
	if hits.Load() != 2 {
		t.Fatalf("hits %d", hits.Load())
	}
}

func TestHTTPErrorIsNotCached(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)
	c := &Client{HTTP: srv.Client(), Base: srv.URL}
	ctx := context.Background()
	if _, err := c.Resultado(ctx, "mg", 3, 1); err == nil {
		t.Fatal("want error")
	}
	if _, err := c.Resultado(ctx, "mg", 3, 1); err == nil {
		t.Fatal("want error")
	}
	if hits.Load() != 2 {
		t.Fatalf("hits %d, want 2", hits.Load())
	}
}
