package periodreport_test

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
	"github.com/AtmosferaSoltec/atm-documents-api/internal/periodreport"
	"github.com/AtmosferaSoltec/atm-documents-api/internal/platform/httpserver"
	"github.com/AtmosferaSoltec/atm-documents-api/internal/renderer"
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
	verifier, err := auth.New(base64.StdEncoding.EncodeToString(pemKey), issuer, []string{"orbita"})
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(httpserver.RequestID())
	recorder := activity.New()
	service := periodreport.NewService(renderer.NewPeriodReport(), 2)
	handler := periodreport.NewHandler(service, recorder, 10*time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	periodreport.RegisterRoutes(engine.Group("/v1", verifier.Service()), handler)
	return env{engine: engine, key: key, recorder: recorder}
}

func (e env) token(t *testing.T, role, system string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"iss": issuer, "sub": "system:" + system, "role": role, "system": system,
		"iat": time.Now().Unix(), "exp": time.Now().Add(time.Minute).Unix(),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(e.key)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func (e env) post(token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/documents/period-report", strings.NewReader(body))
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
		t.Fatalf("not an error envelope: %s", rec.Body.String())
	}
	return body.Error.Code
}

const validBody = `{
  "from": "2026-10-01", "to": "2026-10-31",
  "currencies": [{
    "currency": "PEN", "income_total": "3200.00", "expense_total": "1954.20", "balance": "1245.80",
    "expenses": [{"name": "Comida", "total": "700.00", "percent": "35.8"}],
    "incomes": [{"name": "Sueldo", "total": "3200.00", "percent": "100.0"}]
  }]
}`

func TestRendersPDFForOrbita(t *testing.T) {
	e := newEnv(t)
	rec := e.post(e.token(t, "service", "orbita"), validBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "application/pdf" || !strings.HasPrefix(rec.Body.String(), "%PDF-") {
		t.Fatal("not a PDF response")
	}
	if cd := rec.Header().Get("Content-Disposition"); cd != `inline; filename="orbita-2026-10-01_2026-10-31.pdf"` {
		t.Fatalf("content disposition = %q", cd)
	}
	snap := e.recorder.Snapshot()
	if snap.Totals.OK != 1 || snap.Recent[0].Template != periodreport.TemplateCode || snap.Recent[0].Items != 2 {
		t.Fatalf("activity = %+v", snap)
	}
}

func TestRejectsUnauthorizedCallers(t *testing.T) {
	e := newEnv(t)
	if rec := e.post("", validBody); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: %d", rec.Code)
	}
	if rec := e.post(e.token(t, "service", "lavendimia"), validBody); rec.Code == http.StatusOK {
		t.Fatal("a system outside the allowed list was served")
	}
	if rec := e.post(e.token(t, "superadmin", ""), validBody); rec.Code == http.StatusOK {
		t.Fatal("a non-service token was served")
	}
}

func TestRejectsBadPayloads(t *testing.T) {
	e := newEnv(t)
	token := e.token(t, "service", "orbita")
	cases := map[string]struct {
		body   string
		status int
		code   string
	}{
		"not json":      {"nope", 400, "invalid_request"},
		"unknown field": {strings.Replace(validBody, `"from"`, `"extra": 1, "from"`, 1), 400, "invalid_request"},
		"trailing data": {validBody + `{}`, 400, "invalid_request"},
		"float money":   {strings.Replace(validBody, `"3200.00", "expense_total"`, `3200.00, "expense_total"`, 1), 400, "invalid_request"},
		"bad date":      {strings.Replace(validBody, "2026-10-01", "2026-02-30", 1), 422, "invalid_payload"},
		"bad decimal":   {strings.Replace(validBody, `"1245.80"`, `"1245,80"`, 1), 422, "invalid_payload"},
		"no currencies": {`{"from":"2026-10-01","to":"2026-10-31","currencies":[]}`, 422, "invalid_payload"},
		"too large":     {strings.Replace(validBody, "Comida", strings.Repeat("a", periodreport.MaxBodyBytes), 1), 413, "payload_too_large"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			rec := e.post(token, c.body)
			if rec.Code != c.status || errorCode(t, rec) != c.code {
				t.Fatalf("status = %d code = %s, want %d %s", rec.Code, errorCode(t, rec), c.status, c.code)
			}
		})
	}
}
