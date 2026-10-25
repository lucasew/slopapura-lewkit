# SlopApura

SlopApura shows the official 2026 TSE tally on one page.

The server renders the page. htmx replaces the board. The JSON API uses the same origin.

This tree is a Go port of [luksamuk/slopapura](https://github.com/luksamuk/slopapura). The license text is in `LICENSE` (WTFNMFPL).

## Run the board

You need Go 1.27. `mise.toml` pins Go 1.27.2, `CGO_ENABLED=0`, and `GOTOOLCHAIN=local`.

1. Run `go tool templ generate` after you edit `page.templ`.
2. Run `CGO_ENABLED=0 go run . -listen 127.0.0.1:8892`.
3. Open `http://127.0.0.1:8892/?mock=1` to load the 2022 fixture.
4. Open `http://127.0.0.1:8892/` to load the live TSE files.

`SLOPAPURA_LISTEN` sets the same address as `-listen`. Use `0.0.0.0:8892` when other machines on the LAN must open the board.

A stamped eletrocromo build opens a window when the listen address is empty. Run `go run github.com/lewtec/lewkit/cmd/lewkit release run --config eletrocromo.json`.

An unstamped `go run .` with an empty listen address binds `127.0.0.1:8892`.

## Run the tests

Run `CGO_ENABLED=0 go test ./...`.

The tests talk to `httptest`. They do not call the live TSE host.

## Terms

| Concept | Term | Do not also call it |
|---------|------|---------------------|
| Official count | tally | count, results dump |
| One screen of cards | board | dashboard |
| Office on the ballot | cargo | race, office |
| Election round | turno | round, turn |
| Local math badge | projeção | forecast, prediction |

`SPEC.md` holds the normative rules. `AGENTS.md` holds the commands for agents.
