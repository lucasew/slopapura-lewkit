package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/lewtec/lewkit/x/errs"
	"github.com/lewtec/lewkit/x/http/asset"
	"github.com/lewtec/slopapura/internal/apuracao"
	"github.com/lewtec/slopapura/internal/panel"
	"github.com/lewtec/slopapura/internal/tse"
)

type application struct {
	tse *tse.Client
	now func() time.Time
}

func newHandler(ctx context.Context) (http.Handler, error) {
	client, err := tse.New(ctx)
	if err != nil {
		return nil, err
	}
	return newMux(&application{tse: client}), nil
}

func newMux(app *application) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", app.health)
	mux.HandleFunc("GET /api/config", app.config)
	mux.HandleFunc("GET /api/resultados", app.resultados)
	mux.HandleFunc("GET /api/demo", app.demo)
	mux.HandleFunc("GET /api/painel", app.painel)
	mux.HandleFunc("GET /{$}", app.page)
	return asset.Mount(mux)
}

func (a *application) clock() time.Time {
	if a != nil && a.now != nil {
		return a.now()
	}
	return time.Now()
}

func (a *application) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"time": a.clock().UTC().Format(time.RFC3339),
	})
}

func (a *application) config(w http.ResponseWriter, r *http.Request) {
	raw, err := a.tse.Config(r.Context())
	if err != nil {
		writeDetail(w, http.StatusBadGateway, fmt.Errorf("config TSE indisponível: %w", err).Error())
		return
	}
	writeJSON(w, http.StatusOK, raw)
}

func (a *application) resultados(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cargo, err := parseCargo(q)
	if err != nil {
		writeDetail(w, http.StatusBadRequest, err.Error())
		return
	}
	turno, err := parseTurno(q, 1)
	if err != nil {
		writeDetail(w, http.StatusBadRequest, err.Error())
		return
	}
	limite, err := parseLimite(q)
	if err != nil {
		writeDetail(w, http.StatusBadRequest, err.Error())
		return
	}
	raw, err := a.tse.Resultado(r.Context(), parseUF(q), cargo, turno)
	if err != nil {
		writeDetail(w, http.StatusBadGateway, fmt.Errorf("erro ao consultar TSE: %w", err).Error())
		return
	}
	var obj map[string]any
	if raw != nil {
		var ok bool
		obj, ok = raw.(map[string]any)
		if !ok {
			writeDetail(w, http.StatusBadGateway, "erro ao consultar TSE: payload is not an object")
			return
		}
	}
	out := apuracao.Normalize(obj, parseUF(q), cargo, limite)
	if out == nil {
		writeDetail(w, http.StatusNotFound, "arquivo ainda não publicado pelo TSE (apuração começa após 17h)")
		return
	}
	out.PollTS = a.clock().UTC().Format(time.RFC3339)
	writeJSON(w, http.StatusOK, out)
}

func (a *application) demo(w http.ResponseWriter, r *http.Request) {
	raw, err := apuracao.Fixture()
	if err != nil {
		writeDetail(w, http.StatusNotFound, "demo_fixture.json não encontrado")
		return
	}
	out := apuracao.Normalize(raw, "br", 1, 12)
	if out == nil {
		writeDetail(w, http.StatusNotFound, "arquivo ainda não publicado pelo TSE (apuração começa após 17h)")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *application) painel(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	turno, err := parseTurno(q, 1)
	if err != nil {
		writeDetail(w, http.StatusBadRequest, err.Error())
		return
	}
	limite, err := parseLimite(q)
	if err != nil {
		writeDetail(w, http.StatusBadRequest, err.Error())
		return
	}
	mock := false
	if s, ok := last(q, "mock"); ok {
		mock = flagOn(s)
	}
	out, err := panel.Snapshot(r.Context(), a.tse, panel.Query{
		UFs:    parseUFs(q),
		Turno:  turno,
		Limite: limite,
		Mock:   mock,
		Now:    a.clock,
	})
	if err != nil {
		writeDetail(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *application) page(w http.ResponseWriter, r *http.Request) {
	f := parsePage(r.URL.Query())
	snap, err := panel.Snapshot(r.Context(), a.tse, panel.Query{
		UFs:    ufsFor(f.UF, f.PresBR),
		Turno:  f.Turno,
		Limite: 12,
		Mock:   f.Mock,
		Now:    a.clock,
	})
	var buf bytes.Buffer
	if rerr := pageDocument(buildPage(f, snap, err, a.clock())).Render(r.Context(), &buf); rerr != nil {
		http.Error(w, rerr.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

func parsePage(q url.Values) filters {
	f := filters{UF: "mg", Turno: 2, PresBR: true, Auto: true}
	if s, ok := last(q, "uf"); ok {
		s = strings.ToLower(strings.TrimSpace(s))
		if knownUF(s) {
			f.UF = s
		}
	}
	if s, ok := last(q, "turno"); ok && (s == "1" || s == "2") {
		f.Turno = int(s[0] - '0')
	}
	if s, ok := last(q, "br"); ok && s == "0" {
		f.PresBR = false
	}
	if s, ok := last(q, "auto"); ok && s == "0" {
		f.Auto = false
	}
	if s, ok := last(q, "mock"); ok {
		f.Mock = flagOn(s)
	}
	return f
}

func parseUFs(q url.Values) []string {
	spec := "br,mg"
	if s, ok := last(q, "ufs"); ok {
		spec = s
	}
	var out []string
	for _, part := range strings.Split(spec, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseUF(q url.Values) string {
	s, ok := last(q, "uf")
	if !ok {
		return "br"
	}
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "br"
	}
	return s
}

func parseCargo(q url.Values) (int, error) {
	s, ok := last(q, "cargo")
	if !ok || s == "" {
		return 1, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || apuracao.CargoNome[n] == "" {
		return 0, errs.Wrap(errs.ErrInvalid, "código de cargo inválido")
	}
	return n, nil
}

func parseTurno(q url.Values, def int) (int, error) {
	s, ok := last(q, "turno")
	if !ok || s == "" {
		return def, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || (n != 1 && n != 2) {
		return 0, errs.Wrap(errs.ErrInvalid, "turno inválido")
	}
	return n, nil
}

func parseLimite(q url.Values) (int, error) {
	s, ok := last(q, "limite")
	if !ok || s == "" {
		return 12, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 60 {
		return 0, errs.Wrap(errs.ErrInvalid, "limite inválido")
	}
	return n, nil
}

func last(q url.Values, key string) (string, bool) {
	vs, ok := q[key]
	if !ok || len(vs) == 0 {
		return "", false
	}
	return vs[len(vs)-1], true
}

func flagOn(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "t", "true", "y", "yes", "on":
		return true
	default:
		return false
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_, _ = w.Write(buf.Bytes())
}

func writeDetail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"detail": msg})
}
