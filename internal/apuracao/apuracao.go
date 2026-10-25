// Package apuracao normalizes a TSE result file and applies the local projection.
package apuracao

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html"
	"math"
	"sort"
	"strconv"
	"strings"
)

//go:embed demo_fixture.json
var fixtureJSON []byte

// CargoNome is the dashboard name of a TSE cargo code.
var CargoNome = map[int]string{
	1: "Presidente",
	3: "Governador",
	5: "Senador",
	6: "Dep. Federal",
	7: "Dep. Estadual",
	8: "Dep. Distrital",
}

// Result is one cargo in one UF.
type Result struct {
	PollTS                  string      `json:"poll_ts,omitempty"`
	Cargo                   string      `json:"cargo"`
	Abrangencia             string      `json:"abrangencia"`
	AtualizadoTSE           string      `json:"atualizado_tse"`
	TransmissaoTSE          string      `json:"transmissao_tse,omitempty"`
	ApuradoPct              float64     `json:"apurado_pct"`
	Secoes                  Secoes      `json:"secoes"`
	Eleitorado              Eleitorado  `json:"eleitorado"`
	Votos                   Votos       `json:"votos"`
	MatematicamenteDefinido bool        `json:"matematicamente_definido"`
	IDGeracao               string      `json:"id_geracao,omitempty"`
	EleicaoCodigo           string      `json:"eleicao_codigo,omitempty"`
	Turno                   string      `json:"turno,omitempty"`
	DadosValidos            bool        `json:"dados_validos"`
	TotalizacaoFinal        bool        `json:"totalizacao_final"`
	CargoNomeTSE            string      `json:"cargo_nome_tse,omitempty"`
	ProjNota                string      `json:"proj_nota,omitempty"`
	Mock                    bool        `json:"mock,omitempty"`
	Candidatos              []Candidato `json:"candidatos"`
}

// Secoes counts polling sections.
type Secoes struct {
	Totalizadas     int `json:"totalizadas"`
	Total           int `json:"total"`
	Instanciadas    int `json:"instanciadas"`
	Ativas          int `json:"ativas"`
	NaoTransmitidas int `json:"nao_transmitidas"`
}

// Eleitorado is the electorate block.
type Eleitorado struct {
	Eleitorado        int     `json:"eleitorado"`
	Comparecimento    int     `json:"comparecimento"`
	ComparecimentoPct float64 `json:"comparecimento_pct"`
	AbstencaoPct      float64 `json:"abstencao_pct"`
}

// Votos is the ballot total block.
// ValidosNominais is read only from the nested v.vnom field.
type Votos struct {
	Validos         int     `json:"validos"`
	ValidosNominais int     `json:"validos_nominais"`
	Brancos         int     `json:"brancos"`
	BrancosPct      float64 `json:"brancos_pct"`
	Nulos           int     `json:"nulos"`
	NulosPct        float64 `json:"nulos_pct"`
}

// Candidato is one row after the vote sort and the limit.
type Candidato struct {
	Numero       string  `json:"numero"`
	Nome         string  `json:"nome"`
	NomeCompleto string  `json:"nome_completo"`
	Partido      string  `json:"partido"`
	Coligacao    string  `json:"coligacao"`
	Vice         string  `json:"vice"`
	ViceSQ       string  `json:"vice_sq,omitempty"`
	SQCand       string  `json:"sqcand,omitempty"`
	Foto         string  `json:"foto,omitempty"`
	Nascimento   string  `json:"nascimento,omitempty"`
	TipoVoto     string  `json:"tipo_voto,omitempty"`
	Seq          string  `json:"seq,omitempty"`
	Votos        int     `json:"votos"`
	Pct          float64 `json:"pct"`
	PctExato     float64 `json:"pct_exato"`
	Situacao     string  `json:"situacao,omitempty"`
	Eleito       bool    `json:"eleito"`
	ProjEleito   bool    `json:"proj_eleito,omitempty"`
	ProjTipo     string  `json:"proj_tipo,omitempty"`
}

// Fixture is the 2022 president file used as the layout demo.
func Fixture() (map[string]any, error) {
	return decodeObject(fixtureJSON)
}

func decodeObject(body []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var val any
	if err := dec.Decode(&val); err != nil {
		return nil, err
	}
	obj, ok := val.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("tse payload is not an object")
	}
	return obj, nil
}

// Normalize sorts, limits, and projects one file.
// It returns nil when the file has no candidate for cargo.
// raw is not modified.
func Normalize(raw map[string]any, uf string, cargo, limite int) *Result {
	if raw == nil {
		return nil
	}
	work := raw
	if _, hasCand := raw["cand"]; !hasCand {
		if _, hasCarg := raw["carg"]; hasCarg {
			work = cloneMap(raw)
			work["cand"] = flatten(raw, cargo)
		}
	}
	list, _ := work["cand"].([]any)
	if len(list) == 0 {
		return nil
	}
	ordered := append([]any(nil), list...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return asInt(field(ordered[i], "vap")) > asInt(field(ordered[j], "vap"))
	})
	if limite < 0 {
		limite = 0
	}
	if limite < len(ordered) {
		ordered = ordered[:limite]
	}
	cands := make([]Candidato, 0, len(ordered))
	for _, item := range ordered {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		cands = append(cands, candidate(obj))
	}
	nota := project(work, cands, cargo)
	sblk := blk(work, "s")
	eblk := blk(work, "e")
	vblk := blk(work, "v")
	out := &Result{
		Cargo:          cargoName(cargo),
		Abrangencia:    strings.ToUpper(uf),
		AtualizadoTSE:  strings.TrimSpace(str(work["dg"]) + " " + str(work["hg"])),
		TransmissaoTSE: str(work["ht"]),
		ApuradoPct:     asPct(pick(sblk["pst"], work["pst"])),
		Secoes: Secoes{
			Totalizadas:     asInt(pick(sblk["st"], work["st"])),
			Total:           asInt(pick(sblk["ts"], work["s"])),
			Instanciadas:    asInt(sblk["si"]),
			Ativas:          asInt(sblk["sa"]),
			NaoTransmitidas: asInt(sblk["snt"]),
		},
		Eleitorado: Eleitorado{
			Eleitorado:        asInt(pick(eblk["te"], work["e"])),
			Comparecimento:    asInt(pick(eblk["c"], work["c"])),
			ComparecimentoPct: asPct(pick(eblk["pc"], work["pc"])),
			AbstencaoPct:      asPct(pick(eblk["pa"], work["pa"])),
		},
		Votos: Votos{
			Validos:         asInt(pick(vblk["vvc"], work["vv"])),
			ValidosNominais: asInt(vblk["vnom"]),
			Brancos:         asInt(pick(vblk["vb"], work["vb"])),
			BrancosPct:      asPct(pick(vblk["pvb"], work["pvb"])),
			Nulos:           asInt(pick(vblk["tvn"], work["tvn"])),
			NulosPct:        asPct(pick(vblk["ptvn"], work["ptvn"])),
		},
		MatematicamenteDefinido: strings.EqualFold(str(work["md"]), "s"),
		IDGeracao:               str(work["idg"]),
		EleicaoCodigo:           str(work["ele"]),
		Turno:                   str(work["t"]),
		DadosValidos:            str(work["dv"]) == "s",
		TotalizacaoFinal:        str(work["tf"]) == "s",
		CargoNomeTSE:            cargoNomeTSE(work),
		ProjNota:                nota,
		Candidatos:              cands,
	}
	return out
}

func cargoName(cargo int) string {
	if name, ok := CargoNome[cargo]; ok {
		return name
	}
	return strconv.Itoa(cargo)
}

func cargoNomeTSE(raw map[string]any) string {
	carg, _ := raw["carg"].([]any)
	if len(carg) == 0 {
		return ""
	}
	first, _ := carg[0].(map[string]any)
	if first == nil {
		return ""
	}
	return str(first["nmn"])
}

func flatten(raw map[string]any, cargo int) []any {
	want := strconv.Itoa(cargo)
	var out []any
	cargos, _ := raw["carg"].([]any)
	for _, item := range cargos {
		carg, _ := item.(map[string]any)
		if carg == nil || str(carg["cd"]) != want {
			continue
		}
		agrs, _ := carg["agr"].([]any)
		for _, agrItem := range agrs {
			agr, _ := agrItem.(map[string]any)
			if agr == nil {
				continue
			}
			pars, _ := agr["par"].([]any)
			for _, parItem := range pars {
				par, _ := parItem.(map[string]any)
				if par == nil {
					continue
				}
				cands, _ := par["cand"].([]any)
				for _, candItem := range cands {
					src, _ := candItem.(map[string]any)
					if src == nil {
						continue
					}
					dst := cloneMap(src)
					if _, exists := dst["cc"]; !exists || dst["cc"] == nil {
						if truthy(agr["com"]) {
							dst["cc"] = str(par["sg"]) + " - " + str(agr["com"])
						} else {
							dst["cc"] = str(par["sg"])
						}
					}
					out = append(out, dst)
				}
			}
		}
	}
	return out
}

// project marks a seat only when the pending electorate cannot change it.
// P is the electorate still inside sections that have not reported.
func project(raw map[string]any, cands []Candidato, cargo int) string {
	eblk := blk(raw, "e")
	sblk := blk(raw, "s")
	te := asInt(pick(eblk["te"], raw["e"]))
	ts := asInt(pick(sblk["ts"], raw["s"]))
	st := asInt(pick(sblk["st"], raw["st"]))
	pend := ts - st
	if pend < 0 {
		pend = 0
	}
	var pPend float64
	if ts != 0 {
		pPend = float64(te) * float64(pend) / float64(ts)
	}
	vagas := vagasOf(raw)
	votos := make([]int, len(cands))
	tot := 0
	for i, cand := range cands {
		n := cand.Votos
		if n < 0 {
			n = 0
		}
		votos[i] = n
		tot += n
	}
	nota := ""
	if majoritarian(cargo) && len(votos) > vagas {
		if cargo == 1 && vagas == 1 && float64(tot)+pPend > 0 {
			share := (float64(votos[0]) + pPend) / (float64(tot) + pPend) * 100
			if share < 50 {
				n := 2
				if len(cands) < n {
					n = len(cands)
				}
				for i := 0; i < n; i++ {
					cands[i].ProjEleito = true
					cands[i].ProjTipo = "2º turno"
				}
				nota = "2º turno matematicamente garantido"
			}
		}
		seats := vagas
		if seats > len(votos) {
			seats = len(votos)
		}
		for j := 0; j < seats; j++ {
			rival := 0
			if vagas < len(votos) {
				rival = votos[vagas]
			}
			diff := votos[j] - rival
			if float64(diff) > pPend {
				cands[j].ProjEleito = true
				if cands[j].ProjTipo == "" {
					cands[j].ProjTipo = "eleito"
				}
				if nota == "" {
					nota = fmt.Sprintf("vantagem de %s votos > eleitores pendentes (%s)", GroupInt(diff), GroupInt(int(math.Round(pPend))))
				}
			}
		}
	} else if proportional(cargo) && tot > 0 && vagas > 1 {
		qe := (float64(tot) + pPend) / float64(vagas)
		marked := false
		for i := range cands {
			if float64(votos[i]) > qe {
				cands[i].ProjEleito = true
				cands[i].ProjTipo = "QP indiv."
				marked = true
			}
		}
		if marked {
			nota = fmt.Sprintf("QP individual pior-caso: votos > %s", GroupInt(int(math.Round(qe))))
		}
	}
	return nota
}

func majoritarian(cargo int) bool {
	return cargo == 1 || cargo == 3 || cargo == 5
}

func proportional(cargo int) bool {
	return cargo == 6 || cargo == 7 || cargo == 8
}

func vagasOf(raw map[string]any) int {
	carg, ok := raw["carg"].([]any)
	if !ok || len(carg) == 0 {
		return 1
	}
	first, ok := carg[0].(map[string]any)
	if !ok {
		return 1
	}
	n := asInt(first["v"])
	if n < 1 {
		return 1
	}
	return n
}

func candidate(raw map[string]any) Candidato {
	cc := html.UnescapeString(str(raw["cc"]))
	partido := ""
	if cc != "" {
		partido = strings.Split(cc, " - ")[0]
	}
	var vice, viceSQ string
	if vs, ok := raw["vs"].([]any); ok {
		if len(vs) > 0 {
			if first, ok := vs[0].(map[string]any); ok {
				vice = html.UnescapeString(str(first["nm"]))
				viceSQ = str(first["sqcand"])
			}
		}
	} else {
		vice = html.UnescapeString(str(raw["nv"]))
	}
	sq := str(raw["sqcand"])
	foto := ""
	if sq != "" {
		foto = "https://divulgacandcontas.tse.jus.br/divulga/rest/v1/candidatura/buscar/foto/2/2026/" + sq
	}
	return Candidato{
		Numero:       str(raw["n"]),
		Nome:         html.UnescapeString(str(pick(raw["nmu"], raw["nm"]))),
		NomeCompleto: html.UnescapeString(str(raw["nm"])),
		Partido:      partido,
		Coligacao:    cc,
		Vice:         vice,
		ViceSQ:       viceSQ,
		SQCand:       sq,
		Foto:         foto,
		Nascimento:   str(raw["dt"]),
		TipoVoto:     str(raw["dvt"]),
		Seq:          str(raw["seq"]),
		Votos:        asInt(raw["vap"]),
		Pct:          asPct(raw["pvap"]),
		PctExato:     asPct(raw["pvapn"]),
		Situacao:     str(raw["st"]),
		Eleito:       str(raw["e"]) == "s",
	}
}

func blk(raw map[string]any, key string) map[string]any {
	obj, ok := raw[key].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return obj
}

func field(v any, key string) any {
	obj, _ := v.(map[string]any)
	if obj == nil {
		return nil
	}
	return obj[key]
}

func cloneMap(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src)+1)
	for key, val := range src {
		dst[key] = val
	}
	return dst
}

func pick(vals ...any) any {
	for _, val := range vals {
		if truthy(val) {
			return val
		}
	}
	return nil
}

func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return x != ""
	case bool:
		return x
	case int:
		return x != 0
	case int64:
		return x != 0
	case float64:
		return x != 0
	case json.Number:
		if x == "" {
			return false
		}
		f, err := x.Float64()
		if err != nil {
			return true
		}
		return f != 0
	case []any:
		return len(x) != 0
	case map[string]any:
		return len(x) != 0
	default:
		return true
	}
}

func str(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case json.Number:
		return x.String()
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprint(x)
	}
}

func asInt(v any) int {
	if v == nil {
		return 0
	}
	s := strings.ReplaceAll(str(v), ".", "")
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

func asPct(v any) float64 {
	if v == nil {
		return 0
	}
	s := str(v)
	if s == "" {
		return 0
	}
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ",", ".")
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

// GroupInt formats n with a dot between each group of three digits.
func GroupInt(n int) string {
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	s := strconv.Itoa(n)
	lead := len(s) % 3
	if lead == 0 {
		lead = 3
	}
	var b strings.Builder
	b.WriteString(sign)
	b.WriteString(s[:lead])
	for i := lead; i < len(s); i += 3 {
		b.WriteByte('.')
		b.WriteString(s[i : i+3])
	}
	return b.String()
}
