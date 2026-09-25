package web

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fernandoris/blindenv/pkg/backup"
	"github.com/fernandoris/blindenv/pkg/db"
	webassets "github.com/fernandoris/blindenv/web"
)

// Options configures the dashboard server.
type Options struct {
	Store  *db.Store
	Addr   string
	Token  string
	Dev    bool
	Logger *log.Logger
}

type server struct {
	store  *db.Store
	token  string
	dev    bool
	logger *log.Logger
}

// Serve runs the dashboard until ctx is cancelled.
func Serve(ctx context.Context, opts Options) error {
	if opts.Store == nil {
		return errors.New("web: store is required")
	}
	if opts.Addr == "" {
		opts.Addr = "127.0.0.1:8080"
	}
	host, _, err := net.SplitHostPort(opts.Addr)
	if err != nil {
		return fmt.Errorf("web: invalid address %q: %w", opts.Addr, err)
	}
	if !isLoopbackHost(host) {
		return fmt.Errorf("web: refusing to bind non-loopback address %q", host)
	}
	logger := opts.Logger
	if logger == nil {
		logger = log.New(os.Stderr, "blindenv: ", log.LstdFlags)
	}
	token := opts.Token
	if token == "" {
		token = randomToken()
	}
	s := &server{store: opts.Store, token: token, dev: opts.Dev, logger: logger}

	httpServer := &http.Server{Addr: opts.Addr, Handler: s.routes()}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	logger.Printf("dashboard available at http://%s/?token=%s", opts.Addr, token)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *server) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/app.js", s.handleAsset("app.js", "application/javascript; charset=utf-8"))
	mux.HandleFunc("/app.css", s.handleAsset("dist/app.css", "text/css; charset=utf-8"))
	mux.HandleFunc("/api/state", s.sec(s.handleState))
	mux.HandleFunc("/api/audit", s.sec(s.handleAudit))
	mux.HandleFunc("/api/projects", s.sec(s.handleProjects))
	mux.HandleFunc("/api/projects/", s.sec(s.handleProjectSub))
	mux.HandleFunc("/api/shared/", s.sec(s.handleShared))
	mux.HandleFunc("/api/export", s.sec(s.handleExport))
	mux.HandleFunc("/api/import", s.sec(s.handleImport))
	return mux
}

// sec enforces loopback origin, an allowed Host header and a valid token.
func (s *server) sec(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil || !isLoopbackHost(remoteHost) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		host := r.Host
		if h, _, err := net.SplitHostPort(r.Host); err == nil {
			host = h
		}
		if !isLoopbackHost(host) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if !s.authorized(r) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *server) authorized(r *http.Request) bool {
	header := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(header, "Bearer "); ok && subtleEqual(after, s.token) {
		return true
	}
	if q := r.URL.Query().Get("token"); q != "" && subtleEqual(q, s.token) {
		return true
	}
	return false
}

func subtleEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := 0; i < len(a); i++ {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

func (s *server) asset(name string) ([]byte, error) {
	if s.dev {
		return os.ReadFile(filepath.Join("web", name))
	}
	return webassets.FS.ReadFile(name)
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := s.asset("index.html")
	if err != nil {
		http.Error(w, "index not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func (s *server) handleAsset(name, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := s.asset(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write(data)
	}
}

// --- views ---

type secretView struct {
	Key         string   `json:"key"`
	Scope       string   `json:"scope"`
	Environment string   `json:"environment,omitempty"`
	Overrides   []string `json:"overrides,omitempty"`
}

type effectiveView struct {
	Key         string `json:"key"`
	Scope       string `json:"scope"`
	Environment string `json:"environment,omitempty"`
}

type envView struct {
	Name      string          `json:"name"`
	Secrets   []secretView    `json:"secrets"`
	Effective []effectiveView `json:"effective"`
}

type projectView struct {
	Slug         string       `json:"slug"`
	AllowExecute bool         `json:"allow_execute"`
	Globals      []secretView `json:"globals"`
	Environments []envView    `json:"environments"`
}

type sharedEnvView struct {
	Name    string       `json:"name"`
	Secrets []secretView `json:"secrets"`
}

type sharedView struct {
	Global       []secretView    `json:"global"`
	Environments []sharedEnvView `json:"environments"`
}

func toSecretViews(in []db.SecretInfo) []secretView {
	out := make([]secretView, 0, len(in))
	for _, s := range in {
		overrides := make([]string, 0, len(s.Overrides))
		for _, sc := range s.Overrides {
			overrides = append(overrides, string(sc))
		}
		out = append(out, secretView{Key: s.Key, Scope: string(s.Scope), Environment: s.Environment, Overrides: overrides})
	}
	return out
}

func toEffectiveViews(m map[string]db.Scope) []effectiveView {
	out := make([]effectiveView, 0, len(m))
	for key, scope := range m {
		out = append(out, effectiveView{Key: key, Scope: string(scope)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func (s *server) handleState(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projects, err := s.store.ListProjects(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]projectView, 0, len(projects))
	for _, p := range projects {
		pv := projectView{Slug: p.Slug, AllowExecute: p.AllowExecute, Globals: []secretView{}, Environments: []envView{}}
		globals, err := s.store.ScopeSecrets(ctx, p.Slug, "")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		pv.Globals = toSecretViews(globals)
		envs, err := s.store.ListEnvironments(ctx, p.Slug)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		for _, e := range envs {
			secrets, err := s.store.ScopeSecrets(ctx, p.Slug, e.Name)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			effective, err := s.store.EffectiveScopes(ctx, p.Slug, e.Name)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			pv.Environments = append(pv.Environments, envView{
				Name:      e.Name,
				Secrets:   toSecretViews(secrets),
				Effective: toEffectiveViews(effective),
			})
		}
		out = append(out, pv)
	}

	sharedGlobal, err := s.store.ScopeSecrets(ctx, "", "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	names, err := s.store.ListSharedEnvironments(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	sharedEnvs := make([]sharedEnvView, 0, len(names))
	for _, name := range names {
		secrets, err := s.store.ScopeSecrets(ctx, "", name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		sharedEnvs = append(sharedEnvs, sharedEnvView{Name: name, Secrets: toSecretViews(secrets)})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"projects":            out,
		"shared":              sharedView{Global: toSecretViews(sharedGlobal), Environments: sharedEnvs},
		"shared_environments": names,
	})
}

func (s *server) handleAudit(w http.ResponseWriter, r *http.Request) {
	entries, err := s.store.ListAudit(r.Context(), 200)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		scopes := make([]string, 0, len(e.KeyScopes))
		for _, sc := range e.KeyScopes {
			scopes = append(scopes, string(sc))
		}
		out = append(out, map[string]any{
			"timestamp":   e.Timestamp,
			"client":      e.Client,
			"project":     e.Project,
			"environment": e.Environment,
			"tool":        e.Tool,
			"key_names":   e.KeyNames,
			"key_scopes":  scopes,
			"command":     e.Command,
			"exit_code":   e.ExitCode,
			"redactions":  e.Redactions,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": out})
}

func (s *server) handleProjects(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleState(w, r)
	case http.MethodPost:
		var body struct {
			Slug string `json:"slug"`
		}
		if !readJSON(w, r, &body) {
			return
		}
		project, err := s.store.CreateProject(r.Context(), strings.TrimSpace(body.Slug))
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusCreated, project)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *server) handleProjectSub(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/projects/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	slug := parts[0]
	ctx := r.Context()

	switch {
	case len(parts) == 1 && r.Method == http.MethodDelete:
		if err := s.store.DeleteProject(ctx, slug); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": slug})

	case len(parts) == 2 && parts[1] == "environments" && r.Method == http.MethodPost:
		var body struct {
			Name string `json:"name"`
		}
		if !readJSON(w, r, &body) {
			return
		}
		env, err := s.store.CreateEnvironment(ctx, slug, strings.TrimSpace(body.Name))
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusCreated, env)

	case len(parts) == 2 && parts[1] == "allow-execute" && r.Method == http.MethodPost:
		var body struct {
			Allow bool `json:"allow"`
		}
		if !readJSON(w, r, &body) {
			return
		}
		if err := s.store.SetAllowExecute(ctx, slug, body.Allow); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"allow_execute": body.Allow})

	case len(parts) == 2 && parts[1] == "secrets" && r.Method == http.MethodPost:
		var body struct {
			Environment string `json:"environment"`
			Key         string `json:"key"`
			Value       string `json:"value"`
		}
		if !readJSON(w, r, &body) {
			return
		}
		short, err := s.store.PutSecret(ctx, slug, body.Environment, strings.TrimSpace(body.Key), body.Value)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"short": short})

	case len(parts) == 2 && parts[1] == "secrets" && r.Method == http.MethodDelete:
		environment := r.URL.Query().Get("environment")
		key := r.URL.Query().Get("key")
		if err := s.store.DeleteSecret(ctx, slug, environment, key); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": key})

	case len(parts) == 3 && parts[1] == "secrets" && parts[2] == "reveal" && r.Method == http.MethodPost:
		var body struct {
			Environment string `json:"environment"`
			Key         string `json:"key"`
		}
		if !readJSON(w, r, &body) {
			return
		}
		value, err := s.scopeValue(ctx, slug, body.Environment, body.Key)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"value": value})

	default:
		http.NotFound(w, r)
	}
}

// handleShared manages the cross-project scopes: global (empty environment)
// and environment-global (named environment).
func (s *server) handleShared(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/shared/")
	ctx := r.Context()

	switch {
	case rest == "secrets" && r.Method == http.MethodPost:
		var body struct {
			Environment string `json:"environment"`
			Key         string `json:"key"`
			Value       string `json:"value"`
		}
		if !readJSON(w, r, &body) {
			return
		}
		short, err := s.store.PutSecret(ctx, "", body.Environment, strings.TrimSpace(body.Key), body.Value)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"short": short})

	case rest == "secrets" && r.Method == http.MethodDelete:
		environment := r.URL.Query().Get("environment")
		key := r.URL.Query().Get("key")
		if err := s.store.DeleteSecret(ctx, "", environment, key); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": key})

	case rest == "secrets/reveal" && r.Method == http.MethodPost:
		var body struct {
			Environment string `json:"environment"`
			Key         string `json:"key"`
		}
		if !readJSON(w, r, &body) {
			return
		}
		value, err := s.scopeValue(ctx, "", body.Environment, body.Key)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"value": value})

	default:
		http.NotFound(w, r)
	}
}

// scopeValue returns the value defined in exactly one scope, so a reveal never
// silently falls back to a broader scope.
func (s *server) scopeValue(ctx context.Context, project, environment, key string) (string, error) {
	values, err := s.store.ScopeValues(ctx, project, environment)
	if err != nil {
		return "", err
	}
	value, ok := values[key]
	if !ok {
		return "", fmt.Errorf("%w: %q", db.ErrSecretNotFound, key)
	}
	return value, nil
}

func (s *server) handleExport(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Passphrase string `json:"passphrase"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	blob, err := backup.Export(r.Context(), s.store, body.Passphrase)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="blindenv-backup.bin"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(blob)
}

func (s *server) handleImport(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Passphrase string `json:"passphrase"`
		Data       string `json:"data"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	blob, err := base64.StdEncoding.DecodeString(body.Data)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid base64 data: %w", err))
		return
	}
	if err := backup.Import(r.Context(), s.store, body.Passphrase, blob); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"imported": true})
}

// --- helpers ---
func isLoopbackHost(host string) bool {
	if host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func randomToken() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "insecure-token"
	}
	return hex.EncodeToString(buf)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}

func readJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON body: %w", err))
		return false
	}
	return true
}
