# AGENTS.md — ATM-Documents-API

Instructions for every AI agent (and human) working in this repository. Read it fully before changing anything.

## Language rules

- ALL code is written in English: file names, folder names, packages, types, functions, variables, constants, error codes, error messages, log messages, comments, test names, commit messages, routes, and documentation inside the repo.
- Talk to the user in Spanish. Never put Spanish inside the repository, except literal text that must appear in a generated document (for example the labels of an invoice template). That text lives only in template files, never in Go code.
- Repositories follow the pattern `ATM-<Name>-<Type>` (this one is `ATM-Documents-API`).

## What this service does

ATM-Documents-API generates documents for all ATM systems. For now, that means PDFs:

- Receives a template identifier and a JSON payload, and returns a PDF (or stores it and returns a reference).
- Owns the document templates and their versions (invoices, receipts, tickets, reports, certificates, and so on).
- Generation is deterministic: the same template version and the same payload produce the same document.

Out of scope: identity and access (ATM-Auth-API), file storage (ATM-Storage-API), messaging and delivery (ATM-Messaging-API), and electronic invoicing with the tax authority (ATM-Nubefact-API). This service does not hold business data of any system: callers send everything needed in the payload.

Other services:

- Tokens are validated locally with the public key of ATM-Auth-API. This service never calls auth on every request.
- To persist a generated PDF, this service calls ATM-Storage-API. It never talks to R2 or any bucket directly.

## Stack

- Go (version in `go.mod`). Standard library first (`net/http`, `log/slog`).
- PDF rendering is isolated behind an interface so the engine can be replaced. Engine and third-party libraries are added only when the standard library is not reasonable; tell the user why.
- PostgreSQL (pgx + goose migrations in `migrations/`) only if templates, versions, or generation history need to be persisted. Do not add it before it is needed.

## Architecture: package by feature, lightweight hexagonal

```
cmd/api/              entry point: wiring only, no business logic
migrations/           numbered .sql files (only if a database is used)
internal/
  config/             environment configuration
  httpx/              shared HTTP middlewares and helpers
  auth/               token verification middleware
  server/             route registration, wires the features together
  platform/           database connection, clock, id generation
  template/           document templates and versions
  document/           generation use cases (render, store, fetch)
  renderer/           PDF engine adapter(s); the ONLY place importing the PDF library
```

Each feature folder contains:

- `model.go`: domain types and rules. No HTTP or SQL imports.
- `service.go`: use cases. Depends only on interfaces.
- `repository.go`: the interfaces the service needs, defined where they are used.
- `postgres.go`: repository implementation. The ONLY place with SQL for that feature.
- `handler.go`: HTTP handlers. Decode the request, call the service, encode the response.
- `errors.go`: the feature's error codes.
- `*_test.go`: tests next to the code.

Dependency rules (never break them):

1. `handler` depends on `service`, and `service` depends on interfaces. Never the other way around.
2. SQL lives only in `postgres.go`. PDF engine imports live only in `renderer/`. Storage calls live only in a storage client adapter.
3. `model.go` and `service.go` never import `net/http`, `database/sql`, or the PDF library.
4. A feature never imports another feature's `postgres.go` or `handler.go`. If it needs another feature, it depends on a small interface.
5. `cmd/*` and `internal/server` are the only places that construct concrete implementations.

Keep it simple. Do not add layers, generic base types, or abstractions until a second use case needs them.

## Naming

- Files: lowercase, snake_case when needed (`invoice_template.go`).
- Packages: short, lowercase, singular, no underscores.
- Exported identifiers describe intent (`RenderDocument`, not `DoPDF`).
- Tables: plural snake_case. Columns: snake_case. IDs: UUIDs.

## Errors

- Every domain error has a stable snake_case English code, defined in the feature's `errors.go` (for example `template_not_found`, `invalid_payload`, `render_failed`).
- HTTP error body: `{"error": {"code": "invalid_payload", "message": "..."}}`.
- Wrap errors with context (`fmt.Errorf("render document: %w", err)`). Do not log and return the same error.

## Security rules

- Never log JWTs, signed URLs, or the payload of a document. Payloads contain personal and financial data.
- Never commit secrets. `.env`, `*.pem`, and `*.key` are git-ignored.
- Validate the payload against the template's schema before rendering. Reject unknown templates and oversized payloads.
- Templates are trusted code owned by this repo, never accepted from callers. Never render caller-supplied HTML, URLs, or fonts; do not fetch remote resources during rendering (prevents SSRF and file reads).
- Enforce limits: payload size, page count, and render timeout.
- Company IDs are global auth tenant UUIDs, taken from the token and never from the body. Verify authorization on every operation.
- All SQL uses parameters. Never build SQL with string concatenation.
- Separate development and production credentials and databases.

## HTTP conventions

- Business routes live under `/v1`. `/health` and `/ready` are public.
- JSON in; PDF responses use `application/pdf`. Always propagate `X-Request-ID`.
- Keep the OpenAPI contract and `README.md` updated when routes, templates, or configuration change.

## Testing and quality

- Every use case has tests. Services are tested with in-memory fakes of their interfaces. `postgres.go` is tested against a real PostgreSQL, and only against explicit test databases.
- Renderer tests check structure (valid PDF, page count, extracted text), not byte equality.
- Before finishing any task, all of these must pass: `gofmt -l .` (no output), `go vet ./...`, and `go test ./... -race`.
- Fix the cause of a failing test. Never delete or weaken a test to make it pass.

## Working agreement

- Make small, focused changes. Do not refactor unrelated code.
- Do not modify other services as part of an unrequested refactor.
- Never edit a migration that was already applied. Add a new one.
- When a request is ambiguous or touches a security decision, ask before building.
- Update `README.md` when behavior, configuration, or routes change.
