package salesnote_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AtmosferaSoltec/atm-documents-api/internal/activity"
	"github.com/AtmosferaSoltec/atm-documents-api/internal/auth"
	"github.com/AtmosferaSoltec/atm-documents-api/internal/platform/httpserver"
	"github.com/AtmosferaSoltec/atm-documents-api/internal/renderer"
	"github.com/AtmosferaSoltec/atm-documents-api/internal/salesnote"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const issuer = "atm-auth-test"

type env struct {
	engine   *gin.Engine
	key      *rsa.PrivateKey
	recorder *activity.Recorder
}

func newEnv(t *testing.T) env {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	verifier, err := auth.New(base64.StdEncoding.EncodeToString(pemKey), issuer, []string{"lavendimia"})
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(httpserver.RequestID())
	service := salesnote.NewService(renderer.New(), 2)
	recorder := activity.New()
	handler := salesnote.NewHandler(service, recorder, 10*time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	salesnote.RegisterRoutes(engine.Group("/v1", verifier.Service()), handler)
	return env{engine: engine, key: key, recorder: recorder}
}

func (e env) token(t *testing.T, role, system string, expiresIn time.Duration, signer *rsa.PrivateKey) string {
	t.Helper()
	claims := jwt.MapClaims{
		"iss": issuer, "sub": "system:" + system, "role": role, "system": system,
		"iat": time.Now().Unix(), "exp": time.Now().Add(expiresIn).Unix(),
	}
	if signer == nil {
		signer = e.key
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(signer)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

const validBody = `{
  "format": "a4", "code": "NV-00000021", "issued_at": "2026-10-13T15:45:00Z",
  "company": {"name": "La Vendimia", "legal_name": "Bodegas La Vendimia S.A.C.", "tax_id": "20601234567", "address": "Ica"},
  "customer": {"name": "María Peña", "document": "DNI 45872310", "address": ""},
  "items": [{"name": "Vino", "presentation": "Botella", "quantity": 2, "unit_price": 10, "line_total": 20}],
  "taxable_amount": 16.95, "tax_amount": 3.05, "total": 20, "sold_by": "Carlos"
}`

func (e env) post(t *testing.T, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/documents/sales-note", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.engine.ServeHTTP(rec, req)
	return rec
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body httpserver.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not an error envelope: %s", rec.Body.String())
	}
	return body.Error.Code
}

func TestRendersPDFForAllowedSystem(t *testing.T) {
	e := newEnv(t)
	rec := e.post(t, e.token(t, "service", "lavendimia", time.Minute, nil), validBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Fatalf("content type = %q", ct)
	}
	if !strings.HasPrefix(rec.Body.String(), "%PDF-") {
		t.Fatal("body is not a PDF")
	}
	if cd := rec.Header().Get("Content-Disposition"); cd != `inline; filename="NV-00000021.pdf"` {
		t.Fatalf("content disposition = %q", cd)
	}
	if rec.Header().Get("X-Request-ID") == "" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing request id or no-store header")
	}
}

func TestRejectsMissingOrBadTokens(t *testing.T) {
	e := newEnv(t)
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"no token":     "",
		"garbage":      "not-a-jwt",
		"wrong signer": e.token(t, "service", "lavendimia", time.Minute, other),
		"expired":      e.token(t, "service", "lavendimia", -time.Minute, nil),
	}
	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			rec := e.post(t, token, validBody)
			if rec.Code != http.StatusUnauthorized || errorCode(t, rec) != "unauthorized" {
				t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestForbidsValidTokensThatAreNotAllowedSystems(t *testing.T) {
	e := newEnv(t)
	cases := map[string]string{
		"user role":          e.token(t, "user", "lavendimia", time.Minute, nil),
		"superadmin":         e.token(t, "superadmin", "", time.Minute, nil),
		"system not allowed": e.token(t, "service", "olympus", time.Minute, nil),
	}
	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			rec := e.post(t, token, validBody)
			if rec.Code != http.StatusForbidden || errorCode(t, rec) != "forbidden" {
				t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestRecordsActivityWithoutContent(t *testing.T) {
	e := newEnv(t)
	token := e.token(t, "service", "lavendimia", time.Minute, nil)
	e.post(t, token, validBody)
	e.post(t, token, strings.Replace(validBody, `"a4"`, `"a3"`, 1))
	e.post(t, token, "nope")

	s := e.recorder.Snapshot()
	if s.Totals.OK != 1 || s.Totals.Invalid != 2 {
		t.Fatalf("totals = %+v", s.Totals)
	}
	newest, oldest := s.Recent[0], s.Recent[2]
	if oldest.Outcome != activity.OutcomeOK || oldest.System != "lavendimia" || oldest.Format != "a4" || oldest.Items != 1 || oldest.Bytes == 0 {
		t.Fatalf("ok event = %+v", oldest)
	}
	if newest.Format != "" || newest.Items != 0 {
		t.Fatalf("a body that is not JSON must record no format or items: %+v", newest)
	}
	if s.Recent[1].Format != "" {
		t.Fatalf("an unknown format must not be recorded: %+v", s.Recent[1])
	}
}

func TestRejectsBadPayloads(t *testing.T) {
	e := newEnv(t)
	token := e.token(t, "service", "lavendimia", time.Minute, nil)
	cases := map[string]struct {
		body   string
		status int
		code   string
	}{
		"not json":        {"nope", 400, "invalid_request"},
		"unknown field":   {strings.Replace(validBody, `"format"`, `"colour": "red", "format"`, 1), 400, "invalid_request"},
		"trailing data":   {validBody + `{}`, 400, "invalid_request"},
		"bad format":      {strings.Replace(validBody, `"a4"`, `"a3"`, 1), 422, "invalid_payload"},
		"empty items":     {strings.Replace(validBody, `"items": [{"name": "Vino", "presentation": "Botella", "quantity": 2, "unit_price": 10, "line_total": 20}]`, `"items": []`, 1), 422, "invalid_payload"},
		"bad logo base64": {strings.Replace(validBody, `"address": "Ica"`, `"address": "Ica", "logo_base64": "%%%"`, 1), 422, "invalid_payload"},
		"logo not image":  {strings.Replace(validBody, `"address": "Ica"`, `"address": "Ica", "logo_base64": "`+base64.StdEncoding.EncodeToString([]byte("hello"))+`"`, 1), 422, "invalid_payload"},
		"too large":       {`{"code": "` + strings.Repeat("x", salesnote.MaxBodyBytes) + `"}`, 413, "payload_too_large"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rec := e.post(t, token, tc.body)
			if rec.Code != tc.status || errorCode(t, rec) != tc.code {
				t.Fatalf("status = %d code = %s, want %d %s", rec.Code, rec.Body.String(), tc.status, tc.code)
			}
		})
	}
}

func TestRendersTicketFormat(t *testing.T) {
	e := newEnv(t)
	rec := e.post(t, e.token(t, "service", "lavendimia", time.Minute, nil), strings.Replace(validBody, `"a4"`, `"ticket"`, 1))
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Body.String(), "%PDF-") {
		t.Fatalf("status = %d: %.200s", rec.Code, rec.Body.String())
	}
}
