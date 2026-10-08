package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/fernandoris/blindenv/pkg/db"
)

const (
	proxyTimeout     = 30 * time.Second
	maxResponseBytes = 1 << 20
	maxRedirects     = 10
)

type proxyResult struct {
	Status        int                 `json:"status"`
	Headers       map[string][]string `json:"headers"`
	Body          string              `json:"body"`
	Substitutions int                 `json:"substitutions"`
	UnmatchedTags []string            `json:"unmatched_tags"`
	Redactions    int                 `json:"redactions"`
}

// maxUnmatchedTags bounds the number of distinct unmatched tag names reported,
// so an adversarial request body cannot grow the response.
const maxUnmatchedTags = 20

// unmatchedCollector gathers the distinct {{NAME}} tag names present in
// caller-supplied text that do not name an effective key, in order of first
// appearance and capped at maxUnmatchedTags.
type unmatchedCollector struct {
	keys  map[string]struct{}
	seen  map[string]struct{}
	names []string
}

func newUnmatchedCollector(keys map[string]struct{}) *unmatchedCollector {
	return &unmatchedCollector{keys: keys, seen: make(map[string]struct{}), names: []string{}}
}

// add records one tag name if it is unmatched, distinct and under the cap.
func (c *unmatchedCollector) add(name string) {
	if name == "" || len(c.names) >= maxUnmatchedTags {
		return
	}
	if _, ok := c.keys[name]; ok {
		return
	}
	if _, dup := c.seen[name]; dup {
		return
	}
	c.seen[name] = struct{}{}
	c.names = append(c.names, name)
}

// addAll records each name, deduplicating and respecting the cap.
func (c *unmatchedCollector) addAll(names []string) {
	for _, name := range names {
		c.add(name)
	}
}

// scan records every distinct {{NAME}} tag in text that is not an effective key.
func (c *unmatchedCollector) scan(text string) {
	for i := 0; i+2 <= len(text); {
		j := strings.Index(text[i:], "{{")
		if j < 0 {
			return
		}
		start := i + j + 2
		end := strings.Index(text[start:], "}}")
		if end < 0 {
			return
		}
		name := text[start : start+end]
		i = start + end + 2
		c.add(name)
		if len(c.names) >= maxUnmatchedTags {
			return
		}
	}
}

func (c *unmatchedCollector) list() []string { return c.names }

func (s *Server) handleProxy(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	project, environment, err := s.resolveContextForSecrets(req.GetString("project", ""), req.GetString("environment", ""))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if res := s.vaultUnavailable(); res != nil {
		return res, nil
	}
	target := req.GetString("url", "")
	if target == "" {
		return mcp.NewToolResultError("url is required"), nil
	}
	method := strings.ToUpper(req.GetString("method", "GET"))
	if method == "" {
		method = http.MethodGet
	}
	inHeaders := stringMapArg(req, "headers")
	inBody := req.GetString("body", "")

	resolved, err := s.cfg.Store.ResolveDetailed(ctx, project, environment)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	secrets := valuesOf(resolved)
	unmatched := newUnmatchedCollector(keySet(secrets))
	substitutions := 0

	resolvedHeaders := make(map[string]string, len(inHeaders))
	secretHeader := make(map[string]bool)
	for k, v := range inHeaders {
		resolved, n := substitute(v, secrets)
		substitutions += n
		unmatched.scan(v)
		resolvedHeaders[k] = resolved
		if resolved != v {
			secretHeader[k] = true
		}
	}
	resolvedURL, n := substitute(target, secrets)
	substitutions += n
	unmatched.scan(target)
	resolvedBody, n := substituteBody(inBody, secrets)
	substitutions += n
	unmatched.scan(inBody)

	var bodyReader io.Reader
	if resolvedBody != "" {
		bodyReader = strings.NewReader(resolvedBody)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, resolvedURL, bodyReader)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid request: %v", err)), nil
	}
	for k, v := range resolvedHeaders {
		httpReq.Header.Set(k, v)
	}

	client := &http.Client{
		Timeout: proxyTimeout,
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errors.New("too many redirects")
			}
			if len(via) > 0 && next.URL.Host != via[0].URL.Host {
				for h := range secretHeader {
					next.Header.Del(h)
				}
			}
			return nil
		},
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		s.audit(ctx, project, environment, "proxy_http_request", keysOf(secrets), method+" "+safeURL(resolvedURL), nil, 0, substitutions)
		return mcp.NewToolResultError(fmt.Sprintf("request failed: %v", err)), nil
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("read response: %v", err)), nil
	}

	redactor := NewRedactor(sensitiveValuesOf(resolved), db.MinSecretLength)
	bodyText, redactions := redactor.Redact(bodyBytes)

	headers := make(map[string][]string, len(resp.Header))
	for k, vs := range resp.Header {
		redacted := make([]string, len(vs))
		for i, v := range vs {
			text, n := redactor.RedactString(v)
			redacted[i] = text
			redactions += n
		}
		headers[k] = redacted
	}

	s.audit(ctx, project, environment, "proxy_http_request", keysOf(secrets), method+" "+safeURL(resolvedURL), &resp.StatusCode, redactions, substitutions)

	return mcp.NewToolResultJSON(proxyResult{
		Status:        resp.StatusCode,
		Headers:       headers,
		Body:          bodyText,
		Substitutions: substitutions,
		UnmatchedTags: unmatched.list(),
		Redactions:    redactions,
	})
}

// substitute replaces every {{KEY}} tag with the matching secret value and
// returns the number of occurrences replaced.
func substitute(in string, secrets map[string]string) (string, int) {
	count := 0
	for key, value := range secrets {
		tag := "{{" + key + "}}"
		if n := strings.Count(in, tag); n > 0 {
			in = strings.ReplaceAll(in, tag, value)
			count += n
		}
	}
	return in, count
}

// substituteBody substitutes tags in a body, preserving JSON validity by
// walking decoded string values when the body is valid JSON. It returns the
// resolved body and the number of occurrences replaced.
func substituteBody(body string, secrets map[string]string) (string, int) {
	if strings.TrimSpace(body) == "" {
		return body, 0
	}
	var data any
	if err := json.Unmarshal([]byte(body), &data); err == nil {
		resolved, count := substituteJSON(data, secrets)
		if out, err := json.Marshal(resolved); err == nil {
			return string(out), count
		}
	}
	return substitute(body, secrets)
}

func substituteJSON(v any, secrets map[string]string) (any, int) {
	switch t := v.(type) {
	case string:
		return substitute(t, secrets)
	case []any:
		count := 0
		for i := range t {
			var n int
			t[i], n = substituteJSON(t[i], secrets)
			count += n
		}
		return t, count
	case map[string]any:
		count := 0
		for k := range t {
			var n int
			t[k], n = substituteJSON(t[k], secrets)
			count += n
		}
		return t, count
	default:
		return v, 0
	}
}

// safeURL strips the query string so secrets in query parameters never reach
// the audit log.
func safeURL(raw string) string {
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		return raw[:i] + "?..."
	}
	return raw
}
