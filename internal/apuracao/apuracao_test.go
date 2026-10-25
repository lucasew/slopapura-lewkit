package apuracao

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestNumbers(t *testing.T) {
	if asInt("1.234") != 1234 {
		t.Fatal(asInt("1.234"))
	}
	if asInt(nil) != 0 || asInt("") != 0 || asInt("nope") != 0 {
		t.Fatal("asInt zero")
	}
	if asPct("48,43") != 48.43 || asPct("100,00") != 100 || asPct("1.234,56") != 1234.56 {
		t.Fatalf("pct %v %v %v", asPct("48,43"), asPct("100,00"), asPct("1.234,56"))
	}
	if pick("0", "9") != "0" {
		t.Fatal("string 0 is present")
	}
	if pick(json.Number("0"), "9") != "9" {
		t.Fatal("numeric 0 falls through")
	}
	if GroupInt(57259504) != "57.259.504" || GroupInt(0) != "0" || GroupInt(1000) != "1.000" || GroupInt(-1234) != "-1.234" {
		t.Fatalf("%s %s %s %s", GroupInt(57259504), GroupInt(0), GroupInt(1000), GroupInt(-1234))
	}
}

func TestFixture(t *testing.T) {
	raw, err := Fixture()
	if err != nil {
		t.Fatal(err)
	}
	res := Normalize(raw, "br", 1, 12)
	if res == nil {
		t.Fatal("nil")
	}
	if res.Cargo != "Presidente" || res.Abrangencia != "BR" {
		t.Fatalf("%s %s", res.Cargo, res.Abrangencia)
	}
	if res.AtualizadoTSE != "04/10/2022 12:07:13" || res.TransmissaoTSE != "10:27:34" {
		t.Fatalf("clock %q %q", res.AtualizadoTSE, res.TransmissaoTSE)
	}
	if res.ApuradoPct != 100 || !res.MatematicamenteDefinido || !res.DadosValidos || !res.TotalizacaoFinal {
		t.Fatalf("flags %+v", res)
	}
	if res.Secoes.Totalizadas != 472075 || res.Secoes.Total != 472075 {
		t.Fatalf("secoes %+v", res.Secoes)
	}
	if res.Eleitorado.Eleitorado != 156454011 || res.Eleitorado.Comparecimento != 123682372 || res.Eleitorado.ComparecimentoPct != 79.05 || res.Eleitorado.AbstencaoPct != 20.95 {
		t.Fatalf("eleitorado %+v", res.Eleitorado)
	}
	if res.Votos.Validos != 118229719 || res.Votos.ValidosNominais != 0 || res.Votos.Brancos != 1964779 || res.Votos.BrancosPct != 1.59 || res.Votos.Nulos != 3487874 || res.Votos.NulosPct != 2.82 {
		t.Fatalf("votos %+v", res.Votos)
	}
	if res.ProjNota != "2º turno matematicamente garantido" {
		t.Fatal(res.ProjNota)
	}
	if res.EleicaoCodigo != "544" || res.Turno != "1" {
		t.Fatalf("ele %s turno %s", res.EleicaoCodigo, res.Turno)
	}
	if len(res.Candidatos) != 11 {
		t.Fatal(len(res.Candidatos))
	}
	got := res.Candidatos[0]
	want := Candidato{
		Numero: "13", Nome: "LULA", NomeCompleto: "LULA", Partido: "PT",
		Vice: "GERALDO ALCKMIN", SQCand: "280001607829",
		Foto:     "https://divulgacandcontas.tse.jus.br/divulga/rest/v1/candidatura/buscar/foto/2/2026/280001607829",
		TipoVoto: "Válido", Seq: "1",
		Votos: 57259504, Pct: 48.43, Situacao: "2º turno", Eleito: true, ProjEleito: true, ProjTipo: "2º turno",
	}
	if !strings.HasPrefix(got.Coligacao, "PT - ") {
		t.Fatal(got.Coligacao)
	}
	if diff := cmp.Diff(want, got, cmpopts.IgnoreFields(Candidato{}, "Coligacao"), cmpopts.EquateApprox(0, 1e-9)); diff != "" {
		t.Fatal(diff)
	}
	if !res.Candidatos[1].Eleito || res.Candidatos[1].ProjTipo != "2º turno" || res.Candidatos[1].Votos != 51072345 {
		t.Fatalf("second %+v", res.Candidatos[1])
	}
	if res.Candidatos[2].Eleito || res.Candidatos[2].ProjEleito || res.Candidatos[2].Situacao != "Não eleito" {
		t.Fatalf("third %+v", res.Candidatos[2])
	}
	names := make([]string, len(res.Candidatos))
	for i, cand := range res.Candidatos {
		names[i] = cand.Nome
	}
	wantNames := []string{"LULA", "JAIR BOLSONARO", "SIMONE TEBET", "CIRO GOMES", "SORAYA THRONICKE", "FELIPE D'AVILA", "PADRE KELMON", "LÉO PÉRICLES", "SOFIA MANZANO", "VERA", "CONSTITUINTE EYMAEL"}
	if diff := cmp.Diff(wantNames, names); diff != "" {
		t.Fatal(diff)
	}
	if strings.Contains(res.Candidatos[5].Nome, "&apos;") {
		t.Fatal(res.Candidatos[5].Nome)
	}
	short := Normalize(raw, "br", 1, 2)
	if len(short.Candidatos) != 2 || short.Candidatos[0].Numero != "13" {
		t.Fatalf("%+v", short.Candidatos)
	}
}

func TestFlatten2026(t *testing.T) {
	cand := map[string]any{
		"n": "13", "nm": "NOME", "vap": "80", "pvap": "80,00",
		"vs": []any{map[string]any{"nm": "VICE", "sqcand": "999"}},
	}
	raw := map[string]any{
		"carg": []any{map[string]any{
			"cd": "1", "nmn": "Presidente", "v": "1",
			"agr": []any{map[string]any{
				"com": "Coligação X",
				"par": []any{map[string]any{"sg": "PT", "cand": []any{cand}}},
			}},
		}},
		"s": map[string]any{"pst": "50,00", "st": "5", "ts": "10"},
		"e": map[string]any{"te": "100", "c": "40", "pc": "40,00"},
	}
	res := Normalize(raw, "br", 1, 12)
	if res == nil {
		t.Fatal("nil")
	}
	if _, ok := cand["cc"]; ok {
		t.Fatal("mutated candidate")
	}
	if _, ok := raw["cand"]; ok {
		t.Fatal("mutated raw")
	}
	got := res.Candidatos[0]
	if got.Coligacao != "PT - Coligação X" || got.Partido != "PT" || got.Vice != "VICE" || got.ViceSQ != "999" {
		t.Fatalf("%+v", got)
	}
	if res.CargoNomeTSE != "Presidente" {
		t.Fatal(res.CargoNomeTSE)
	}
	if Normalize(raw, "br", 3, 12) != nil {
		t.Fatal("other cargo must be empty")
	}
}

func TestVantagem(t *testing.T) {
	raw := map[string]any{
		"cand": []any{
			map[string]any{"n": "13", "nm": "A", "vap": "2000000", "pvap": "99,00"},
			map[string]any{"n": "22", "nm": "B", "vap": "1", "pvap": "1,00"},
		},
		"carg": []any{map[string]any{"v": "1"}},
		"s":    map[string]any{"ts": "100", "st": "100", "pst": "100,00"},
		"e":    map[string]any{"te": "1000"},
	}
	res := Normalize(raw, "br", 1, 12)
	if res.ProjNota != "vantagem de 1.999.999 votos > eleitores pendentes (0)" {
		t.Fatal(res.ProjNota)
	}
	if !res.Candidatos[0].ProjEleito || res.Candidatos[0].ProjTipo != "eleito" || res.Candidatos[1].ProjEleito {
		t.Fatalf("%+v %+v", res.Candidatos[0], res.Candidatos[1])
	}
}

func TestQuociente(t *testing.T) {
	raw := map[string]any{
		"cand": []any{
			map[string]any{"n": "1111", "nm": "A", "vap": "100", "cc": "PT"},
			map[string]any{"n": "2222", "nm": "B", "vap": "10", "cc": "PL"},
		},
		"carg": []any{map[string]any{"v": "2", "cd": "6"}},
		"s":    map[string]any{"ts": "10", "st": "10"},
		"e":    map[string]any{"te": "100"},
	}
	res := Normalize(raw, "mg", 6, 12)
	if res.Cargo != "Dep. Federal" || res.Abrangencia != "MG" {
		t.Fatalf("%s %s", res.Cargo, res.Abrangencia)
	}
	if res.ProjNota != "QP individual pior-caso: votos > 55" {
		t.Fatal(res.ProjNota)
	}
	if res.Candidatos[0].ProjTipo != "QP indiv." || res.Candidatos[1].ProjEleito {
		t.Fatalf("%+v %+v", res.Candidatos[0], res.Candidatos[1])
	}
}

func TestStableSort(t *testing.T) {
	raw := map[string]any{"cand": []any{
		map[string]any{"n": "1", "nm": "A", "vap": "10"},
		map[string]any{"n": "2", "nm": "B", "vap": "10"},
	}}
	res := Normalize(raw, "mg", 3, 12)
	if res.Candidatos[0].Nome != "A" || res.Candidatos[1].Nome != "B" {
		t.Fatalf("%s %s", res.Candidatos[0].Nome, res.Candidatos[1].Nome)
	}
}
