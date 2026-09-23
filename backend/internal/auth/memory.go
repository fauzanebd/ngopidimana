package auth

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MemoryStore is an in-memory Store for tests and local experimentation. It
// mirrors ingestion.NewMemoryStore: same contract, no database.
type MemoryStore struct {
	mutex        sync.Mutex
	contributors map[string]Contributor
	loginTokens  map[string]memoryLoginToken
	sessions     map[string]memorySession
}

type memoryLoginToken struct {
	email      string
	createdAt  time.Time
	expiresAt  time.Time
	consumedAt time.Time
}

type memorySession struct {
	email     string
	expiresAt time.Time
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		contributors: map[string]Contributor{},
		loginTokens:  map[string]memoryLoginToken{},
		sessions:     map[string]memorySession{},
	}
}

func (store *MemoryStore) Contributor(_ context.Context, email string) (Contributor, bool, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	contributor, found := store.contributors[email]
	return contributor, found, nil
}

func (store *MemoryStore) ListContributors(_ context.Context) ([]Contributor, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	contributors := []Contributor{}
	for _, contributor := range store.contributors {
		contributors = append(contributors, contributor)
	}
	return contributors, nil
}

func (store *MemoryStore) AddContributor(_ context.Context, email, role, displayName string) (Contributor, error) {
	normalized, valid := NormalizeEmail(email)
	if !valid {
		return Contributor{}, fmt.Errorf("%q is not a usable email address", email)
	}
	if role != "contributor" && role != "owner" {
		return Contributor{}, fmt.Errorf("role must be contributor or owner, got %q", role)
	}
	contributor := Contributor{Email: normalized, Role: role, DisplayName: displayName}
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.contributors[normalized] = contributor
	return contributor, nil
}

func (store *MemoryStore) RemoveContributor(_ context.Context, email string) (bool, error) {
	normalized, valid := NormalizeEmail(email)
	if !valid {
		return false, fmt.Errorf("%q is not a usable email address", email)
	}
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if _, found := store.contributors[normalized]; !found {
		return false, nil
	}
	delete(store.contributors, normalized)
	for hash, token := range store.loginTokens {
		if token.email == normalized {
			delete(store.loginTokens, hash)
		}
	}
	for hash, session := range store.sessions {
		if session.email == normalized {
			delete(store.sessions, hash)
		}
	}
	return true, nil
}

func (store *MemoryStore) CountRecentLoginTokens(_ context.Context, email string, since time.Time) (int, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	count := 0
	for _, token := range store.loginTokens {
		if token.email == email && token.createdAt.After(since) {
			count++
		}
	}
	return count, nil
}

func (store *MemoryStore) SaveLoginToken(_ context.Context, tokenHash []byte, email string, expiresAt time.Time) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.loginTokens[string(tokenHash)] = memoryLoginToken{email: email, createdAt: time.Now(), expiresAt: expiresAt}
	return nil
}

func (store *MemoryStore) ConsumeLoginToken(_ context.Context, tokenHash []byte, now time.Time) (Contributor, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	token, found := store.loginTokens[string(tokenHash)]
	if !found || !token.consumedAt.IsZero() || !token.expiresAt.After(now) {
		return Contributor{}, ErrInvalidToken
	}
	contributor, found := store.contributors[token.email]
	if !found {
		return Contributor{}, ErrInvalidToken
	}
	token.consumedAt = now
	store.loginTokens[string(tokenHash)] = token
	return contributor, nil
}

func (store *MemoryStore) CreateSession(_ context.Context, tokenHash []byte, email string, expiresAt time.Time, _ string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.sessions[string(tokenHash)] = memorySession{email: email, expiresAt: expiresAt}
	return nil
}

func (store *MemoryStore) SessionContributor(_ context.Context, tokenHash []byte, now time.Time) (Contributor, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	session, found := store.sessions[string(tokenHash)]
	if !found || !session.expiresAt.After(now) {
		return Contributor{}, ErrNoSession
	}
	contributor, found := store.contributors[session.email]
	if !found {
		return Contributor{}, ErrNoSession
	}
	return contributor, nil
}

func (store *MemoryStore) DeleteSession(_ context.Context, tokenHash []byte) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	delete(store.sessions, string(tokenHash))
	return nil
}
