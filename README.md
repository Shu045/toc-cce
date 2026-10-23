# Will it ever stop? Halting problem simulator (Next.js + Go)

- **Frontend:** Next.js 15 (App Router) + React 19 + TypeScript. Only UI and `fetch` calls.
- **Backend:** Go 1.22, standard library only (`net/http`, `go/parser`, `go/ast`). It parses and runs the code.

```
browser ──► Next.js (:3000) ──/api/* rewrite──► Go net/http server (:8080)
```

The browser only talks to Next.js. `frontend/next.config.mjs` proxies `/api/*` to the Go server, so there are no CORS problems in development or production.

## Run

Terminal 1 (backend):
```bash
cd backend
go run ./cmd/server          # PORT=8080 by default
```
Terminal 2 (frontend):
```bash
cd frontend
npm install
npm run dev                  # http://localhost:3000
```
Or use `make backend` and `make frontend`. Run the backend tests with `make test` (or `cd backend && go vet ./... && go test ./...`).

Environment variables
| Variable | Where | Default | Purpose |
|---|---|---|---|
| `PORT` | Go | `8080` | Listen port |
| `CORS_ORIGIN` | Go | empty | Only needed if the browser calls Go directly instead of through the Next.js proxy |
| `GO_API_URL` | Next.js | `http://localhost:8080` | Where the `/api` proxy sends requests |

## API

All responses are JSON. Errors look like `{"error":{"message":"...","line":3}}`.

| Method and path | Body | Result |
|---|---|---|
| `GET /api/health` | none | `{"status":"ok"}` |
| `GET /api/programs` | none | the five preset programs |
| `POST /api/simulate` | `{"program":"collatz","start":27,"limit":200}` | a run result |
| `POST /api/check` | `{"code":"x := 5\nfor x > 0 {\n\tx--\n}","limit":1000}` | a run result, or `422` with a line-numbered message |

```bash
curl -s localhost:8080/api/check -H 'Content-Type: application/json' \
  -d '{"code":"a, b := 48, 18\nfor b != 0 {\n\ta, b = b, a%b\n}","limit":1000}'
```

Run result fields: `verdict` (`halts`, `loops`, `unknown`, `crash`), `steps`, `limit`, `limitAdjusted`, `final`, `message`, `loopLine`, `firstSeen`, `repeatedAt`, `state`, `trail`, `elided`, `notes`.

Status codes: `200` ok, `400` bad JSON or unknown field, `404` unknown route or program, `413` body over 32 KB, `415` not `application/json`, `422` Go code can't be parsed, isn't supported, or has a name or type error, `500` internal error, `503` request took over 5 s.

## Writing code for tab 3

Paste Go in any of three forms. Statements alone are wrapped in `func main()` for you:

```go
n := 27
for n != 1 {
	if n%2 == 0 {
		n /= 2
	} else {
		n = 3*n + 1
	}
}
```
A `func main() { ... }`, or a full `package main` program, also works.

Supported: `int`, `int8`–`int64`, `uint8`–`uint32`, `float64`, `bool`; `:=` and `var`; arithmetic, comparisons, `&&`, `||`, `!`, bit operators and shifts; `if`/`else`; every form of `for`, including `for i := range 10`; `break`, `continue`, `return`; `min`, `max`, number conversions; `math.Abs/Sqrt/Floor/Ceil/Trunc/Round/Pow/Mod/Max/Min` and `math.MaxInt64`-style constants. `fmt.Print*` calls are accepted and ignored.
Not supported: other functions, slices, maps, strings, structs, pointers, goroutines, `switch`, other imports.

Go integers wrap around exactly like real Go (`var x uint8 = 250; for x != 5 { x++ }` halts after 11 steps). Name and type mistakes (for example adding an `int` to a `float64`) are reported with a line number when that line runs.

## Layout
```
backend/
  cmd/server/main.go            server, timeouts, graceful shutdown
  internal/api/                 handlers, middleware (recover, log, headers, CORS, timeout)
  internal/engine/
    compile.go                  go/parser, subset validation, loop analysis, static notes
    values.go                   Go types, typed arithmetic with wrap-around, constants
    interp.go                   tree-walking interpreter over go/ast, step limit, repeated-state detection
    programs.go                 tab 1 presets and simulation
    *_test.go                   table-driven tests
frontend/
  app/                          layout, page (tabs), CSS
  components/                   Simulator, Story, LoopChecker, ResultView
  lib/api.ts                    typed fetch client
```

## Why the Go code is interpreted, not compiled
Compiling and running pasted Go on the server would execute arbitrary code. Instead the standard library parses it into an AST (no hand-written tokenizer or grammar), a validator accepts only the subset above, and a small interpreter walks the tree with a 100,000-step limit, a 4,000-character input cap, request-size and time limits. Nothing the user writes is ever compiled or run natively.

## How "loops forever" is decided
At the head of every `for` loop the interpreter records the loop id and every live variable (including hidden range counters). Go is deterministic, so if that state ever repeats, the program repeats forever. If nothing repeats before the step limit, the answer is "can't tell". No program can do better in general (the halting problem).

With fixed-width integers a Go program has only finitely many states, so in principle halting is decidable for it (by the pigeonhole principle); the state space is just far too large (2^64 per variable) to exhaust, which is why "can't tell" still happens.
# toc-cce
