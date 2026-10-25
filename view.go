package main

import (
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/a-h/templ"
	"github.com/lewtec/slopapura/internal/apuracao"
	"github.com/lewtec/slopapura/internal/panel"
)

const (
	successNote = `Percentuais e horário: publicação oficial do TSE (horário de Brasília). "eleito (proj.)" = projeção local conservadora, não oficial. Deputados: lista nominal, não é a eleição.`
	errorNote   = `Se "ainda não publicado": os arquivos nascem quando a apuração começa (após 17h).`
)

var barColors = []string{
	"#ff6b6b", "#5b9bff", "#3fb950", "#d29922", "#bc8cff",
	"#39c5cf", "#ff9f43", "#f778ba", "#9ae6ae",
}

var partPrefix = []string{
	"PL", "PT", "PSB", "PDT", "MDB", "UN", "PP", "PSD", "RE", "PO",
	"PSOL", "PC", "PV", "C", "NO", "MO", "AG", "SA", "CI", "MB",
}

var partColors = []string{
	"#ff6b6b", "#4ecdc4", "#45b7d1", "#f9a825", "#ab47bc",
	"#66bb6a", "#ef5350", "#26c6da", "#8d6e63", "#7e57c2",
	"#29b6f6", "#9ccc65", "#ffa726", "#ec407a", "#26a69a",
	"#5c6bc0", "#d4e157", "#ff7043", "#42a5f5", "#8d6e63",
}

// estados is the "todos" fetch order. mg sits between ms and pa.
var estados = []string{
	"ac", "al", "ap", "am", "ba", "ce", "df", "es", "go", "ma", "mt", "ms",
	"mg", "pa", "pb", "pr", "pe", "pi", "rn", "rj", "rs", "ro", "rr", "sc", "sp", "se", "to",
}

type ufOption struct {
	Value string
	Label string
}

var ufChoices = []ufOption{
	{"mg", "MG — Minas Gerais"},
	{"todos", "— TODOS os estados —"},
	{"ac", "AC — Acre"},
	{"al", "AL — Alagoas"},
	{"ap", "AP — Amapá"},
	{"am", "AM — Amazonas"},
	{"ba", "BA — Bahia"},
	{"ce", "CE — Ceará"},
	{"df", "DF — Distrito Federal"},
	{"es", "ES — Espírito Santo"},
	{"go", "GO — Goiás"},
	{"ma", "MA — Maranhão"},
	{"mt", "MT — Mato Grosso"},
	{"ms", "MS — Mato Grosso do Sul"},
	{"pa", "PA — Pará"},
	{"pb", "PB — Paraíba"},
	{"pr", "PR — Paraná"},
	{"pe", "PE — Pernambuco"},
	{"pi", "PI — Piauí"},
	{"rn", "RN — Rio Grande do Norte"},
	{"rj", "RJ — Rio de Janeiro"},
	{"rs", "RS — Rio Grande do Sul"},
	{"ro", "RO — Rondônia"},
	{"rr", "RR — Roraima"},
	{"sc", "SC — Santa Catarina"},
	{"sp", "SP — São Paulo"},
	{"se", "SE — Sergipe"},
	{"to", "TO — Tocantins"},
}

type filters struct {
	UF     string
	Turno  int
	PresBR bool
	Auto   bool
	Mock   bool
}

func (f filters) query() string {
	v := url.Values{}
	v.Set("uf", f.UF)
	v.Set("turno", strconv.Itoa(f.Turno))
	if f.PresBR {
		v.Set("br", "1")
	} else {
		v.Set("br", "0")
	}
	if f.Auto {
		v.Set("auto", "1")
	} else {
		v.Set("auto", "0")
	}
	if f.Mock {
		v.Set("mock", "1")
	}
	return "/?" + v.Encode()
}

func ufsFor(uf string, pres bool) []string {
	var out []string
	if pres {
		out = append(out, "br")
	}
	switch uf {
	case "todos":
		out = append(out, estados...)
	case "br":
		if !pres {
			out = append(out, "br")
		}
	default:
		if uf != "" {
			out = append(out, uf)
		}
	}
	if len(out) == 0 {
		return []string{"br"}
	}
	return out
}

func knownUF(s string) bool {
	for _, opt := range ufChoices {
		if opt.Value == s {
			return true
		}
	}
	return false
}

type pageView struct {
	Turno       int
	UF          string
	PresBR      bool
	Auto        bool
	Mock        bool
	PollURL     string
	Clock       string
	Status      string
	StatusClass string
	Note        string
	Cards       []cardView
	Waiting     bool
}

type cardView struct {
	Class          string
	Cargo          string
	UF             string
	Mock           bool
	Definido       bool
	Proj           string
	Meta           string
	Tip            string
	Rows           []rowView
	Comparecimento string
	Brancos        string
	Nulos          string
}

type rowView struct {
	Numero     string
	Nome       string
	NomeClass  string
	Badge      string
	BadgeClass string
	Pct        string
	Votos      string
	Sub        string
	Vice       string
	Tip        string
	Initials   string
	BarCSS     map[string]templ.SafeCSSProperty
	AvatarCSS  map[string]templ.SafeCSSProperty
}

func buildPage(f filters, snap panel.Painel, err error, now time.Time) pageView {
	v := pageView{
		Turno:       f.Turno,
		UF:          f.UF,
		PresBR:      f.PresBR,
		Auto:        f.Auto,
		Mock:        f.Mock,
		Clock:       brazilClock(now),
		StatusClass: "text-xs tabular-nums",
	}
	if f.Auto {
		v.PollURL = f.query()
	}
	if err != nil {
		v.Status = "erro: " + err.Error()
		v.StatusClass += " text-error"
		v.Note = errorNote
		return v
	}
	ts := ""
	if len(snap.Resultados) > 0 {
		ts = snap.Resultados[0].AtualizadoTSE
	}
	v.Status = "ok — TSE: " + ts
	v.Note = successNote
	if len(snap.Resultados) == 0 {
		v.Waiting = true
		return v
	}
	v.Cards = make([]cardView, 0, len(snap.Resultados))
	for _, item := range snap.Resultados {
		v.Cards = append(v.Cards, cardFrom(item))
	}
	return v
}

func cardFrom(r apuracao.Result) cardView {
	class := "card w-96 shrink-0 snap-start border bg-base-200"
	if r.Mock {
		class += " border-warning"
	} else {
		class += " border-base-300"
	}
	card := cardView{
		Class:          class,
		Cargo:          r.Cargo,
		UF:             r.Abrangencia,
		Mock:           r.Mock,
		Definido:       r.MatematicamenteDefinido,
		Proj:           r.ProjNota,
		Meta:           metaLine(r),
		Comparecimento: fmt.Sprintf("%s (%s%%)", apuracao.GroupInt(r.Eleitorado.Comparecimento), jsNumber(r.Eleitorado.ComparecimentoPct)),
		Brancos:        fmt.Sprintf("%s (%s%%)", apuracao.GroupInt(r.Votos.Brancos), jsNumber(r.Votos.BrancosPct)),
		Nulos:          fmt.Sprintf("%s (%s%%)", apuracao.GroupInt(r.Votos.Nulos), jsNumber(r.Votos.NulosPct)),
	}
	if !r.Mock {
		card.Tip = cardTip(r)
	}
	card.Rows = make([]rowView, 0, len(r.Candidatos))
	for i, cand := range r.Candidatos {
		card.Rows = append(card.Rows, rowFrom(cand, i))
	}
	return card
}

func rowFrom(c apuracao.Candidato, i int) rowView {
	row := rowView{
		Numero:    c.Numero,
		Nome:      trunc(c.Nome, 26),
		Pct:       pctComma(c.Pct, 2),
		Votos:     apuracao.GroupInt(c.Votos),
		Vice:      trunc(c.Vice, 18),
		Initials:  initials(c.Nome),
		Tip:       rowTip(c),
		BarCSS:    barCSS(i, c.Pct),
		AvatarCSS: avatarCSS(c.Partido, c.Numero),
	}
	colig := c.Coligacao
	if colig == "" {
		colig = c.Partido
	}
	row.Sub = trunc(colig, 70)
	if c.Vice == "" {
		row.Vice = ""
	}
	switch {
	case c.Eleito:
		row.Badge = c.Situacao
		if row.Badge == "" {
			row.Badge = "Eleito"
		}
		row.BadgeClass = "badge badge-success"
		row.NomeClass = "font-bold text-success"
	case c.ProjEleito:
		row.Badge = c.ProjTipo
		if row.Badge == "" {
			row.Badge = "Eleito (proj.)"
		}
		row.BadgeClass = "badge badge-info"
		row.NomeClass = "font-bold text-success"
	case c.Situacao != "":
		row.Badge = c.Situacao
		row.BadgeClass = "badge"
	}
	return row
}

func metaLine(r apuracao.Result) string {
	tx := ""
	if r.TransmissaoTSE != "" {
		tx = " (tx " + r.TransmissaoTSE + ")"
	}
	pend := ""
	if r.Secoes.NaoTransmitidas != 0 {
		pend = " (" + apuracao.GroupInt(r.Secoes.NaoTransmitidas) + " pendentes)"
	}
	return fmt.Sprintf("TSE: %s%s · apurado %s%% · seções %s/%s%s",
		r.AtualizadoTSE, tx, jsNumber(r.ApuradoPct),
		apuracao.GroupInt(r.Secoes.Totalizadas), apuracao.GroupInt(r.Secoes.Total), pend)
}

func cardTip(r apuracao.Result) string {
	dados := "não"
	if r.DadosValidos {
		dados = "sim"
	}
	fim := "não"
	if r.TotalizacaoFinal {
		fim = "SIM"
	}
	ele := r.EleicaoCodigo
	if ele == "" {
		ele = "?"
	}
	turno := r.Turno
	if turno == "" {
		turno = "1"
	}
	idg := r.IDGeracao
	if idg == "" {
		idg = "?"
	}
	return fmt.Sprintf("%s %s — eleição %s · turno %s\nvotos válidos: %s · nominais: %s\nbrancos: %s · nulos: %s\nseções instanciadas: %s · ativas: %s\npendentes: %s · dados válidos: %s\ntotalização final: %s · id geração: %s",
		r.Cargo, r.Abrangencia, ele, turno,
		apuracao.GroupInt(r.Votos.Validos), apuracao.GroupInt(r.Votos.ValidosNominais),
		apuracao.GroupInt(r.Votos.Brancos), apuracao.GroupInt(r.Votos.Nulos),
		apuracao.GroupInt(r.Secoes.Instanciadas), apuracao.GroupInt(r.Secoes.Ativas),
		apuracao.GroupInt(r.Secoes.NaoTransmitidas), dados, fim, idg)
}

func rowTip(c apuracao.Candidato) string {
	nome := c.NomeCompleto
	if nome == "" {
		nome = c.Nome
	}
	nasc := ""
	if c.Nascimento != "" {
		nasc = " · nasceu em " + c.Nascimento
	}
	seq := c.Seq
	if seq == "" {
		seq = "?"
	}
	tipo := c.TipoVoto
	if tipo == "" {
		tipo = "Válido"
	}
	colig := c.Coligacao
	if colig == "" {
		colig = c.Partido
	}
	vice := ""
	if c.Vice != "" {
		vice = "\nvice: " + c.Vice
		if c.Foto != "" {
			vice += " · foto: TSE divulga"
		}
	}
	return fmt.Sprintf("%s%s\nseq %s · nº %s · %s\n%s\n%%: %s (exato %s)\n%s votos nominais%s",
		nome, nasc, seq, c.Numero, tipo, colig, pctComma(c.Pct, 2), pctComma(c.PctExato, 6), apuracao.GroupInt(c.Votos), vice)
}

func barCSS(i int, pct float64) map[string]templ.SafeCSSProperty {
	width := strconv.FormatFloat(math.Max(pct, 0.8), 'f', -1, 64) + "%"
	return map[string]templ.SafeCSSProperty{
		"width":      templ.SafeCSSProperty(width),
		"background": templ.SafeCSSProperty(barColor(i)),
	}
}

func avatarCSS(partido, numero string) map[string]templ.SafeCSSProperty {
	return map[string]templ.SafeCSSProperty{
		"background": templ.SafeCSSProperty(partyColor(partido, numero)),
		"color":      "#fff",
	}
}

func barColor(i int) string {
	return barColors[i%len(barColors)]
}

func partyColor(partido, numero string) string {
	upper := strings.ToUpper(partido)
	for i, prefix := range partPrefix {
		if strings.HasPrefix(upper, prefix) {
			return partColors[i]
		}
	}
	n, err := strconv.Atoi(numero)
	if err != nil || n < 0 {
		return partColors[0]
	}
	return partColors[n%len(partColors)]
}

func initials(nome string) string {
	nome = strings.TrimSpace(nome)
	if nome == "" {
		nome = "?"
	}
	fields := strings.Fields(nome)
	if len(fields) > 2 {
		fields = fields[:2]
	}
	var b strings.Builder
	for _, field := range fields {
		r, _ := utf8.DecodeRuneInString(field)
		if r == utf8.RuneError {
			continue
		}
		b.WriteRune(unicode.ToUpper(r))
	}
	return b.String()
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func jsNumber(f float64) string {
	return strings.ReplaceAll(strconv.FormatFloat(f, 'f', -1, 64), ".", ",")
}

func pctComma(f float64, prec int) string {
	return strings.ReplaceAll(strconv.FormatFloat(f, 'f', prec, 64), ".", ",")
}

var (
	zoneOnce sync.Once
	zone     *time.Location
)

func brazilClock(t time.Time) string {
	zoneOnce.Do(func() {
		loc, err := time.LoadLocation("America/Sao_Paulo")
		if err != nil {
			loc = time.FixedZone("BRT", -3*3600)
		}
		zone = loc
	})
	return t.In(zone).Format("15:04:05")
}
