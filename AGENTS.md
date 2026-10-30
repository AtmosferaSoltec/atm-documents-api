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
- PDF rendering is native Go. NO headless browser (no Chromium, chromedp, or HTML-to-PDF): it keeps the image small (about 35 MB) and removes the SSRF and file-read surface. Library: `go-pdf/fpdf` (maintained fork of gofpdf; the original is archived), used directly because the templates need exact control (rounded boxes, bands, absolute footers, custom page sizes). Other libraries are added only when needed (for example `boombuler/barcode` for QR and Code128 once a template needs them); check the current maintenance status and version first, and tell the user why.
- PDF rendering is isolated behind the `Renderer` interface of the feature, so the engine can be replaced. Nothing outside `internal/renderer/` imports the PDF library.
- Fonts: the built-in Helvetica (Windows-1252), so no font file ships. It covers Spanish accents, `ñ`, `¡`, `¿` and `·`; other characters print as a replacement. If a template ever needs wider Unicode coverage, embed a TTF with `go:embed` instead of relying on system fonts.
- Paper formats: A4 and the 80 mm ticket. A template declares its formats; callers choose only among them, never an arbitrary size. A 58 mm ticket is added when a client needs it.
- Layout is drawn in points. Keep measures as named constants and measure text with the canvas helpers before drawing, so a long value wraps instead of overflowing.
- PostgreSQL (pgx + goose migrations in `migrations/`) only if templates, versions, or generation history need to be persisted. Do not add it before it is needed.

## Architecture: package by feature, lightweight hexagonal

```
cmd/api/              entry point: wiring only, no business logic
migrations/           numbered .sql files (only if a database is used)
internal/
  config/             environment configuration
  auth/               service-token verification middleware
  platform/httpserver/   error envelope, X-Request-ID
  salesnote/          feature: internal sales note (model, service, handler, formats, amount in words)
  renderer/           PDF layouts (A4, ticket); the ONLY place importing the PDF library
```

One feature per document type (`salesnote/` today; for example `contract/` later). Each defines its own `Renderer` interface and its own endpoint `POST /v1/documents/<type>`. Do not build a generic template engine until a third document type shows what is really shared.

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
- Templates are trusted code owned by this repo, never accepted from callers. Never accept caller-supplied layouts, fonts, or file paths.
- Do not fetch remote resources while rendering. Logos and images arrive as bytes or as an object reference resolved through ATM-Storage-API, never as an arbitrary URL. Validate image type and size before decoding.
- QR and barcode content is data from the payload, encoded by the renderer. Links in a document are written as plain text or PDF link annotations; validate their scheme (`https` only).
- Enforce limits: payload size, number of items/rows, page count, image size, and render timeout.
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
- Layout is defined in code, so every new or changed template is checked visually: render a sample PDF, convert it to an image (for example with `pdftoppm`), and look at it before declaring the template done. Keep sample payloads next to the template tests.
- Before finishing any task, all of these must pass: `gofmt -l .` (no output), `go vet ./...`, and `go test ./... -race`.
- Fix the cause of a failing test. Never delete or weaken a test to make it pass.

## Working agreement

- Make small, focused changes. Do not refactor unrelated code.
- Do not modify other services as part of an unrequested refactor.
- Never edit a migration that was already applied. Add a new one.
- When a request is ambiguous or touches a security decision, ask before building.
- Update `README.md` when behavior, configuration, or routes change.
