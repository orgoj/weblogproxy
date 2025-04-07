package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// GenerateToken creates a security token for the given site ID and GTM ID
// expirationDuration should be a positive duration.
func GenerateToken(secret, siteID, gtmID string, expirationDuration time.Duration) (string, error) {
	if secret == "" {
		return "", fmt.Errorf("secret cannot be empty")
	}
	if expirationDuration <= 0 {
		return "", fmt.Errorf("expirationDuration must be positive")
	}

	// Create a timestamp for token expiration
	// Timestamp is now stored directly in the token for validation.
	expiresAt := time.Now().Add(expirationDuration).Unix()

	// Create the message to sign
	// Include expiration time in the signed message
	message := fmt.Sprintf("%s:%s:%d", siteID, gtmID, expiresAt)

	// Create HMAC
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(message))
	signature := hex.EncodeToString(h.Sum(nil))

	// Return the token as expiresAt:signature
	return fmt.Sprintf("%d:%s", expiresAt, signature), nil
}

// ValidateToken validates a security token
// expirationWindow allows for a small time drift or processing delay.
func ValidateToken(secret, siteID, gtmID, token string) (bool, error) {
	if secret == "" {
		return false, fmt.Errorf("secret cannot be empty")
	}

	// Split the token into timestamp (expiresAt) and signature
	var expiresAt int64
	var signature string
	_, err := fmt.Sscanf(token, "%d:%s", &expiresAt, &signature)
	if err != nil {
		return false, fmt.Errorf("invalid token format: %w", err)
	}

	// Check if the token has expired using more precise time comparison
	expirationTime := time.Unix(expiresAt, 0)
	if time.Now().After(expirationTime) {
		return false, fmt.Errorf("token has expired (expired at %s)", expirationTime.Format(time.RFC3339))
	}

	// Recreate the message
	// The message MUST match the one used during generation (including expiresAt)
	message := fmt.Sprintf("%s:%s:%d", siteID, gtmID, expiresAt)

	// Create HMAC
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(message))
	expectedSignature := hex.EncodeToString(h.Sum(nil))

	// Compare signatures
	if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
		return false, nil
	}

	return true, nil
}
