package managementasset

import (
	"strings"
	"testing"
)

func TestPatchManagementHTML_InsertsBeforeHead(t *testing.T) {
	input := []byte("<html><head><title>test</title></head><body><div id='root'></div></body></html>")

	got := string(PatchManagementHTML(input))

	if !strings.Contains(got, managementSessionPatchMarker) {
		t.Fatalf("expected injected marker in patched html")
	}
	if strings.Count(got, managementSessionPatchMarker) != 1 {
		t.Fatalf("expected injected marker once, got %d", strings.Count(got, managementSessionPatchMarker))
	}

	headClose := strings.Index(strings.ToLower(got), "</head>")
	markerIdx := strings.Index(got, managementSessionPatchMarker)
	if markerIdx < 0 || headClose < 0 || markerIdx > headClose {
		t.Fatalf("expected patch marker before </head>: marker=%d headClose=%d", markerIdx, headClose)
	}
}

func TestPatchManagementHTML_InsertsBeforeFirstScriptInHead(t *testing.T) {
	input := []byte("<html><head><script>window.appBooted=true;</script><title>test</title></head><body></body></html>")

	got := string(PatchManagementHTML(input))

	if !strings.Contains(got, managementSessionPatchMarker) {
		t.Fatalf("expected injected marker in patched html")
	}

	scriptIdx := strings.Index(strings.ToLower(got), "<script>")
	markerIdx := strings.Index(got, managementSessionPatchMarker)
	if markerIdx < 0 || scriptIdx < 0 || markerIdx > scriptIdx {
		t.Fatalf("expected patch marker before first <script>: marker=%d script=%d", markerIdx, scriptIdx)
	}
}

func TestPatchManagementHTML_IncludesRouteFlashGuardScript(t *testing.T) {
	input := []byte("<html><head></head><body><div id='root'></div></body></html>")

	got := string(PatchManagementHTML(input))

	if !strings.Contains(got, "__cliproxyRouteFlashGuardV1") {
		t.Fatalf("expected route flash guard script in patched html")
	}
}

func TestManagementSessionPatchScript_HydratesPersistedAuthState(t *testing.T) {
	if !strings.Contains(managementSessionPatchScript, "key === 'cli-proxy-auth'") {
		t.Fatalf("expected session patch to intercept cli-proxy-auth reads")
	}
	if !strings.Contains(managementSessionPatchScript, "parsed.state.apiUrl = session.apiBase") {
		t.Fatalf("expected session patch to restore the api base into persisted auth state")
	}
	if strings.Contains(managementSessionPatchScript, "parsed.state.isAuthenticated = true") {
		t.Fatalf("expected session patch to stop faking authenticated state during refresh")
	}
	if strings.Contains(managementSessionPatchScript, "parsed.state.managementKey = session.managementKey") {
		t.Fatalf("expected session patch to keep the management key out of persisted auth hydration")
	}
}

func TestManagementSessionPatchScript_DefaultsToSessionOnlyUntilRememberPasswordIsEnabled(t *testing.T) {
	if !strings.Contains(managementSessionPatchScript, "writeSession(apiBase, match[1], current ? current.sessionOnly : true);") {
		t.Fatalf("expected new sessions to default to session-only mode")
	}
	if !strings.Contains(managementSessionPatchScript, "return parsed.state.rememberPassword === true;") {
		t.Fatalf("expected persistent mode to require rememberPassword=true")
	}
	if strings.Contains(managementSessionPatchScript, "typeof parsed.state.managementKey === 'string'") {
		t.Fatalf("expected managementKey presence alone to no longer disable session-only mode")
	}
}

func TestManagementSessionPatchScript_NormalizesManagementRequestURLs(t *testing.T) {
	if !strings.Contains(managementSessionPatchScript, "function normalizeManagementRequestUrl(requestUrl)") {
		t.Fatalf("expected request URL normalization helper in session patch")
	}
	if !strings.Contains(managementSessionPatchScript, "pathname = pathname.replace(/\\/management\\.html(?=\\/|$)/ig, '');") {
		t.Fatalf("expected request URL normalization to strip /management.html from request paths")
	}
	if !strings.Contains(managementSessionPatchScript, "pathname = pathname.replace(/\\/v0\\/management\\/v0\\/management(?=\\/|$)/ig, '/v0/management');") {
		t.Fatalf("expected request URL normalization to collapse duplicated management prefixes")
	}
}

func TestPatchManagementHTML_IsIdempotent(t *testing.T) {
	input := []byte("<html><head></head><body></body></html>")

	once := PatchManagementHTML(input)
	twice := PatchManagementHTML(once)

	if string(once) != string(twice) {
		t.Fatalf("expected patching to be idempotent")
	}
}

func TestPatchManagementHTML_FallsBackToBody(t *testing.T) {
	input := []byte("<html><body><div id='root'></div></body></html>")

	got := string(PatchManagementHTML(input))

	if !strings.Contains(got, managementSessionPatchMarker) {
		t.Fatalf("expected injected marker in body fallback html")
	}

	bodyClose := strings.Index(strings.ToLower(got), "</body>")
	markerIdx := strings.Index(got, managementSessionPatchMarker)
	if markerIdx < 0 || bodyClose < 0 || markerIdx > bodyClose {
		t.Fatalf("expected patch marker before </body>: marker=%d bodyClose=%d", markerIdx, bodyClose)
	}
}
