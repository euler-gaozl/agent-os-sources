package sources

import "strings"

func Authorize(caller, method, path string) bool {
	if path == "/readyz" || path == "/metrics" {
		return method == "GET" && (caller == "sources" || caller == "control")
	}
	if caller == "sources" {
		return method == "POST" && path == "/mcp"
	}
	if caller == "control" {
		return method == "POST" && path == "/captures" || method == "GET" && (path == "/captures" || path == "/capture-capabilities" || strings.HasPrefix(path, "/captures/"))
	}
	return caller == "knowledge" && method == "GET" && (path == "/capture-capabilities" || captureBundlePath.MatchString(path))
}
