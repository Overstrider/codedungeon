package taskexec

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/loldinis/codedungeon/internal/db"
)

type SessionStore struct {
	store      *db.Store
	sessionDir string
	ttlHours   int
	now        func() time.Time
}

type StartSessionRequest struct {
	RunID     int64
	TaskID    string
	TaskPath  string
	Provider  string
	OutputDir string
}

func NewSessionStore(store *db.Store, sessionDir string, ttlHours int) *SessionStore {
	if ttlHours <= 0 {
		ttlHours = 24
	}
	return &SessionStore{
		store:      store,
		sessionDir: sessionDir,
		ttlHours:   ttlHours,
		now:        time.Now,
	}
}

func (s *SessionStore) Start(req StartSessionRequest) (*db.ExecutionSession, error) {
	id, err := randomSessionID()
	if err != nil {
		return nil, err
	}
	now := s.now().Unix()
	if req.OutputDir == "" {
		req.OutputDir = filepath.Join(s.sessionDir, id)
	}
	sess := db.ExecutionSession{
		ID:        id,
		RunID:     req.RunID,
		TaskID:    req.TaskID,
		TaskPath:  req.TaskPath,
		Provider:  firstNonEmpty(req.Provider, "codex"),
		Status:    StatusRunning,
		OutputDir: req.OutputDir,
		StartedAt: now,
		UpdatedAt: now,
		ExpiresAt: now + int64(s.ttlHours*60*60),
	}
	if err := os.MkdirAll(sess.OutputDir, 0o755); err != nil {
		return nil, err
	}
	if err := s.store.UpsertExecutionSession(sess); err != nil {
		return nil, err
	}
	if _, err := s.store.InsertExecutionTransition(db.ExecutionTransition{
		SessionID: sess.ID,
		ToStatus:  StatusRunning,
		Reason:    "session started",
	}); err != nil {
		return nil, err
	}
	if err := writeJSONFile(filepath.Join(sess.OutputDir, "session.json"), sess); err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *SessionStore) Resume(id string, reset bool) (*db.ExecutionSession, error) {
	if id == "" {
		return nil, fmt.Errorf("--resume requires an explicit session id")
	}
	sess, err := s.store.ExecutionSession(id)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, fmt.Errorf("execution session not found: %s", id)
	}
	now := s.now().Unix()
	expired := sess.ExpiresAt > 0 && now > sess.ExpiresAt
	if expired && !reset {
		return nil, fmt.Errorf("execution session expired: %s", id)
	}
	if reset {
		old := sess.Status
		sess.Status = StatusRunning
		sess.Attempt = 0
		sess.FailureMessage = ""
		sess.FinishedAt = 0
		sess.UpdatedAt = now
		sess.ExpiresAt = now + int64(s.ttlHours*60*60)
		if err := s.store.UpsertExecutionSession(*sess); err != nil {
			return nil, err
		}
		if _, err := s.store.InsertExecutionTransition(db.ExecutionTransition{
			SessionID:  sess.ID,
			FromStatus: old,
			ToStatus:   StatusRunning,
			Reason:     "manual reset",
		}); err != nil {
			return nil, err
		}
	}
	return sess, nil
}

func randomSessionID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "exec-" + hex.EncodeToString(buf), nil
}
