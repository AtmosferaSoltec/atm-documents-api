package catalog_test

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
	"github.com/AtmosferaSoltec/atm-documents-api/internal/catalog"
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
	recorder := activity.New()
	handler := catalog.NewHandler(salesnote.NewService(renderer.New(), 2), recorder, catalog.Settings{
		Environment: "production", AllowedSystems: []string{"lavendimia"}, MaxConcurrentRenders: 4, RenderTimeout: 15 * time.Second,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	catalog.RegisterRoutes(engine.Group("/v1/admin", verifier.Superadmin()), handler)
	return env{engine: engine, key: key, recorder: recorder}
}

func (e env) get(t *testing.T, role, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if role != "" {
		claims := jwt.MapClaims{"iss": issuer, "sub": "someone", "role": role, "system": "lavendimia", "iat": time.Now().Unix(), "exp": time.Now().Add(time.Minute).Unix()}
		token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(e.key)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.engine.ServeHTTP(rec, req)
	return rec
}

func TestOnlySuperadminReachesTheAdminRoutes(t *testing.T) {
	e := newEnv(t)
	for _, path := range []string{"/v1/admin/templates", "/v1/admin/templates/sales-note/preview", "/v1/admin/activity"} {
		if rec := e.get(t, "", path); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without token = %d", path, rec.Code)
		}
		for _, role := range []string{"service", "user"} {
			if rec := e.get(t, role, path); rec.Code != http.StatusForbidden {
				t.Errorf("%s as %s = %d, want 403", path, role, rec.Code)
			}
		}
	}
}

func TestListsTemplatesAndServiceSettings(t *testing.T) {
	rec := newEnv(t).get(t, "superadmin", "/v1/admin/templates")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var body catalog.Listing
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Templates) != 1 || body.Templates[0].Code != "sales-note" || len(body.Templates[0].Formats) != 2 {
		t.Fatalf("templates = %+v", body.Templates)
	}
	if body.Templates[0].Limits.MaxItems["ticket"] != salesnote.MaxItemsTicket {
		t.Fatalf("limits = %+v", body.Templates[0].Limits)
	}
	if body.Service.Environment != "production" || body.Service.RenderTimeoutSeconds != 15 || body.Service.AllowedSystems[0] != "lavendimia" {
		t.Fatalf("service = %+v", body.Service)
	}
}

func TestPreviewRendersBothFormatsWithoutRecordingActivity(t *testing.T) {
	e := newEnv(t)
	for _, format := range []string{"a4", "ticket", ""} {
		path := "/v1/admin/templates/sales-note/preview"
		if format != "" {
			path += "?format=" + format
		}
		rec := e.get(t, "superadmin", path)
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/pdf" || !strings.HasPrefix(rec.Body.String(), "%PDF-") {
			t.Fatalf("format %q: status %d type %s", format, rec.Code, rec.Header().Get("Content-Type"))
		}
	}
	if got := e.recorder.Snapshot().Totals; got != (activity.Totals{}) {
		t.Fatalf("previews must not count as rendered documents: %+v", got)
	}
}

func TestPreviewRejectsUnknownTemplateAndFormat(t *testing.T) {
	e := newEnv(t)
	if rec := e.get(t, "superadmin", "/v1/admin/templates/nope/preview"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown template = %d", rec.Code)
	}
	if rec := e.get(t, "superadmin", "/v1/admin/templates/sales-note/preview?format=a3"); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("unknown format = %d", rec.Code)
	}
}

func TestActivityEndpointReturnsTheSnapshot(t *testing.T) {
	e := newEnv(t)
	e.recorder.Record(activity.Event{System: "lavendimia", Template: "sales-note", Format: "a4", Outcome: activity.OutcomeOK, DurationMS: 12})
	rec := e.get(t, "superadmin", "/v1/admin/activity")
	var body activity.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("status %d err %v: %s", rec.Code, err, rec.Body.String())
	}
	if body.Totals.OK != 1 || len(body.Recent) != 1 || body.Recent[0].System != "lavendimia" {
		t.Fatalf("activity = %+v", body)
	}
}
