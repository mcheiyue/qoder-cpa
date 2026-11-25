package main

import (
	"regexp"
	"testing"
)

// TestEmbeddedWebUIHasNoExternalResources 保证 go:embed 产物自包含，
// 以免离线运行或 CSP 阻断页面依赖。
func TestEmbeddedWebUIHasNoExternalResources(t *testing.T) {
	html := string(qoderWebUI)
	externalResource := regexp.MustCompile(`(?i)<(?:script|link)\b[^>]*(?:src|href)\s*=\s*["'](?:https?:)?//`)
	if match := externalResource.FindString(html); match != "" {
		t.Fatalf("embedded web UI must not add external script/link resource: %q", match)
	}
}
