package auth

import (
	"crypto/rand"
	"errors"
	"sync"
	"time"
)

const TokenSize = 32

var ErrInvalidToken = errors.New("invalid token")

type Record struct {
	SessionID uint64
	IssuedAt  time.Time
}

type TokenManager struct {
	mu      sync.RWMutex
	records map[[TokenSize]byte]Record
	maxAge  time.Duration
}

func NewTokenManager(maxAge time.Duration) *TokenManager {
	return &TokenManager{
		records: make(map[[TokenSize]byte]Record),
		maxAge:  maxAge,
	}
}

func (m *TokenManager) Generate(sessionID uint64, now time.Time) ([TokenSize]byte, error) {
	var token [TokenSize]byte
	if _, err := rand.Read(token[:]); err != nil {
		return [TokenSize]byte{}, err
	}
	m.mu.Lock()
	m.records[token] = Record{SessionID: sessionID, IssuedAt: now}
	m.mu.Unlock()
	return token, nil
}

func (m *TokenManager) Validate(token [TokenSize]byte) (uint64, bool) {
	m.mu.RLock()
	rec, ok := m.records[token]
	m.mu.RUnlock()
	return rec.SessionID, ok
}

func (m *TokenManager) RotateIfExpired(token [TokenSize]byte, now time.Time) ([TokenSize]byte, bool, uint64, error) {
	m.mu.RLock()
	rec, ok := m.records[token]
	m.mu.RUnlock()
	if !ok {
		return [TokenSize]byte{}, false, 0, ErrInvalidToken
	}

	if now.Sub(rec.IssuedAt) < m.maxAge {
		return token, false, rec.SessionID, nil
	}

	newToken, err := m.Generate(rec.SessionID, now)
	if err != nil {
		return [TokenSize]byte{}, false, 0, err
	}

	m.mu.Lock()
	delete(m.records, token)
	m.mu.Unlock()

	return newToken, true, rec.SessionID, nil
}
