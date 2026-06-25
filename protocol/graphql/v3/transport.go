// transport.go 实现 GraphQL adapter 的 HTTP 传输入口与请求处理流程。
package graphql

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ServeHTTP 实现 `http.Handler`，用于处理 GraphQL HTTP 请求与可选的 Playground 页面。
func (a *Adapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if a.playgroundEnabled && r.Method == http.MethodGet && r.URL.Path == a.playgroundPath {
		a.servePlayground(w)
		return
	}
	if a.standalone && r.URL.Path != a.path {
		http.NotFound(w, r)
		return
	}
	if a.host == nil {
		writeJSON(w, http.StatusInternalServerError, GraphQLResponse{Errors: []any{a.formatError(&GraphQLError{Message: "graphql adapter not mounted"})}})
		return
	}
	if err := a.ensureSchema(); err != nil {
		writeJSON(w, http.StatusInternalServerError, GraphQLResponse{Errors: []any{a.formatError(err)}})
		return
	}

	requests, batched, status, err := a.parseGraphQLRequests(w, r)
	if err != nil {
		writeJSON(w, status, GraphQLResponse{Errors: []any{a.formatError(err)}})
		return
	}

	if batched {
		responses := make([]GraphQLResponse, 0, len(requests))
		for _, req := range requests {
			responses = append(responses, a.executeRequest(r.Context(), w, r, req))
		}
		writeJSON(w, http.StatusOK, responses)
		return
	}

	resp := a.executeRequest(r.Context(), w, r, requests[0])
	writeJSON(w, http.StatusOK, resp)
}

func (a *Adapter) servePlayground(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, strings.ReplaceAll(playgroundHTML, "{{GRAPHQL_PATH}}", a.path))
}

func (a *Adapter) parseGraphQLRequests(w http.ResponseWriter, r *http.Request) ([]*GraphQLRequest, bool, int, error) {
	switch r.Method {
	case http.MethodGet:
		req := &GraphQLRequest{
			Query:         r.URL.Query().Get("query"),
			OperationName: r.URL.Query().Get("operationName"),
			Extensions:    map[string]any{},
		}
		if raw := r.URL.Query().Get("variables"); raw != "" {
			if err := json.Unmarshal([]byte(raw), &req.Variables); err != nil {
				return nil, false, http.StatusBadRequest, fmt.Errorf("invalid variables: %w", err)
			}
		}
		return normalizeRequests([]*GraphQLRequest{req})
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, a.maxBodyBytes)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, false, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err)
		}
		defer r.Body.Close()
		ct := strings.ToLower(r.Header.Get("Content-Type"))
		if strings.Contains(ct, "application/graphql") {
			return normalizeRequests([]*GraphQLRequest{{Query: string(body), Extensions: map[string]any{}}})
		}
		trimmed := strings.TrimSpace(string(body))
		if strings.HasPrefix(trimmed, "[") {
			var requests []*GraphQLRequest
			if err := json.Unmarshal(body, &requests); err != nil {
				return nil, false, http.StatusBadRequest, fmt.Errorf("invalid graphql batch request: %w", err)
			}
			reqs, _, status, err := normalizeRequests(requests)
			return reqs, true, status, err
		}
		var req GraphQLRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, false, http.StatusBadRequest, fmt.Errorf("invalid graphql request: %w", err)
		}
		return normalizeRequests([]*GraphQLRequest{&req})
	default:
		return nil, false, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed")
	}
}

func normalizeRequests(items []*GraphQLRequest) ([]*GraphQLRequest, bool, int, error) {
	if len(items) == 0 {
		return nil, false, http.StatusBadRequest, fmt.Errorf("graphql request is empty")
	}
	for _, item := range items {
		if item == nil {
			return nil, false, http.StatusBadRequest, fmt.Errorf("graphql request contains nil item")
		}
		if strings.TrimSpace(item.Query) == "" {
			return nil, false, http.StatusBadRequest, fmt.Errorf("query is required")
		}
		if item.Variables == nil {
			item.Variables = map[string]any{}
		}
		if item.Extensions == nil {
			item.Extensions = map[string]any{}
		}
	}
	return items, len(items) > 1, http.StatusOK, nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

const playgroundHTML = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8" />
  <title>GraphQL Playground</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, sans-serif; margin: 2rem; }
    textarea { width: 100%; min-height: 16rem; font-family: monospace; }
    pre { background: #111827; color: #f9fafb; padding: 1rem; overflow: auto; }
  </style>
</head>
<body>
  <h1>GraphQL Playground</h1>
  <p>POST to <code>{{GRAPHQL_PATH}}</code></p>
  <textarea id="query">query { __typename }</textarea>
  <button onclick="run()">Run</button>
  <pre id="output"></pre>
  <script>
    async function run() {
      const query = document.getElementById('query').value;
      const res = await fetch('{{GRAPHQL_PATH}}', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ query })
      });
      document.getElementById('output').textContent = await res.text();
    }
  </script>
</body>
</html>`
