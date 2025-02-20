// basic-auth.go
package session

import (
	"net/http"
	"sync"

	"kubevirt.io/kubevirtbmc/pkg/secret"
)

var (
	credentialsMutex sync.RWMutex
	credentials      *secret.AuthCredentials
)

// SetBasicAuthCredentials sets the credentials to use for basic auth
func SetBasicAuthCredentials(creds *secret.AuthCredentials) {
	credentialsMutex.Lock()
	defer credentialsMutex.Unlock()
	credentials = creds
}

// GetBasicAuthCredentials returns the current basic auth credentials
func GetBasicAuthCredentials() *secret.AuthCredentials {
	credentialsMutex.RLock()
	defer credentialsMutex.RUnlock()
	return credentials
}

// ValidateBasicAuth validates the provided username and password
func ValidateBasicAuth(username, password string) bool {
	credentialsMutex.RLock()
	defer credentialsMutex.RUnlock()

	if credentials == nil {
		return false
	}

	return username == credentials.Username && password == credentials.Password
}

// BasicAuthMiddleware is middleware that checks for basic authentication
func BasicAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, hasBasicAuth := r.BasicAuth()
		if hasBasicAuth && ValidateBasicAuth(username, password) {
			next.ServeHTTP(w, r)
			return
		}

		// Fall through to next middleware (which might be token auth)
		next.ServeHTTP(w, r)
	})
}
