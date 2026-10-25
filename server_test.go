package main

import (
	"encoding/json"
	"html"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lewtec/slopapura/internal/panel"
	"github.com/lewtec/slopapura/internal/tse"
)

func TestAPI(t *testing.T) {
	fixed := time.Date(2026, 10, 25, 18, 0, 0, 0, time.UTC)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if strings.Contains(r.URL.Path, "/br/br-c0001-e006258-") {
			_, _ = w.Write([]byte(`{"cand":[{"n":"13","nm":"LULA","vap":"10","pvap":"10,00"}]}`))
			return
		}
		if strings.Contains(r.URL.Path, "ele-c.json") {
			_, _ = w.Write([]byte(`{"ciclo":"2026"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	app := &application{
		tse: &tse.Client{HTTP: srv.Client(), Base: srv.URL},
		now: func() time.Time { return fixed },
	}
	h := newMux(app)

	rec := get(h, "/api/health")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"ok":true`) || !strings.Contains(rec.Body.String(), "2026-10-25T18:00:00Z") {
		t.Fatalf("health %d %s", rec.Code, rec.Body.String())
	}

	rec = get(h, "/api/demo")
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "poll_ts") || !strings.Contains(rec.Body.String(), "LULA") {
		t.Fatalf("demo %d %s", rec.Code, rec.Body.String()[:80])
	}
	if !strings.Contains(rec.Body.String(), "divulgacandcontas.tse.jus.br") {
		t.Fatal("demo foto")
	}
	before := hits.Load()

	rec = get(h, "/api/resultados?cargo=2")
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "código de cargo inválido") {
		t.Fatalf("cargo %d %s", rec.Code, rec.Body.String())
	}
	if hits.Load() != before {
		t.Fatal("invalid cargo hit TSE")
	}

	rec = get(h, "/api/resultados?uf=br&cargo=1&turno=2")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"poll_ts"`) {
		t.Fatalf("resultados %d %s", rec.Code, rec.Body.String())
	}

	rec = get(h, "/api/resultados?uf=sp&cargo=3&turno=2")
	if rec.Code != 404 || !strings.Contains(rec.Body.String(), "arquivo ainda não publicado pelo TSE (apuração começa após 17h)") {
		t.Fatalf("404 %d %s", rec.Code, rec.Body.String())
	}

	rec = get(h, "/api/config")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "2026") {
		t.Fatalf("config %d %s", rec.Code, rec.Body.String())
	}

	rec = get(h, "/api/painel?mock=true")
	if rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	var body panel.Painel
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Resultados) != 5 || body.Avisos[0] != "MOCK de layout — dados do TSE 2022 clonados" {
		t.Fatalf("%d %v", len(body.Resultados), body.Avisos)
	}
}

func TestPage(t *testing.T) {
	fixed := time.Date(2026, 10, 25, 18, 0, 0, 0, time.UTC)
	var hits atomic.Int32
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		paths = append(paths, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	app := &application{
		tse: &tse.Client{HTTP: srv.Client(), Base: srv.URL},
		now: func() time.Time { return fixed },
	}
	h := newMux(app)

	rec := get(h, "/")
	body := rec.Body.String()
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("page %d %s", rec.Code, rec.Header().Get("Cache-Control"))
	}
	for _, needle := range []string{
		"Apuração 2026",
		"Aguardando o TSE…",
		"17h30 e 18h15",
		"15:00:00",
		"every 300s",
		"outerMorph",
		"/__lewkit__/htmx.min.js",
		"/__lewkit__/daisyui.css",
		html.EscapeString(successNote),
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("missing %s", needle)
		}
	}
	if strings.Contains(body, "function card") || strings.Contains(body, "innerHTML") {
		t.Fatal("client renderer")
	}
	if !optionSelected(body, "2") || !optionSelected(body, "mg") || !checkbox(body, "br") || !checkbox(body, "auto") {
		t.Fatal("defaults")
	}

	before := hits.Load()
	rec = get(h, "/?mock=1")
	body = rec.Body.String()
	if hits.Load() != before {
		t.Fatal("mock hit TSE")
	}
	for _, needle := range []string{"LULA", "57.259.504", "48,43", "MOCK", "D&#39;AVILA", "123.682.372", "apurado 100%"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("mock missing %s", needle)
		}
	}
	if strings.Contains(body, "D&apos;") || strings.Contains(body, "divulgacandcontas") {
		t.Fatal("fixture entity or photo url in html")
	}

	paths = nil
	hits.Store(0)
	rec = get(h, "/?uf=sp&turno=1&br=0&br=1&auto=1&auto=0&mock=0")
	body = rec.Body.String()
	if !optionSelected(body, "sp") || !optionSelected(body, "1") || !checkbox(body, "br") || checkbox(body, "auto") {
		t.Fatal("query flags")
	}
	if strings.Contains(body, "every 300s") {
		t.Fatal("auto off still polls")
	}
	joined := strings.Join(paths, " ")
	if !strings.Contains(joined, "/dados/br/") || !strings.Contains(joined, "/dados/sp/") {
		t.Fatal(joined)
	}

	rec = get(h, "/__lewkit__/htmx.min.js")
	if rec.Code != 200 || rec.Body.Len() < 1000 {
		t.Fatalf("htmx %d %d", rec.Code, rec.Body.Len())
	}
	rec = get(h, "/__lewkit__/daisyui.css")
	if rec.Code != 200 || rec.Body.Len() < 1000 {
		t.Fatalf("daisy %d", rec.Code)
	}
	rec = get(h, "/__lewkit__/tailwindcss.js")
	if rec.Code != 200 {
		t.Fatal(rec.Code)
	}
}

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func optionSelected(html, value string) bool {
	re := regexp.MustCompile(`<option value="` + regexp.QuoteMeta(value) + `"[^>]*>`)
	return strings.Contains(re.FindString(html), "selected")
}

func checkbox(html, name string) bool {
	re := regexp.MustCompile(`<input type="checkbox"[^>]*name="` + regexp.QuoteMeta(name) + `"[^>]*>`)
	return strings.Contains(re.FindString(html), "checked")
}

func TestPartyColor(t *testing.T) {
	if partyColor("PT", "13") != "#4ecdc4" || partyColor("UNIÃO", "44") != "#66bb6a" || partyColor("PTB", "14") != "#4ecdc4" {
		t.Fatalf("%s %s %s", partyColor("PT", "13"), partyColor("UNIÃO", "44"), partyColor("PTB", "14"))
	}
	if partyColor("NOVO", "30") != "#26a69a" || partyColor("", "13") != "#ec407a" || partyColor("", "") != "#ff6b6b" {
		t.Fatalf("%s %s %s", partyColor("NOVO", "30"), partyColor("", "13"), partyColor("", ""))
	}
	if barColor(0) != "#ff6b6b" || barColor(7) != "#f778ba" || barColor(9) != "#ff6b6b" {
		t.Fatalf("%s %s %s", barColor(0), barColor(7), barColor(9))
	}
}
