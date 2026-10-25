# SlopApura

Go eletrocromo app. Server-rendered 2026 TSE tally board. Same-origin JSON API.

## Commands

- Generate: `go tool templ generate`
- Test: `CGO_ENABLED=0 go test ./...`
- Scoped test: `CGO_ENABLED=0 go test ./internal/tse/`
- Serve: `CGO_ENABLED=0 go run . -listen 127.0.0.1:8892`
- Build: `CGO_ENABLED=0 go build -o bin/slopapura .`

Go 1.27.2. `GOTOOLCHAIN=local`. `CGO_ENABLED=0`. Queue `go test` with Task Spooler: `ts -L 'slopapura:test' -- go test ./...`.

## Layout

- `page.templ` — HTML. Regenerate `page_templ.go`. Do not edit the generated file.
- `server.go` — routes and query parsing.
- `view.go` — page model.
- `internal/tse` — cache and TSE URLs.
- `internal/apuracao` — normalize and project.
- `internal/panel` — multi-card fetch.

## Boundaries

**Always**

- Run `CGO_ENABLED=0 go test ./...` before you claim a behavior change.
- Take the last repeated query value. A hidden `br=0` precedes the checkbox.
- Keep painel fanout on `taskgroup.Control`. Close each TSE body.
- Keep the page in templ. htmx swaps `#live` with `outerMorph`.

**Ask first**

- Add a dependency.
- Call the live TSE host from a test.
- Change a JSON field name.
- Change an HTTP status.
- Change a Portuguese error sentence.

**Never**

- Render candidate photos. Initials only. The JSON may still carry the foto URL.
- Request cargo 8 for DF.
- Put `/api` behind an SPA fallback.
- Hand-edit `page_templ.go`.
- Scan ports.

## More context

- [`SPEC.md`](SPEC.md) — tally rules, cache TTL, and the HTTP contract
- [`README.md`](README.md) — when you change how a person starts the board
