# ATM-Documents-API

Generates PDF documents for the ATM systems. It holds no business data and no database: callers send everything the document needs, and the service answers with the PDF.

Today it renders one document, the **internal sales note** (`NOTA DE VENTA`) that La Vendimia prints, in two formats:

| Format | Paper | Notes |
|---|---|---|
| `a4` | A4, paginated | Header table repeats on every page; footer shows "Page n of N" |
| `ticket` | 80 mm thermal strip | Black only, no boxes; height is measured so the ticket is always one piece |

Boletas, facturas and guías de remisión are **not** produced here: they come from Nubefact through ATM-Nubefact-API.

## Stack

Go, Gin, `go-pdf/fpdf` (native PDF, no browser), `golang-jwt/jwt` (RS256), `godotenv`. Text uses the built-in Helvetica (Windows-1252), so no font file ships with the service. Characters outside Windows-1252 print as a replacement character.

## Layout

```
cmd/api/                  entry point: wiring only
internal/config/          environment configuration
internal/auth/            service-token verification (RS256, public key)
internal/platform/httpserver/   error envelope, X-Request-ID
internal/salesnote/       feature: model + validation, service, handler, formats, amount in words
internal/renderer/        the ONLY package importing fpdf: A4 and ticket layouts
```

## Configuration

See [.env.example](.env.example). `AUTH_PUBLIC_KEY_BASE64` and `AUTH_ISSUER` are required; they are the same values storage-api uses.

| Variable | Default | Meaning |
|---|---|---|
| `APP_ENV` | `development` | `development` or `production` |
| `HTTP_PORT` | `8085` | Listen port |
| `AUTH_PUBLIC_KEY_BASE64` | | Base64 PEM public key of atm-auth-api |
| `AUTH_ISSUER` | | Must equal atm-auth-api `JWT_ISSUER` |
| `ALLOWED_SYSTEMS` | `lavendimia` | System codes whose service token may call the service |
| `MAX_CONCURRENT_RENDERS` | `4` | Documents rendered at once; others wait, then get `503 service_busy` |
| `RENDER_TIMEOUT` | `15s` | Upper bound for one request |

```
cp .env.example .env   # fill AUTH_PUBLIC_KEY_BASE64
go run ./cmd/api serve
```

## Authentication

Every `/v1` route needs `Authorization: Bearer <token>`, RS256, from the configured issuer and unexpired. The service validates tokens locally and never calls auth-api.

| Routes | Token | Otherwise |
|---|---|---|
| `/v1/documents/*` | Service token (`role: service`) of a system listed in `ALLOWED_SYSTEMS`. A backend gets it from atm-auth-api `POST /v1/token` with its client secret | 403 for a user or superadmin token, or a system not listed |
| `/v1/admin/*` | Superadmin token (`role: superadmin`): the console | 403 for a service or user token |

A missing, malformed, expired or wrongly signed token is always 401.

## API

| Method | Path | Description |
|---|---|---|
| GET | `/health` | Liveness |
| GET | `/ready` | Readiness (no external dependencies: ready when running) |
| POST | `/v1/documents/sales-note` | Render a sales note; responds `application/pdf` (service token) |
| GET | `/v1/admin/templates` | Catalog of document types, their formats and limits, and the service settings (superadmin) |
| GET | `/v1/admin/templates/{code}/preview?format=a4\|ticket` | The template rendered with fictional data, as `application/pdf` (superadmin) |
| GET | `/v1/admin/activity` | What the service rendered since it started (superadmin) |

### `POST /v1/documents/sales-note`

```json
{
  "format": "a4",
  "code": "NV-00000021",
  "issued_at": "2026-10-13T15:45:00Z",
  "company": {
    "name": "La Vendimia",
    "legal_name": "Bodegas La Vendimia S.A.C.",
    "tax_id": "20601234567",
    "address": "Av. Los Viñedos 123, Ica",
    "phone": "956 123 456",
    "email": "ventas@lavendimia.pe",
    "note": "ENVÍOS A TODO EL PERÚ\nPedidos al 956 123 456",
    "logo_base64": "<PNG or JPEG, standard base64, optional>"
  },
  "customer": { "name": "María Peña", "document": "DNI 45872310", "address": "Lima" },
  "items": [
    { "name": "Vino Tinto", "presentation": "Botella 750 ml", "quantity": 2, "unit_price": 68, "line_total": 136 }
  ],
  "taxable_amount": 115.25,
  "tax_amount": 20.75,
  "total": 136,
  "sold_by": "Carlos Ramírez",
  "is_voided": false,
  "is_test": false
}
```

- Optional: `phone`, `email`, `note`, `logo_base64`, `customer.document`, `customer.address`, `is_voided`, `is_test`.
- `company.note`: the first line prints in bold; each further line below it.
- `customer.document` is one text such as `DNI 45872310`; the first word becomes the label.
- `is_voided` prints the note marked "ANULADA". `is_test` adds the "SIN VALOR" stamp.
- Dates print in Lima time (UTC-5).
- Unknown JSON fields are rejected.

Limits: body 4 MiB; logo 2 MiB and 8 megapixels, PNG or JPEG only; at most 500 items for `a4` (20 pages) and 150 for `ticket`; money values up to 99,999,999.99; the request must finish within `RENDER_TIMEOUT`.

Response headers: `Content-Type: application/pdf`, `Content-Disposition: inline; filename="<code>.pdf"`, `Cache-Control: no-store`, `X-Request-ID`.

### Errors

`{"error": {"code": "...", "message": "..."}}`

| Status | Code | When |
|---|---|---|
| 400 | `invalid_request` | Not JSON, unknown field, trailing data |
| 401 | `unauthorized` | Missing, invalid, expired, wrong role or system |
| 413 | `payload_too_large` | Body over 4 MiB |
| 422 | `invalid_payload` | A field failed validation; the message names it |
| 500 | `render_failed` | Unexpected rendering error |
| 503 | `service_busy` | No render slot freed up before the timeout |

### Management routes

They exist for the console (`Documentos > Plantillas` and `Actividad`).

- **Catalog**: templates are code in this repository, so the catalog is a fixed list; it cannot be edited through the API. Each entry has its code, name, endpoint, paper formats and limits. The response also carries the service settings (environment, allowed systems, render slots, timeout).
- **Preview**: renders the template with a built-in fictional note, so the console never sends or stores a real document. Previews do not count as activity. Errors: `404 template_not_found`, `422 invalid_format`.
- **Activity**: in-memory counters since the process started: totals by outcome (`ok`, `invalid`, `busy`, `error`), the same by system and by format, the average duration of successful renders, and the last 50 requests. The counters reset on every restart and deploy. Each event holds only time, system, format, item count, size, duration and outcome.

The console reaches the service on its own origin under `/documents-api/` (host nginx forwards it to `127.0.0.1:3133`), so no CORS is needed. See [deploy/host-nginx/console-documents-api.location](deploy/host-nginx/console-documents-api.location).

## Privacy

Payloads hold personal and financial data. The service logs only request id, system, format, item count, size and duration. It never logs payloads, tokens or documents, and stores nothing. The activity log keeps the same metadata in memory only.

## Quality

```
gofmt -l .          # no output
go vet ./...
go test -race ./...
```

Layout is defined in code, so a changed template is checked by eye. Write sample PDFs and convert them to images:

```
PREVIEW_DIR=/tmp/preview go test ./internal/renderer -run TestPreview
pdftoppm -r 100 -png /tmp/preview/a4-short.pdf /tmp/preview/a4-short
```

On macOS, `qlmanage -t -s 1100 -o <dir> file.pdf` renders with Apple's engine; poppler (`pdftoppm`) draws the non-embedded Helvetica-Bold without bold, which is a viewer artifact, not a defect of the PDF.

## Deployment

Pushing to the `production` branch (or running the workflow by hand) runs `.github/workflows/deploy.yml`: `gofmt`, `vet`, `test -race`, build; then the image is built once and published to `ghcr.io/atmosferasoltec/atm-documents-api` (tags `latest` and `sha-<commit>`); finally the compose file is copied to the VPS and `docker compose up -d` runs. The deploy waits until the container is healthy and fails with the last logs if it is not. Re-running the workflow on an older commit rolls back.

Repository secrets: `SSH_HOST`, `SSH_USER`, `SSH_KEY`.

On the server (already prepared):

| What | Where |
|---|---|
| Stack | `/opt/documents-api/docker-compose.yml` (copied by CI, never edited by hand) |
| Environment | `/opt/documents-api/.env` (`deploy:deploy`, mode 600; only on the server) |
| Container | `documents-api`, 384 MiB, read-only filesystem, no capabilities |
| Port | `127.0.0.1:3133` -> container `8085` |
| Public site | `https://documents.atmosferast.com` via host nginx, wildcard certificate `/etc/ssl/atmosferast/origin.pem` |
| Internal URL | `http://documents-api:8085` on the `atm-network` Docker network |

The nginx site is versioned in [deploy/host-nginx/documents.atmosferast.com](deploy/host-nginx/documents.atmosferast.com); it is installed once in `/etc/nginx/sites-available` and linked from `sites-enabled`. The console needs one more block, [console-documents-api.location](deploy/host-nginx/console-documents-api.location), added by hand inside the `server` block of `/etc/nginx/sites-available/console.atmosferast.com` before `location /` (then `nginx -t && systemctl reload nginx`). The production `.env` carries the same `AUTH_PUBLIC_KEY_BASE64` and `AUTH_ISSUER` as storage-api. To let another system call this service, add its code to `ALLOWED_SYSTEMS` in that file and restart the container.

Backends on the same Docker network should call `http://documents-api:8085`; the public domain exists for tools and external callers.

## Not implemented yet

- OpenAPI contract.
- La Vendimia is not connected yet; its own `@react-pdf` generation still runs.
