// Package panel builds the dashboard snapshot from several TSE files.
package panel

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/lewtec/lewkit/x/errs"
	"github.com/lewtec/lewkit/x/taskgroup"
	"github.com/lewtec/slopapura/internal/apuracao"
	"github.com/lewtec/slopapura/internal/tse"
)

// Query is one painel request.
type Query struct {
	UFs    []string
	Turno  int
	Limite int
	Mock   bool
	Now    func() time.Time
}

// Painel is the JSON body of /api/painel.
type Painel struct {
	PollTS     string            `json:"poll_ts"`
	Resultados []apuracao.Result `json:"resultados"`
	Avisos     []string          `json:"avisos"`
}

type job struct {
	i     int
	uf    string
	cargo int
}

// Snapshot loads every card.
// A missing file or a transport error drops that card and stays out of the error result.
func Snapshot(ctx context.Context, client *tse.Client, q Query) (Painel, error) {
	if q.Mock {
		return mockPainel(q.Now)
	}
	if q.Limite == 0 {
		q.Limite = 12
	}
	if q.Turno == 0 {
		q.Turno = 1
	}
	if client == nil {
		return Painel{}, errs.Wrap(errs.ErrMissing, "tse client")
	}
	var jobs []job
	for _, uf := range q.UFs {
		uf = strings.ToLower(strings.TrimSpace(uf))
		if uf == "" {
			continue
		}
		// DF uses the same four cargos as every other state. Cargo 8 is not requested.
		cargos := []int{3, 5, 6, 7}
		if uf == "br" {
			cargos = []int{1}
		}
		for _, cargo := range cargos {
			jobs = append(jobs, job{i: len(jobs), uf: uf, cargo: cargo})
		}
	}
	out := make([]*apuracao.Result, len(jobs))
	// Children stay on Control. The HTTP client promotes each GET to an
	// Internet task and holds that slot until the body is closed. An Internet
	// child that then calls the client deadlocks the pool (limit 4).
	err := taskgroup.WithSession(ctx, func(ctx context.Context) error {
		return taskgroup.Each[job]{
			Name:     "painel",
			Items:    jobs,
			PoolKind: taskgroup.Control,
			Fn: func(ctx context.Context, _ *taskgroup.Status, item job) error {
				res, ferr := one(ctx, client, item.uf, item.cargo, q.Turno, q.Limite)
				if ferr != nil {
					slog.DebugContext(ctx, "painel card", "uf", item.uf, "cargo", item.cargo, "err", ferr)
					return nil
				}
				out[item.i] = res
				return nil
			},
		}.Run(ctx)
	})
	if err != nil {
		return Painel{}, err
	}
	results := make([]apuracao.Result, 0, len(out))
	for _, item := range out {
		if item != nil {
			results = append(results, *item)
		}
	}
	return Painel{
		PollTS:     stamp(q.Now),
		Resultados: results,
		Avisos:     avisos(q.Turno, len(results) == 0),
	}, nil
}

func one(ctx context.Context, client *tse.Client, uf string, cargo, turno, limite int) (*apuracao.Result, error) {
	raw, err := client.Resultado(ctx, uf, cargo, turno)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, nil
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil, errs.Wrap(errs.ErrInvalid, "tse payload is not an object")
	}
	return apuracao.Normalize(obj, uf, cargo, limite), nil
}

func mockPainel(now func() time.Time) (Painel, error) {
	raw, err := apuracao.Fixture()
	if err != nil {
		return Painel{}, err
	}
	pairs := []struct {
		cargo int
		uf    string
	}{
		{1, "br"},
		{3, "mg"},
		{5, "mg"},
		{6, "mg"},
		{7, "mg"},
	}
	out := make([]apuracao.Result, 0, len(pairs))
	for _, pair := range pairs {
		item := apuracao.Normalize(raw, pair.uf, pair.cargo, 8)
		if item == nil {
			continue
		}
		item.Cargo = apuracao.CargoNome[pair.cargo]
		item.Abrangencia = strings.ToUpper(pair.uf)
		item.Mock = true
		out = append(out, *item)
	}
	return Painel{
		PollTS:     stamp(now),
		Resultados: out,
		Avisos:     []string{"MOCK de layout — dados do TSE 2022 clonados"},
	}, nil
}

func avisos(turno int, empty bool) []string {
	if empty {
		text := "Ainda sem dados publicados pelo TSE — a apuração começa a aparecer quando os arquivos forem gerados. Histórico: entre 17h30 e 18h15 no dia D."
		if turno == 2 {
			text += " Em 2º turno, só cargos em disputa aparecem."
		}
		return []string{text}
	}
	return []string{
		"Fonte: TSE divulga oficial (resultados.tse.jus.br) — parciais oficiais.",
		"Projeção local (não oficial) aparece como dica ciano 'eleito (proj.)': só impossibilidade matemática no pior caso.",
		"Deputados: top N por votos nominais (o cargo é proporcional — eleição por quociente/fator eleitoral, fechada pelo TSE no fim).",
	}
}

func stamp(now func() time.Time) string {
	t := time.Now()
	if now != nil {
		t = now()
	}
	return t.UTC().Format(time.RFC3339)
}
