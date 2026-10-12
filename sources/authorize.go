package sources

import "strings"

func Authorize(caller, method, path string) bool {
	if path == "/readyz" || path == "/metrics" {
		return method == "GET" && (caller == "sources" || caller == "control")
	}
	return caller == "sources" && method == "POST" && path == "/mcp" || caller == "control" && method == "GET" && strings.HasPrefix(path, "/captures/")
}
