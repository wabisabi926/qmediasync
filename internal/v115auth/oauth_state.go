package v115auth

import (
	"sync"
	"time"

	"Q115-STRM/internal/v115open"
)

type OAuthStateInfo struct {
	AccountID   string
	State       string
	Provider    AuthProvider
	RedirectURL string
	CreatedAt   time.Time
}

type QrCodeStateInfo struct {
	AccountID     string
	UID           string
	CodeVerifier  string
	QrTime        int64
	Sign          string
	AppID         string
	AppIDName     string
	CreatedAt     time.Time
	Status        string
	Token         *v115open.TokenData
}

type AuthStateManager struct {
	mu            sync.RWMutex
	oauthStates   map[string]*OAuthStateInfo
	qrCodeStates  map[string]*QrCodeStateInfo
	cleanupTicker *time.Ticker
	stopChan      chan struct{}
}

var GlobalAuthStateManager = NewAuthStateManager()

func NewAuthStateManager() *AuthStateManager {
	m := &AuthStateManager{
		oauthStates:  make(map[string]*OAuthStateInfo),
		qrCodeStates: make(map[string]*QrCodeStateInfo),
		stopChan:     make(chan struct{}),
	}
	m.startCleanup()
	return m
}

func (m *AuthStateManager) startCleanup() {
	m.cleanupTicker = time.NewTicker(5 * time.Minute)
	go func() {
		for {
			select {
			case <-m.cleanupTicker.C:
				m.cleanupExpired()
			case <-m.stopChan:
				m.cleanupTicker.Stop()
				return
			}
		}
	}()
}

func (m *AuthStateManager) Stop() {
	close(m.stopChan)
}

func (m *AuthStateManager) cleanupExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	expiry := 30 * time.Minute

	for key, state := range m.oauthStates {
		if now.Sub(state.CreatedAt) > expiry {
			delete(m.oauthStates, key)
		}
	}

	for key, state := range m.qrCodeStates {
		if now.Sub(state.CreatedAt) > expiry {
			delete(m.qrCodeStates, key)
		}
	}
}

func (m *AuthStateManager) SetOAuthState(state, accountID string, provider AuthProvider, redirectURL string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.oauthStates[state] = &OAuthStateInfo{
		AccountID:   accountID,
		State:       state,
		Provider:    provider,
		RedirectURL: redirectURL,
		CreatedAt:   time.Now(),
	}
}

func (m *AuthStateManager) GetOAuthState(state string) (*OAuthStateInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	info, ok := m.oauthStates[state]
	if !ok {
		return nil, false
	}

	if time.Since(info.CreatedAt) > 30*time.Minute {
		return nil, false
	}

	return info, true
}

func (m *AuthStateManager) GetOAuthStateWithProvider(state string, provider AuthProvider) (*OAuthStateInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	info, ok := m.oauthStates[state]
	if !ok {
		return nil, false
	}

	if time.Since(info.CreatedAt) > 30*time.Minute {
		return nil, false
	}

	if info.Provider != provider {
		return nil, false
	}

	return info, true
}

func (m *AuthStateManager) DeleteOAuthState(state string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.oauthStates, state)
}

func (m *AuthStateManager) SetQrCodeState(accountID, uid, codeVerifier string, qrTime int64, sign, appID, appIDName string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.qrCodeStates[uid] = &QrCodeStateInfo{
		AccountID:    accountID,
		UID:          uid,
		CodeVerifier: codeVerifier,
		QrTime:       qrTime,
		Sign:         sign,
		AppID:        appID,
		AppIDName:    appIDName,
		CreatedAt:    time.Now(),
		Status:       "waiting",
	}
}

func (m *AuthStateManager) GetQrCodeState(uid string) (*QrCodeStateInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	info, ok := m.qrCodeStates[uid]
	if !ok {
		return nil, false
	}

	if time.Since(info.CreatedAt) > 30*time.Minute {
		return nil, false
	}

	return info, true
}

func (m *AuthStateManager) UpdateQrCodeStatus(uid, status string, token *v115open.TokenData) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if info, ok := m.qrCodeStates[uid]; ok {
		info.Status = status
		if token != nil {
			info.Token = token
		}
	}
}

func (m *AuthStateManager) DeleteQrCodeState(uid string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.qrCodeStates, uid)
}

func (m *AuthStateManager) GetQrCodeStateByAccountID(accountID string) (*QrCodeStateInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, info := range m.qrCodeStates {
		if info.AccountID == accountID {
			if time.Since(info.CreatedAt) > 30*time.Minute {
				return nil, false
			}
			return info, true
		}
	}

	return nil, false
}
