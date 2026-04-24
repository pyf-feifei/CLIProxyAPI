package store

import (
	"context"
	"os"
	"sort"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	gitSyncModeAsync        = "async"
	gitSyncModeSync         = "sync"
	gitSyncDefaultDebounce  = 2 * time.Second
	gitSyncDefaultRetry     = 30 * time.Second
	gitSyncWaitPollInterval = 20 * time.Millisecond
)

// GitSyncStatus describes the local queue and last remote push result.
type GitSyncStatus struct {
	Mode         string    `json:"mode"`
	PendingFiles int       `json:"pending_files"`
	NeedsPush    bool      `json:"needs_push"`
	SyncActive   bool      `json:"sync_active"`
	LastSyncAt   time.Time `json:"last_sync_at,omitempty"`
	LastPushAt   time.Time `json:"last_push_at,omitempty"`
	LastError    string    `json:"last_error,omitempty"`
}

func defaultGitSyncMode() string {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("GITSTORE_SYNC_MODE")))
	if mode == gitSyncModeSync {
		return gitSyncModeSync
	}
	return gitSyncModeAsync
}

func defaultGitSyncDebounce() time.Duration {
	return durationFromEnv("GITSTORE_SYNC_DEBOUNCE", gitSyncDefaultDebounce)
}

func defaultGitSyncRetryInterval() time.Duration {
	return durationFromEnv("GITSTORE_SYNC_RETRY_INTERVAL", gitSyncDefaultRetry)
}

func durationFromEnv(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	d, err := time.ParseDuration(value)
	if err != nil || d < 0 {
		return fallback
	}
	return d
}

func (s *GitTokenStore) enqueueGitSync(message string, relPaths ...string) error {
	if strings.EqualFold(strings.TrimSpace(s.syncMode), gitSyncModeSync) {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.commitAndPushLocked(message, relPaths...)
	}

	s.syncMu.Lock()
	s.initSyncLocked()
	for _, rel := range relPaths {
		trimmed := strings.TrimSpace(rel)
		if trimmed == "" {
			continue
		}
		s.pendingPaths[trimmed] = struct{}{}
	}
	if strings.TrimSpace(message) != "" {
		s.pendingMessages = append(s.pendingMessages, message)
	}
	s.startGitSyncWorkerLocked()
	notify := s.syncNotify
	s.syncMu.Unlock()

	select {
	case notify <- struct{}{}:
	default:
	}
	return nil
}

func (s *GitTokenStore) Flush(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		if err := s.performGitSync(ctx); err != nil {
			return err
		}
		if s.gitSyncDrained() {
			return nil
		}
	}
}

func (s *GitTokenStore) SyncStatus() GitSyncStatus {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	s.initSyncLocked()
	return GitSyncStatus{
		Mode:         s.syncMode,
		PendingFiles: len(s.pendingPaths),
		NeedsPush:    s.needsPush,
		SyncActive:   s.syncActive,
		LastSyncAt:   s.lastSyncAt,
		LastPushAt:   s.lastPushAt,
		LastError:    s.lastError,
	}
}

func (s *GitTokenStore) initSyncLocked() {
	if s.syncNotify == nil {
		s.syncNotify = make(chan struct{}, 1)
	}
	if s.pendingPaths == nil {
		s.pendingPaths = make(map[string]struct{})
	}
	if strings.TrimSpace(s.syncMode) == "" {
		s.syncMode = defaultGitSyncMode()
	}
	if s.syncDebounce == 0 {
		s.syncDebounce = gitSyncDefaultDebounce
	}
	if s.syncRetryInterval == 0 {
		s.syncRetryInterval = gitSyncDefaultRetry
	}
}

func (s *GitTokenStore) startGitSyncWorkerLocked() {
	if s.syncStarted {
		return
	}
	s.syncStarted = true
	notify := s.syncNotify
	go s.gitSyncLoop(notify)
}

func (s *GitTokenStore) gitSyncLoop(notify <-chan struct{}) {
	for range notify {
		debounce := s.currentSyncDebounce()
		if debounce > 0 {
			time.Sleep(debounce)
		}
		for {
			err := s.performGitSync(context.Background())
			if err != nil {
				log.WithError(err).Warn("git token store: background sync failed")
				retry := s.currentSyncRetryInterval()
				if retry > 0 {
					time.Sleep(retry)
				}
				continue
			}
			if s.gitSyncDrained() {
				break
			}
		}
	}
}

func (s *GitTokenStore) performGitSync(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		s.syncMu.Lock()
		s.initSyncLocked()
		if s.syncActive {
			s.syncMu.Unlock()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(gitSyncWaitPollInterval):
				continue
			}
		}
		s.syncActive = true
		paths := s.pendingPathListLocked()
		message := s.pendingMessageLocked()
		hasWork := len(paths) > 0 || s.needsPush
		s.pendingPaths = make(map[string]struct{})
		s.pendingMessages = nil
		s.syncMu.Unlock()

		if !hasWork {
			s.finishGitSync(nil, nil, "")
			return nil
		}

		err := func() error {
			s.mu.Lock()
			defer s.mu.Unlock()
			return s.commitAndPushLocked(message, paths...)
		}()
		s.finishGitSync(err, paths, message)
		return err
	}
}

func (s *GitTokenStore) finishGitSync(err error, paths []string, message string) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	s.initSyncLocked()
	if err != nil {
		for _, rel := range paths {
			if strings.TrimSpace(rel) != "" {
				s.pendingPaths[rel] = struct{}{}
			}
		}
		if strings.TrimSpace(message) != "" {
			s.pendingMessages = append([]string{message}, s.pendingMessages...)
		}
		s.lastError = err.Error()
	} else {
		s.lastError = ""
		s.lastSyncAt = time.Now()
	}
	s.syncActive = false
}

func (s *GitTokenStore) pendingPathListLocked() []string {
	paths := make([]string, 0, len(s.pendingPaths))
	for rel := range s.pendingPaths {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	return paths
}

func (s *GitTokenStore) pendingMessageLocked() string {
	if len(s.pendingMessages) == 0 {
		return "Sync git token store"
	}
	seen := make(map[string]struct{}, len(s.pendingMessages))
	messages := make([]string, 0, len(s.pendingMessages))
	for _, msg := range s.pendingMessages {
		trimmed := strings.TrimSpace(msg)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		messages = append(messages, trimmed)
		if len(messages) >= 3 {
			break
		}
	}
	if len(messages) == 0 {
		return "Sync git token store"
	}
	if len(messages) == 1 {
		return messages[0]
	}
	return "Batch sync git token store: " + strings.Join(messages, "; ")
}

func (s *GitTokenStore) gitSyncDrained() bool {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	s.initSyncLocked()
	return !s.syncActive && len(s.pendingPaths) == 0 && !s.needsPush
}

func (s *GitTokenStore) currentSyncDebounce() time.Duration {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	s.initSyncLocked()
	return s.syncDebounce
}

func (s *GitTokenStore) currentSyncRetryInterval() time.Duration {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	s.initSyncLocked()
	return s.syncRetryInterval
}

func (s *GitTokenStore) remotePushPending() bool {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	s.initSyncLocked()
	return s.needsPush
}

func (s *GitTokenStore) markRemotePushPending() {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	s.initSyncLocked()
	s.needsPush = true
}

func (s *GitTokenStore) markRemotePushComplete() {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	s.initSyncLocked()
	s.needsPush = false
	s.lastPushAt = time.Now()
}
