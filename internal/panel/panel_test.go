package panel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lewtec/slopapura/internal/tse"
)

func TestMock(t *testing.T) {
	out, err := Snapshot(context.Background(), nil, Query{Mock: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Resultados) != 5 || len(out.Avisos) != 1 || out.Avisos[0] != "MOCK de layout — dados do TSE 2022 clonados" {
		t.Fatalf("%d cards %v", len(out.Resultados), out.Avisos)
	}
	want := []struct{ cargo, uf string }{
		{"Presidente", "BR"},
		{"Governador", "MG"},
		{"Senador", "MG"},
		{"Dep. Federal", "MG"},
		{"Dep. Estadual", "MG"},
	}
	for i, item := range want {
		got := out.Resultados[i]
		if got.Cargo != item.cargo || got.Abrangencia != item.uf || !got.Mock || len(got.Candidatos) != 8 {
			t.Fatalf("%d %+v cand %d", i, got.Cargo, len(got.Candidatos))
		}
	}
	if out.PollTS == "" {
		t.Fatal("poll_ts")
	}
}

func TestOrderAndEmpty(t *testing.T) {
	var hits atomic.Int32
	var mu sync.Mutex
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		switch {
		case strings.Contains(r.URL.Path, "/br/br-c0001-"):
			time.Sleep(30 * time.Millisecond)
			_, _ = w.Write([]byte(`{"cand":[{"n":"13","nm":"PRES","vap":"10","pvap":"10,00"}],"dg":"25/10/2026","hg":"18:00:00"}`))
		case strings.Contains(r.URL.Path, "/mg/mg-c0003-"):
			_, _ = w.Write([]byte(`{"cand":[{"n":"45","nm":"GOV","vap":"9","pvap":"9,00"}],"dg":"25/10/2026","hg":"18:01:00"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	client := &tse.Client{HTTP: srv.Client(), Base: srv.URL}
	ctx := context.Background()
	out, err := Snapshot(ctx, client, Query{UFs: []string{"br", "mg"}, Turno: 2, Limite: 12})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Resultados) != 2 || out.Resultados[0].Candidatos[0].Nome != "PRES" || out.Resultados[1].Candidatos[0].Nome != "GOV" {
		t.Fatalf("%+v", out.Resultados)
	}
	if len(out.Avisos) != 3 {
		t.Fatal(out.Avisos)
	}
	first := hits.Load()
	if _, err = Snapshot(ctx, client, Query{UFs: []string{"br", "mg"}, Turno: 2, Limite: 12}); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != first {
		t.Fatalf("second pass refetched: %d -> %d", first, hits.Load())
	}

	empty, err := Snapshot(ctx, client, Query{UFs: []string{"zz"}, Turno: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.Resultados) != 0 || len(empty.Avisos) != 1 || !strings.Contains(empty.Avisos[0], "Em 2º turno") {
		t.Fatal(empty.Avisos)
	}
	turno1, err := Snapshot(ctx, client, Query{UFs: []string{"zz"}, Turno: 1})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(turno1.Avisos[0], "Em 2º turno") {
		t.Fatal(turno1.Avisos[0])
	}
}

func TestDistritoFederalSkipsCargo8(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	client := &tse.Client{HTTP: srv.Client(), Base: srv.URL}
	if _, err := Snapshot(context.Background(), client, Query{UFs: []string{"df"}, Turno: 1}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(paths) != 4 {
		t.Fatalf("paths %v", paths)
	}
	for _, path := range paths {
		if strings.Contains(path, "c0008") {
			t.Fatal(path)
		}
	}
}
