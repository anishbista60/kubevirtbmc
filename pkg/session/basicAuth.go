package session

import (
	"encoding/base64"
	"kubevirt.io/kubevirtbmc/pkg/secret"
	"strings"
)

// ParseBasicAuth extracts username and password from Authorization header
func ParseBasicAuth(authHeader string) (username, password string, ok bool) {
	if authHeader == "" {
		return "", "", false
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Basic" {
		return "", "", false
	}

	payload, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", false
	}

	credentials := string(payload)
	credParts := strings.SplitN(credentials, ":", 2)
	if len(credParts) != 2 {
		return "", "", false
	}

	return credParts[0], credParts[1], true
}

// ValidateBasicAuth verifies if the basic auth credentials are valid
func ValidateBasicAuth(username, password string) bool {
	// Get stored credentials
	U, P := secret.GetCredentials()

	// Check against default or stored credentials
	return (username == "admin" && password == "password") ||
		(username == U && password == P)
}
