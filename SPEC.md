# SlopApura Specification

This document constrains the 2026 TSE tally board and its same-origin JSON API.

Status: draft
Genre: app

Readers apply BCP 14 (RFC 2119, RFC 8174) to MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY.
That rule applies only when the word is in all capitals.

## Intention

Job: show official TSE partials for the 2026 Brazilian election, plus a conservative local projeção, without an account and without a client-side card script.

Non-goals:

1. Durable history of past polls.
2. User accounts, cast votes, and a second data source.
3. Scraping every UF on a timer outside a board the user opened.
4. Candidate photographs in the HTML.
5. Cargo 8 (Dep. Distrital) on the DF board. Upstream omits it.

Inherited C (cite the file): behavior of [luksamuk/slopapura](https://github.com/luksamuk/slopapura) at `5084310078ce9ea5b77282df63b499c3215853d3`, except the UI technique in TEC-04.

## Technique

| ID | Input | Rule | Output |
|----|-------|------|--------|
| TEC-01 | cargo, turno, UF | Map to the official file URL under `https://resultados.tse.jus.br/oficial`. Federal codes: 1º turno `6257`, 2º turno `6258`. Estadual codes: 1º turno `6259`, 2º turno `6260`. Presidente (cargo 1) uses the federal code. Cargo 3, 5, 6, 7, 8 use the estadual code. | One JSON URL |
| TEC-02 | HTTP response | Keep a 200 body for 270s. Send `If-None-Match` and `If-Modified-Since`. A 304 refreshes the timestamp and keeps the body and the validators. A 404 stays in the cache for 30s. The cache skips every other HTTP error. Body cap is 32 MiB. Decode with `UseNumber`. | Cached document, absent document, and the error return |
| TEC-03 | TSE object | Flatten `carg[].agr[].par[].cand[]` only when the object has `carg` and lacks `cand`. Copy each candidate before adding `cc`. Sort by votos descending, then cut to `limite`, then project. | One `Result`, absent when no candidate remains |
| TEC-04 | Page query | Render the whole document on the server. The form is a normal GET. htmx swaps `#live` and leaves the form in place. Auto poll is 300s and uses a URL the server built. | HTML document |
| TEC-05 | Several UF and cargo pairs | Fetch in submission order on the Control pool. Drop a card on HTTP 404. Drop a card on transport failure. An empty board is still a successful board. | `Painel` |
| TEC-06 | Listen address, stamp | With an address, serve that address and skip the window. With no address and no stamp, bind `127.0.0.1:8892`. With a stamp and no address, open the eletrocromo window. | HTTP server |

## Tooling

| TEC | Tool | Relation | We do not | Cite |
|-----|------|----------|-----------|------|
| TEC-01 | `net/http` plus lewkit `httpclient` | adopt | Invent a second HTTP stack | lewtec/lewkit `x/driver/httpclient` |
| TEC-02 | process memory map | implement | Store the cache on disk | none |
| TEC-03 | `internal/apuracao` | implement | Recompute the projeção in the browser | none |
| TEC-04 | templ, htmx 4, daisyUI, Tailwind | adopt | Ship a `function card` renderer | lewtec/lewkit `x/http/asset` |
| TEC-05 | `taskgroup.Each` with `PoolKind: Control` | adopt | Run the fanout on the Internet pool | lewtec/lewkit `x/taskgroup` |
| TEC-06 | `x/app`, `x/entry`, `eletrocromo.json` entry `go:.` | adopt | A separate UI process on port 8891 | `eletrocromo.json` |

| Cell | Pick | C or D | Implements | Cite if C |
|------|------|--------|------------|-----------|
| Language | Go | C | the program | eletrocromo entry `go:.` |
| Runtime | Go 1.27 | C | module line `go 1.27.0` | `go.mod` |
| Persistence | process memory | C | TEC-02 | no account store |
| UI | server HTML | C | TEC-04 | `page.templ` |
| Packaging | eletrocromo app | C | TEC-06 | `eletrocromo.json` |
| Identity | none | C | public board | no login surface |
| Host OS | Linux. Android after the stamp. | C | `net.Listen("tcp4")` and `app.App.Run` | lewkit `x/app` |

## Terminology

| Concept | Approved | Banned |
|---------|----------|--------|
| Official count | tally | count, results dump |
| Screen of cards | board | dashboard |
| Office on the ballot | cargo | race, office |
| Election round | turno | round, turn |
| Local math note | projeção | forecast, prediction |
| Repeated query key | last value | first value |

## Types

| Entity | Kind | Identity authority | A/B rels `(min,max)` | Root | Invariant IDs |
|--------|------|--------------------|----------------------|------|---------------|
| CachedFile | entity | request URL, this process | yields Result: CachedFile (0,1), Result (1,1) | yes | INV-01, INV-02 |
| Result | value | none | contains Candidate: Result (0,*), Candidate (1,1) | no | INV-03 |
| Candidate | value | none | member of one Result | no | INV-03 |
| Board | value | none | lists Result: Board (0,*), Result (0,1) | no | INV-04 |
| PageQuery | value | none | selects one Board | no | INV-05 |

| Rel | A | B | A card | B card | Id | Owner | Attrs | Ban |
|-----|---|---|--------|--------|----|-------|-------|-----|
| yields | CachedFile | Result | (0,1) | (1,1) | yes | memory slot | fetched-at | one cache per URL |
| contains | Result | Candidate | (0,*) | (1,1) | yes | sort order | votos | no edit of the cache |
| lists | Board | Result | (0,*) | (0,1) | no | submit index | cargo, UF | no cargo 8 |
| selects | PageQuery | Board | (1,1) | (0,1) | no | this request | five query keys | page skips avisos |

CachedFile lifecycle:

```
Absent —200→ Fresh —270s→ Stale —304→ Fresh
Absent —404→ Negative —30s→ Stale
```

Each arrow keeps INV-01 and INV-02. A transport error does not enter Fresh. A transport error does not enter Negative.

## Invariants

| ID | Predicate | On | Forbidden bypass |
|----|-----------|----|------------------|
| INV-01 | A stored 200 stays reusable for 270s. A stored 404 stays reusable for 30s. A 304 does not replace the body. | CachedFile | A test that calls the live TSE host |
| INV-02 | Callers copy a candidate map before they set `cc`. The cached object stays unchanged. | CachedFile | In-place edit of `cand` |
| INV-03 | Sort by votos, then apply `limite`, then run the projeção on that prefix. `eleito` is `e` equal to `s`. `validos_nominais` comes only from nested `v.vnom`. | Result | Project the full list, then cut |
| INV-04 | UF `br` requests cargo 1. Any other UF requests cargos 3, 5, 6, and 7, in that order. The board omits a failed card. Order follows submission. | Board | Adding cargo 8 for DF |
| INV-05 | Page defaults are turno 2, UF `mg`, Pres. BR on, auto on. Only the last value `0` turns `br` off. Only the last value `0` turns `auto` off. Unknown UF falls back to `mg`. | PageQuery | `Query().Get`, which returns the first value |

## Errors

| Public operation | Bad input | One reaction |
|------------------|-----------|--------------|
| `GET /api/resultados` | cargo outside {1, 3, 5, 6, 7, 8} | 400 `código de cargo inválido` |
| `GET /api/resultados` | turno outside {1, 2} | 400 `turno inválido` |
| `GET /api/resultados` | limite outside 1..60 | 400 `limite inválido` |
| `GET /api/resultados` | file not published (nil document) | 404 `arquivo ainda não publicado pelo TSE (apuração começa após 17h)` |
| `GET /api/resultados` | transport error | 502 whose text starts with `erro ao consultar TSE:` |
| `GET /api/resultados` | a body that is not a JSON object | 502 whose text starts with `erro ao consultar TSE:` |
| `GET /api/config` | transport error | 502 whose text starts with `config TSE indisponível:` |
| `GET /api/config` | file not published | 200 body `null` |
| `GET /api/painel` | turno outside {1, 2} | 400 `turno inválido` |
| `GET /api/painel` | limite outside 1..60 | 400 `limite inválido` |
| `GET /api/painel` | nil TSE client | 502 |
| `GET /api/painel` | every card missing | 200 with `resultados: []` and the empty-board aviso |
| `GET /api/demo` | fixture missing at build time | 404 `demo_fixture.json não encontrado` |
| `GET /` | snapshot error | 200 HTML, status text `erro:`, note about files after 17h |
| `GET /` | render failure | 500 plain text |

API turno defaults to 1. Page turno defaults to 2. API `uf` defaults to `br`. An empty `uf` becomes `br`. API `ufs` defaults to `br,mg` only when the key is absent. A present empty `ufs` selects no UF.

`mock` is true for `1`, `t`, `true`, `y`, `yes`, and `on`, compared in lower case. Mock clones the fixture into five cards and ignores `limite` and `turno` for those numbers.

## Actors

| Actor | Obligations |
|-------|-------------|
| Reader | Opens the page. May turn filters. Does not authenticate. |
| TSE | Publishes the JSON files. Remains the authority for official numbers. |
| Process | Caches, projects, and renders. Drops the cache when the process exits. |

## Capabilities

| ID | Actor | Sea-level goal |
|----|-------|----------------|
| CAP-01 | Reader | See the current board for the chosen UF and turno |
| CAP-02 | Reader | Keep a bookmark of those filters |
| CAP-03 | Reader | Read the same numbers as JSON |
| CAP-04 | Process | Avoid a second fetch inside the TTL |

## Public contract

The page and the API share one origin. Paths:

- `GET /` HTML, `Cache-Control: no-store`, `lang=pt-BR`.
- `GET /api/health` `{ok, time}` with `time` in RFC3339 UTC.
- `GET /api/config` the raw config object.
- `GET /api/resultados` one `Result` plus `poll_ts`.
- `GET /api/demo` the fixture at limite 12. No `poll_ts`.
- `GET /api/painel` `{poll_ts, resultados, avisos}`.

Error JSON is `{"detail": "<sentence>"}` with HTML escaping off.

The page does not print `avisos`. On a successful snapshot the note is the fixed Portuguese sentence about official percents and the local projeção, including the waiting board.

Foto URLs stay in JSON. The HTML shows initials. The HTML does not name `divulgacandcontas`.

## Quality

| Concern | Measure, or why it cannot happen |
|---------|----------------------------------|
| Security | No account and no secret. HTML goes through templ escaping. API JSON uses `SetEscapeHTML(false)`. Each TSE body stops at 32 MiB. |
| Identity | No login. The reader is anonymous. A second identity store has no job. |
| Persistence | The cache lives in the process. A restart fetches again. There is no disk tally. |

## Security

In scope: response encoding, body size, same origin, and the User-Agent `SlopApura/0.2 (projeto local, cache >=270s, sem chave, sem cadastro)`.

Why a broader claim is false: the process still depends on TSE availability and on the TLS stack of the host.

Residual risk: the projeção can disagree with the later official result. The badge text says it is local.

## Success

- [ ] `GET /?mock=1` contains `LULA`, `57.259.504`, and `D'AVILA` (escaped as `&#39;` in HTML) and does not contain `function card`.
- [ ] `GET /api/resultados` for cargo 2 returns 400 and does not call TSE.
- [ ] A second read of a fresh 200 does not download the body again inside 270s.
- [ ] DF board paths omit `c0008`.
- [ ] With auto on, `#live` carries `every 300s` and `outerMorph`. With auto off, that trigger is absent.

## Later work

1. Stop the process after the 2026 election. The upstream product is temporary.
2. Do not add a history database under this job.

## Assumptions

| ID | Fact | If false |
|----|------|----------|
| AS-01 | The 2026 codes stay 6257, 6258, 6259, and 6260. | TEC-01 fetches the wrong file. |
| AS-02 | Brazil stays at UTC−3 for this election. | The `FixedZone` fallback shows the wrong clock when zone data is missing. |
| AS-03 | htmx 4 treats `outerMorph` as a morph swap. | `#live` would nest inside `#live`. |
| AS-04 | The vendored daisyUI and htmx files answer on `/__lewkit__/`. | The page has no style and no swap. |

## Decision history

- ADR-0001: Render the board with templ and swap it with htmx. Rejected: a browser `function card` that wrote `innerHTML`.
- ADR-0002: One process serves the page and the API. Rejected: a fixed API port 8891 beside a separate page server.
- ADR-0003: Painel children use the Control pool. Rejected: Internet children, which deadlock behind the HTTP progress slot (limit 4).
- ADR-0004: Group projeção integers with pt-BR dots. Rejected: copying Python `:,.0f` commas into Portuguese sentences.
