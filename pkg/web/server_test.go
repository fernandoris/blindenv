package web

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fernandoris/blindenv/pkg/crypto"
	"github.com/fernandoris/blindenv/pkg/db"
)

const testToken = "test-token-1234567890"

func newTestWeb(t *testing.T) (*server, *db.Store, http.Handler) {
	t.Helper()
	key, err := crypto.NewSalt(crypto.KeySize)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	store, err := db.Open(filepath.Join(t.TempDir(), "vault.db"), key)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	s := &server{store: store, token: testToken, logger: log.New(io.Discard, "", 0)}
	return s, store, s.routes()
}

func do(t *testing.T, mux http.Handler, method, path, body, token, host string) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	req.RemoteAddr = "127.0.0.1:55555"
	if host == "" {
		host = "127.0.0.1:8080"
	}
	req.Host = host
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func TestUnauthorized(t *testing.T) {
	_, _, mux := newTestWeb(t)
	if got := do(t, mux, http.MethodGet, "/api/state", "", "", "").Code; got != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", got)
	}
	if got := do(t, mux, http.MethodGet, "/api/state?token="+testToken, "", "", "").Code; got != http.StatusOK {
		t.Fatalf("query token status = %d, want 200", got)
	}
}

func TestHostHeaderRejected(t *testing.T) {
	_, _, mux := newTestWeb(t)
	if got := do(t, mux, http.MethodGet, "/api/state", "", testToken, "evil.example.com").Code; got != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", got)
	}
}

func TestStateHidesValues(t *testing.T) {
	_, store, mux := newTestWeb(t)
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, "my-api")
	_, _ = store.CreateEnvironment(ctx, "my-api", "staging")
	_, _ = store.PutSecret(ctx, "my-api", "staging", "API_KEY", "sk-should-not-appear")

	w := do(t, mux, http.MethodGet, "/api/state", "", testToken, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "API_KEY") {
		t.Fatalf("key name missing: %s", body)
	}
	if strings.Contains(body, "sk-should-not-appear") {
		t.Fatalf("secret value leaked: %s", body)
	}
}

func TestSecretLifecycleViaAPI(t *testing.T) {
	_, _, mux := newTestWeb(t)

	if got := do(t, mux, http.MethodPost, "/api/projects", `{"slug":"my-api"}`, testToken, "").Code; got != http.StatusCreated {
		t.Fatalf("create project status = %d", got)
	}
	if got := do(t, mux, http.MethodPost, "/api/projects/my-api/environments", `{"name":"staging"}`, testToken, "").Code; got != http.StatusCreated {
		t.Fatalf("create env status = %d", got)
	}
	if got := do(t, mux, http.MethodPost, "/api/projects/my-api/secrets", `{"environment":"staging","key":"API_KEY","value":"sk-live-123456"}`, testToken, "").Code; got != http.StatusOK {
		t.Fatalf("put secret status = %d", got)
	}

	reveal := do(t, mux, http.MethodPost, "/api/projects/my-api/secrets/reveal", `{"environment":"staging","key":"API_KEY"}`, testToken, "")
	if reveal.Code != http.StatusOK || !strings.Contains(reveal.Body.String(), "sk-live-123456") {
		t.Fatalf("reveal = %d %s", reveal.Code, reveal.Body.String())
	}

	del := do(t, mux, http.MethodDelete, "/api/projects/my-api/secrets?environment=staging&key=API_KEY", "", testToken, "")
	if del.Code != http.StatusOK {
		t.Fatalf("delete status = %d", del.Code)
	}
}

func TestAllowExecuteAndAuditAPI(t *testing.T) {
	_, store, mux := newTestWeb(t)
	ctx := context.Background()
	_, _ = store.CreateProject(ctx, "my-api")
	_ = store.AppendAudit(ctx, db.AuditEntry{Timestamp: time.Now(), Tool: "list_secret_keys", Project: "my-api"})

	if got := do(t, mux, http.MethodPost, "/api/projects/my-api/allow-execute", `{"allow":true}`, testToken, "").Code; got != http.StatusOK {
		t.Fatalf("allow-execute status = %d", got)
	}
	p, _ := store.GetProject(ctx, "my-api")
	if !p.AllowExecute {
		t.Fatal("allow_execute not persisted")
	}

	audit := do(t, mux, http.MethodGet, "/api/audit", "", testToken, "")
	if audit.Code != http.StatusOK || !strings.Contains(audit.Body.String(), "list_secret_keys") {
		t.Fatalf("audit = %d %s", audit.Code, audit.Body.String())
	}
}

func TestDevAssets(t *testing.T) {
	t.Chdir("../..")
	s, _, _ := newTestWeb(t)
	s.dev = true
	if _, err := s.asset("index.html"); err != nil {
		t.Fatalf("dev asset index.html: %v", err)
	}
	if _, err := s.asset("dist/app.css"); err != nil {
		t.Fatalf("dev asset app.css: %v", err)
	}
}
