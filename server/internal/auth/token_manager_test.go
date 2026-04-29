package auth

import (
	"testing"
	"time"
)

func TestGenerateValidate(t *testing.T) {
	now := time.Unix(1700000000, 0)
	mgr := NewTokenManager(30 * 24 * time.Hour)

	token, err := mgr.Generate(88, now)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	sessionID, ok := mgr.Validate(token)
	if !ok || sessionID != 88 {
		t.Fatalf("Validate mismatch: ok=%v sessionID=%d", ok, sessionID)
	}
}

func TestRotateIfExpired(t *testing.T) {
	now := time.Unix(1700000000, 0)
	mgr := NewTokenManager(24 * time.Hour)

	token, err := mgr.Generate(99, now)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	unchanged, rotated, sessionID, err := mgr.RotateIfExpired(token, now.Add(12*time.Hour))
	if err != nil {
		t.Fatalf("RotateIfExpired unexpired: %v", err)
	}
	if rotated || unchanged != token || sessionID != 99 {
		t.Fatalf("unexpected unexpired rotation result: rotated=%v session=%d", rotated, sessionID)
	}

	newToken, rotated, sessionID, err := mgr.RotateIfExpired(token, now.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("RotateIfExpired expired: %v", err)
	}
	if !rotated || newToken == token || sessionID != 99 {
		t.Fatalf("unexpected expired rotation result: rotated=%v session=%d", rotated, sessionID)
	}
	if _, ok := mgr.Validate(token); ok {
		t.Fatalf("old token still valid after rotation")
	}
}
