package auth

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
)

type refreshStateStore struct {
	saveCount int
	lastSaved *Auth
}

func (s *refreshStateStore) List(context.Context) ([]*Auth, error) { return nil, nil }

func (s *refreshStateStore) Save(_ context.Context, auth *Auth) (string, error) {
	s.saveCount++
	s.lastSaved = auth.Clone()
	return "", nil
}

func (s *refreshStateStore) Delete(context.Context, string) error { return nil }

type refreshStateExecutor struct {
	provider string
	err      error
}

func (e refreshStateExecutor) Identifier() string { return e.provider }

func (e refreshStateExecutor) Execute(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	return cliproxyexecutor.Response{}, nil
}

func (e refreshStateExecutor) ExecuteStream(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error) {
	return nil, nil
}

func (e refreshStateExecutor) Refresh(context.Context, *Auth) (*Auth, error) {
	return nil, e.err
}

func (e refreshStateExecutor) CountTokens(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	return cliproxyexecutor.Response{}, nil
}

func (e refreshStateExecutor) HttpRequest(context.Context, *Auth, *http.Request) (*http.Response, error) {
	return nil, nil
}

func TestAuthRuntimeStateMetadataRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	original := &Auth{
		Status:           StatusError,
		StatusMessage:    "refresh token invalid; sign in again",
		Unavailable:      true,
		Disabled:         false,
		LastRefreshedAt:  now.Add(-time.Hour),
		NextRefreshAfter: now.Add(2 * time.Hour),
		NextRetryAfter:   now.Add(3 * time.Hour),
		LastError: &Error{
			Code:       "reauth_required",
			Message:    "refresh_token_reused",
			Retryable:  false,
			HTTPStatus: 401,
		},
		Metadata: map[string]any{"type": "codex"},
	}

	original.PersistRuntimeStateToMetadata()

	restored := &Auth{Metadata: original.Metadata}
	HydrateRuntimeStateFromMetadata(restored)

	if restored.Status != StatusError {
		t.Fatalf("Status = %q, want %q", restored.Status, StatusError)
	}
	if restored.StatusMessage != original.StatusMessage {
		t.Fatalf("StatusMessage = %q, want %q", restored.StatusMessage, original.StatusMessage)
	}
	if !restored.Unavailable {
		t.Fatalf("Unavailable = false, want true")
	}
	if restored.LastError == nil || restored.LastError.HTTPStatus != 401 {
		t.Fatalf("LastError = %#v, want http_status 401", restored.LastError)
	}
	if !restored.NextRefreshAfter.Equal(original.NextRefreshAfter) {
		t.Fatalf("NextRefreshAfter = %v, want %v", restored.NextRefreshAfter, original.NextRefreshAfter)
	}
	if !restored.NextRetryAfter.Equal(original.NextRetryAfter) {
		t.Fatalf("NextRetryAfter = %v, want %v", restored.NextRetryAfter, original.NextRetryAfter)
	}
	if !restored.LastRefreshedAt.Equal(original.LastRefreshedAt) {
		t.Fatalf("LastRefreshedAt = %v, want %v", restored.LastRefreshedAt, original.LastRefreshedAt)
	}
}

func TestManagerRefreshAuth_PersistsReauthBackoff(t *testing.T) {
	store := &refreshStateStore{}
	manager := NewManager(store, nil, nil)
	manager.RegisterExecutor(refreshStateExecutor{
		provider: "codex",
		err:      fmt.Errorf("token refresh failed with status 401: {\"error\":{\"code\":\"refresh_token_reused\"}}"),
	})

	auth := &Auth{
		ID:       "codex-auth-1",
		Provider: "codex",
		Status:   StatusActive,
		Metadata: map[string]any{
			"type":          "codex",
			"refresh_token": "bad-refresh-token",
		},
	}
	if _, err := manager.Register(context.Background(), auth); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	before := time.Now()
	manager.refreshAuth(context.Background(), auth.ID)

	updated, ok := manager.GetByID(auth.ID)
	if !ok {
		t.Fatalf("GetByID(%q) returned no auth", auth.ID)
	}
	if updated.Status != StatusError {
		t.Fatalf("Status = %q, want %q", updated.Status, StatusError)
	}
	if updated.StatusMessage != "refresh token invalid; sign in again" {
		t.Fatalf("StatusMessage = %q", updated.StatusMessage)
	}
	if updated.Unavailable {
		t.Fatalf("Unavailable = true, want false")
	}
	if updated.NextRefreshAfter.Before(before.Add(23 * time.Hour)) {
		t.Fatalf("NextRefreshAfter = %v, want at least 23h in future", updated.NextRefreshAfter)
	}
	if store.saveCount < 2 {
		t.Fatalf("Save() count = %d, want at least 2", store.saveCount)
	}
	if store.lastSaved == nil {
		t.Fatalf("lastSaved = nil")
	}
	if got := store.lastSaved.Metadata[metadataStatusMessageKey]; got != "refresh token invalid; sign in again" {
		t.Fatalf("saved status_message = %#v", got)
	}
	if _, ok := store.lastSaved.Metadata[metadataNextRefreshAfter]; !ok {
		t.Fatalf("saved metadata missing %q", metadataNextRefreshAfter)
	}
	if _, ok := store.lastSaved.Metadata[metadataLastErrorKey]; !ok {
		t.Fatalf("saved metadata missing %q", metadataLastErrorKey)
	}
}
