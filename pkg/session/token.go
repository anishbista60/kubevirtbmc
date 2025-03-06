package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
)

var ts TokenStore

type TokenStore struct {
	rwMutex sync.RWMutex
	store   map[string]TokenInfo
}

type TokenInfo struct {
	ID       string
	Username string
}

func NewTokenInfo(id, username string) TokenInfo {
	return TokenInfo{
		ID:       id,
		Username: username,
	}
}

func init() {
	ts = TokenStore{
		store: make(map[string]TokenInfo, 1),
	}

}

func generateToken(tokenInfo TokenInfo) string {
	tokenJSON, _ := json.Marshal(tokenInfo)

	hash := sha256.Sum256(tokenJSON)
	return hex.EncodeToString(hash[:])
}

func AddToken(tokenInfo TokenInfo) string {
	ts.rwMutex.Lock()
	defer ts.rwMutex.Unlock()

	token := generateToken(tokenInfo)
	ts.store[token] = tokenInfo

	return token
}

func GetToken(token string) (TokenInfo, bool) {
	ts.rwMutex.RLock()
	defer ts.rwMutex.RUnlock()

	tokenInfo, exists := ts.store[token]

	return tokenInfo, exists
}

func RemoveToken(token string) {
	ts.rwMutex.Lock()
	defer ts.rwMutex.Unlock()

	delete(ts.store, token)
}

func GetTokenFromSessionID(sessionID string) (TokenInfo, bool) {
	ts.rwMutex.RLock()
	defer ts.rwMutex.RUnlock()

	for _, tokenInfo := range ts.store {
		if tokenInfo.ID == sessionID {
			return tokenInfo, true
		}
	}

	return TokenInfo{}, false
}

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try token-based authentication first
		token := r.Header.Get("X-Auth-Token")
		if token != "" {
			ts.rwMutex.RLock()
			_, exists := ts.store[token]
			ts.rwMutex.RUnlock()

			if exists {
				next.ServeHTTP(w, r)
				return
			}
		}

		// If token auth fails, try basic auth
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			username, password, ok := ParseBasicAuth(authHeader)
			if ok && ValidateBasicAuth(username, password) {
				// Basic auth credentials are valid
				next.ServeHTTP(w, r)
				return
			}
		}

		// If both authentication methods fail
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})
}
