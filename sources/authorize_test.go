package sources

import (
	"github.com/euler-gaozl/agent-os-sdk/servicehttp"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServiceIdentityBoundary(t *testing.T) {
	tokens := map[string]string{}
	peers := map[string]string{}
	for i, role := range []string{"control", "futures", "worker", "runtime", "registry", "toolhub", "sources"} {
		tokens[role] = strings.Repeat(string(rune('a'+i)), 40)
		peers[role] = servicehttp.TokenHash(tokens[role])
	}
	handler := servicehttp.New(servicehttp.Config{Role: "sources", Peers: peers, Authorize: Authorize}, nil, func(m *http.ServeMux) {
		m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	})
	for _, tc := range []struct {
		role, method, path string
		want               int
	}{{"futures", "POST", "/mcp", 403}, {"worker", "POST", "/mcp", 403}, {"toolhub", "POST", "/mcp", 403}, {"sources", "POST", "/mcp", 204}, {"control", "GET", "/captures/id", 204}, {"sources", "GET", "/captures/id", 403}, {"", "POST", "/mcp", 401}} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.Header.Set("Authorization", "Bearer "+tokens[tc.role])
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != tc.want {
			t.Fatalf("%+v got %d", tc, res.Code)
		}
	}
}
