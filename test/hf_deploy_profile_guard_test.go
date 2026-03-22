package test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve caller path")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}

func readRepoFile(t *testing.T, parts ...string) string {
	t.Helper()

	path := filepath.Join(append([]string{repoRoot(t)}, parts...)...)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}

	return string(data)
}

func requireContains(t *testing.T, label, content string, subs ...string) {
	t.Helper()

	for _, sub := range subs {
		if !strings.Contains(content, sub) {
			t.Fatalf("%s must contain %q", label, sub)
		}
	}
}

func requireNotContains(t *testing.T, label, content string, subs ...string) {
	t.Helper()

	for _, sub := range subs {
		if strings.Contains(content, sub) {
			t.Fatalf("%s must not contain %q", label, sub)
		}
	}
}

func TestHFDeployProfileGuard(t *testing.T) {
	deployScript := readRepoFile(t, "deploy-hf.ps1")
	gitAttributes := readRepoFile(t, ".gitattributes")
	overlayDockerfile := readRepoFile(t, "deploy", "hf-profile", "Dockerfile")
	overlayStartScript := readRepoFile(t, "deploy", "hf-profile", "start.sh")
	qwenAuth := readRepoFile(t, "internal", "auth", "qwen", "qwen_auth.go")
	managementPatch := readRepoFile(t, "internal", "managementasset", "updater.go")

	requireContains(
		t,
		"deploy-hf.ps1",
		deployScript,
		"deploy",
		"hf-profile",
		"xray-config.json",
	)
	requireContains(t, ".gitattributes", gitAttributes, "*.sh text eol=lf")

	requireContains(
		t,
		"deploy/hf-profile/Dockerfile",
		overlayDockerfile,
		"mihomo",
		"curl",
		`socks5://127.0.0.1:10808`,
	)
	requireNotContains(
		t,
		"deploy/hf-profile/Dockerfile",
		overlayDockerfile,
		"Xray-linux-64.zip",
		"xray-config.json",
	)

	requireContains(
		t,
		"deploy/hf-profile/start.sh",
		overlayStartScript,
		"CLASH_SUB_URL",
		"fail_proxy_bootstrap",
		"exit 1",
		"Clash.Meta",
		"--socks5-hostname 127.0.0.1:10808",
		"chat.qwen.ai/api/v1/oauth2/device/code",
		"grep '^ *- name:'",
		`proxy-url: "socks5://127.0.0.1:10808"`,
		"DOMAIN,portal.qwen.ai,DIRECT",
		"DOMAIN-SUFFIX,qwen.ai,auto",
	)
	requireNotContains(
		t,
		"deploy/hf-profile/start.sh",
		overlayStartScript,
		"QWEN_AUTH_PROXY_URL",
		"start_without_proxy",
		"xray run",
	)

	requireContains(t, "internal/auth/qwen/qwen_auth.go", qwenAuth, "cfg.ProxyURL")
	requireNotContains(t, "internal/auth/qwen/qwen_auth.go", qwenAuth, "QWEN_AUTH_PROXY_URL")

	requireContains(
		t,
		"internal/managementasset/updater.go",
		managementPatch,
		"__cliproxyRouteFlashGuardV1",
		"writeSession(apiBase, match[1], current ? current.sessionOnly : true);",
		"return parsed.state.rememberPassword === true;",
		"parsed.state.apiUrl = session.apiBase;",
	)
	requireNotContains(
		t,
		"internal/managementasset/updater.go",
		managementPatch,
		"parsed.state.isAuthenticated = true;",
		"parsed.state.managementKey = session.managementKey;",
		"typeof parsed.state.managementKey === 'string'",
	)
}
