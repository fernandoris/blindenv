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
	Status     int                 `json:"status"`
	Headers    map[string][]string `json:"headers"`
	Body       string              `json:"body"`
	Redactions int                 `json:"redactions"`
}

func (s *Server) handleProxy(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	project, environment, err := s.resolveContext(req.GetString("project", ""), req.GetString("environment", ""))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
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

	secrets, err := s.cfg.Store.Resolve(ctx, project, environment)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	resolvedHeaders := make(map[string]string, len(inHeaders))
	secretHeader := make(map[string]bool)
	for k, v := range inHeaders {
		resolved := substitute(v, secrets)
		resolvedHeaders[k] = resolved
		if resolved != v {
			secretHeader[k] = true
		}
	}
	resolvedURL := substitute(target, secrets)
	resolvedBody := substituteBody(inBody, secrets)

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
		s.audit(ctx, project, environment, "proxy_http_request", keysOf(secrets), method+" "+safeURL(resolvedURL), nil, 0)
		return mcp.NewToolResultError(fmt.Sprintf("request failed: %v", err)), nil
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("read response: %v", err)), nil
	}

	redactor := NewRedactor(secrets, db.MinSecretLength)
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

	s.audit(ctx, project, environment, "proxy_http_request", keysOf(secrets), method+" "+safeURL(resolvedURL), &resp.StatusCode, redactions)

	return mcp.NewToolResultJSON(proxyResult{
		Status:     resp.StatusCode,
		Headers:    headers,
		Body:       bodyText,
		Redactions: redactions,
	})
}

// substitute replaces every {{KEY}} tag with the matching secret value.
func substitute(in string, secrets map[string]string) string {
	for key, value := range secrets {
		in = strings.ReplaceAll(in, "{{"+key+"}}", value)
	}
	return in
}

// substituteBody substitutes tags in a body, preserving JSON validity by
// walking decoded string values when the body is valid JSON.
func substituteBody(body string, secrets map[string]string) string {
	if strings.TrimSpace(body) == "" {
		return body
	}
	var data any
	if err := json.Unmarshal([]byte(body), &data); err == nil {
		if out, err := json.Marshal(substituteJSON(data, secrets)); err == nil {
			return string(out)
		}
	}
	return substitute(body, secrets)
}

func substituteJSON(v any, secrets map[string]string) any {
	switch t := v.(type) {
	case string:
		return substitute(t, secrets)
	case []any:
		for i := range t {
			t[i] = substituteJSON(t[i], secrets)
		}
		return t
	case map[string]any:
		for k := range t {
			t[k] = substituteJSON(t[k], secrets)
		}
		return t
	default:
		return v
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
