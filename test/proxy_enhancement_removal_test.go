package test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func proxyRepoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve caller path")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}

func readProxyRepoFile(t *testing.T, parts ...string) string {
	t.Helper()

	path := filepath.Join(append([]string{proxyRepoRoot(t)}, parts...)...)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}

	return string(data)
}

func requireRepoPathMissing(t *testing.T, parts ...string) {
	t.Helper()

	path := filepath.Join(append([]string{proxyRepoRoot(t)}, parts...)...)
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected %s to be removed", path)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat %s: %v", path, err)
	}
}

func TestProxyEnhancementArtifactsRemoved(t *testing.T) {
	dockerfile := readProxyRepoFile(t, "Dockerfile")
	qwenAuth := readProxyRepoFile(t, "internal", "auth", "qwen", "qwen_auth.go")

	requireRepoPathMissing(t, "deploy-hf.ps1")
	requireRepoPathMissing(t, "deploy", "hf-profile")
	requireRepoPathMissing(t, "xray-config.json")
	requireRepoPathMissing(t, "start.sh")
	requireRepoPathMissing(t, "docs", "upstream-merge-checklist.md")

	forbiddenDockerSnippets := []string{
		"xray",
		`socks5://127.0.0.1:10808`,
		`CMD ["./start.sh"]`,
		"ENV DEPLOY=cloud",
	}
	for _, snippet := range forbiddenDockerSnippets {
		if strings.Contains(dockerfile, snippet) {
			t.Fatalf("Dockerfile must not contain %q", snippet)
		}
	}

	forbiddenQwenSnippets := []string{
		"HelloChrome_Auto",
		"X-Dashscope-Useragent",
		"generateRequestID",
		"Alibaba Cloud WAF",
	}
	for _, snippet := range forbiddenQwenSnippets {
		if strings.Contains(qwenAuth, snippet) {
			t.Fatalf("internal/auth/qwen/qwen_auth.go must not contain %q", snippet)
		}
	}
}
