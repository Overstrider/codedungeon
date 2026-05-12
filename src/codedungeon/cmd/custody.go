package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"

	"github.com/loldinis/codedungeon/internal/db"
)

const (
	envRunID        = "CODEDUNGEON_RUN_ID"
	envSessionID    = "CODEDUNGEON_SESSION_ID"
	envSessionToken = "CODEDUNGEON_SESSION_TOKEN"
)

func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func requireAutonomousCustody(s *db.Store, runID int64, action string) error {
	sess, err := s.ActiveRunSession(runID)
	if err != nil {
		return EmitErr("autonomous-session-check failed: "+err.Error(), "")
	}
	if sess == nil {
		return nil
	}
	if strings.EqualFold(sess.Status, runSessionWaitingForAgent) {
		return nil
	}
	if os.Getenv(envSessionID) != sess.ID || hashSessionToken(os.Getenv(envSessionToken)) != sess.TokenSHA256 {
		return EmitErr("autonomous-session-required",
			action+" is locked to codedungeon runner session "+sess.ID)
	}
	return nil
}
