package management

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/store"
)

type gitSyncStatusProvider interface {
	SyncStatus() store.GitSyncStatus
}

type gitSyncFlusher interface {
	Flush(context.Context) error
}

func (h *Handler) gitSyncBackend() (gitSyncStatusProvider, gitSyncFlusher, bool) {
	backend := h.tokenStoreWithBaseDir()
	if backend == nil {
		return nil, nil, false
	}
	statusProvider, okStatus := backend.(gitSyncStatusProvider)
	flusher, okFlush := backend.(gitSyncFlusher)
	return statusProvider, flusher, okStatus || okFlush
}

func (h *Handler) GetGitStoreStatus(c *gin.Context) {
	statusProvider, _, ok := h.gitSyncBackend()
	if !ok || statusProvider == nil {
		c.JSON(http.StatusOK, gin.H{"enabled": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"enabled": true, "status": statusProvider.SyncStatus()})
}

func (h *Handler) FlushGitStore(c *gin.Context) {
	statusProvider, flusher, ok := h.gitSyncBackend()
	if !ok || flusher == nil {
		c.JSON(http.StatusNotFound, gin.H{"enabled": false, "error": "git store sync is not available"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
	defer cancel()
	if err := flusher.Flush(ctx); err != nil {
		body := gin.H{"enabled": true, "ok": false, "error": err.Error()}
		if statusProvider != nil {
			body["status"] = statusProvider.SyncStatus()
		}
		c.JSON(http.StatusBadGateway, body)
		return
	}
	body := gin.H{"enabled": true, "ok": true}
	if statusProvider != nil {
		body["status"] = statusProvider.SyncStatus()
	}
	c.JSON(http.StatusOK, body)
}
