package management

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/store"
)

type gitSyncTestStore struct {
	memoryAuthStore
	status  store.GitSyncStatus
	flushed bool
}

func (s *gitSyncTestStore) SyncStatus() store.GitSyncStatus {
	return s.status
}

func (s *gitSyncTestStore) Flush(context.Context) error {
	s.flushed = true
	s.status.PendingFiles = 0
	s.status.NeedsPush = false
	return nil
}

func TestGetGitStoreStatusDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: t.TempDir()}, nil)
	h.tokenStore = &memoryAuthStore{}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/gitstore/status", nil)

	h.GetGitStoreStatus(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusOK)
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if enabled, _ := body["enabled"].(bool); enabled {
		t.Fatalf("enabled = true, want false")
	}
}

func TestFlushGitStoreFlushesBackend(t *testing.T) {
	gin.SetMode(gin.TestMode)
	backend := &gitSyncTestStore{status: store.GitSyncStatus{Mode: "async", PendingFiles: 2, NeedsPush: true}}
	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: t.TempDir()}, nil)
	h.tokenStore = backend

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v0/management/gitstore/flush", nil)

	h.FlushGitStore(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !backend.flushed {
		t.Fatal("backend was not flushed")
	}
	var body struct {
		Enabled bool `json:"enabled"`
		OK      bool `json:"ok"`
		Status  struct {
			PendingFiles int  `json:"pending_files"`
			NeedsPush    bool `json:"needs_push"`
		} `json:"status"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.Enabled || !body.OK {
		t.Fatalf("response enabled/ok = %v/%v, want true/true", body.Enabled, body.OK)
	}
	if body.Status.PendingFiles != 0 || body.Status.NeedsPush {
		t.Fatalf("status after flush = %+v, want drained", body.Status)
	}
}
