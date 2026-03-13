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
