package management

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

func TestListAuthFiles_UsesDiskEntriesWhenManagerIsEmpty(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	authDir := t.TempDir()
	fileName := "codex-user@example.com-plus.json"
	filePath := filepath.Join(authDir, fileName)
	if err := os.WriteFile(filePath, []byte(`{"type":"codex","email":"user@example.com"}`), 0o600); err != nil {
		t.Fatalf("failed to write auth file: %v", err)
	}

	manager := coreauth.NewManager(nil, nil, nil)
	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: authDir}, manager)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/auth-files", nil)

	h.ListAuthFiles(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected list status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var payload struct {
		Files []map[string]any `json:"files"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode list payload: %v", err)
	}
	if len(payload.Files) != 1 {
		t.Fatalf("expected 1 auth file entry, got %d: %#v", len(payload.Files), payload.Files)
	}

	entry := payload.Files[0]
	if got, _ := entry["name"].(string); got != fileName {
		t.Fatalf("expected name %q, got %#v", fileName, entry["name"])
	}
	if got, _ := entry["type"].(string); got != "codex" {
		t.Fatalf("expected type codex, got %#v", entry["type"])
	}
	if got, _ := entry["email"].(string); got != "user@example.com" {
		t.Fatalf("expected email user@example.com, got %#v", entry["email"])
	}
	if got, _ := entry["source"].(string); got != "file" {
		t.Fatalf("expected source file, got %#v", entry["source"])
	}
	if got, _ := entry["auth_index"].(string); got == "" {
		t.Fatalf("expected auth_index to be populated, entry: %#v", entry)
	}
}
