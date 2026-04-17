package management

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func TestGetConfigYAMLFallsBackToCurrentConfigWhenFileMissing(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	tmpDir := t.TempDir()
	handler := &Handler{
		cfg: &config.Config{
			Host: "0.0.0.0",
			Port: 8317,
		},
		configFilePath: filepath.Join(tmpDir, "missing-config.yaml"),
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/config.yaml", nil)

	handler.GetConfigYAML(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); !strings.Contains(got, "application/yaml") {
		t.Fatalf("content-type = %q, want application/yaml", got)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "host: 0.0.0.0") {
		t.Fatalf("body = %q, want host field", body)
	}
	if !strings.Contains(body, "port: 8317") {
		t.Fatalf("body = %q, want port field", body)
	}
}
