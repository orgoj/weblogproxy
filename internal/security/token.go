package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
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
		return false, fmt.Errorf("invalid token")
	}

	// Recreate the message
	// The message MUST match the one used during generation (including expiresAt)
	message := fmt.Sprintf("%s:%s:%d", siteID, gtmID, expiresAt)

	// Create HMAC and validate signature FIRST (constant-time comparison)
	// This prevents timing attacks that could distinguish between expired and invalid tokens
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(message))
	expectedSignature := hex.EncodeToString(h.Sum(nil))

	// Compare signatures using constant-time comparison
	if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
		// Return generic error - don't reveal whether signature or expiration failed
		return false, fmt.Errorf("invalid token")
	}

	// Only check expiration AFTER signature validation succeeds
	// This prevents timing attacks by ensuring all invalid tokens take similar time
	expirationTime := time.Unix(expiresAt, 0)
	if time.Now().After(expirationTime) {
		// Return generic error to prevent information leakage
		return false, fmt.Errorf("invalid token")
	}

	return true, nil
}

// TokenValidationRateLimiter implements rate limiting for token validation attempts
// to prevent brute force attacks on token validation.
type TokenValidationRateLimiter struct {
	attempts      sync.Map // map[string]*validationAttempts
	maxAttempts   int      // Maximum failed attempts before blocking
	blockDuration time.Duration
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}
}

// validationAttempts tracks failed validation attempts for a specific key (typically IP or IP:siteID)
type validationAttempts struct {
	count        int
	firstFail    time.Time
	blockedUntil time.Time
	mu           sync.Mutex
}

// NewTokenValidationRateLimiter creates a new rate limiter for token validation
// maxAttempts: maximum failed attempts before blocking (recommended: 5-10)
// blockDuration: how long to block after exceeding max attempts (recommended: 1-15 minutes)
// cleanupInterval: how often to clean up old entries (recommended: 5 minutes)
func NewTokenValidationRateLimiter(maxAttempts int, blockDuration, cleanupInterval time.Duration) *TokenValidationRateLimiter {
	rl := &TokenValidationRateLimiter{
		maxAttempts:   maxAttempts,
		blockDuration: blockDuration,
		cleanupTicker: time.NewTicker(cleanupInterval),
		stopCleanup:   make(chan struct{}),
	}

	// Start cleanup goroutine
	go rl.cleanupLoop()

	return rl
}

// CheckAndRecordAttempt checks if an IP is blocked and records the attempt
// Returns true if the request should be blocked, false otherwise
func (rl *TokenValidationRateLimiter) CheckAndRecordAttempt(key string, success bool) bool {
	now := time.Now()

	// Load or create attempts entry
	val, _ := rl.attempts.LoadOrStore(key, &validationAttempts{})
	attempt := val.(*validationAttempts)

	attempt.mu.Lock()
	defer attempt.mu.Unlock()

	// Check if currently blocked
	if now.Before(attempt.blockedUntil) {
		return true // Blocked
	}

	// If successful validation, reset counter
	if success {
		attempt.count = 0
		attempt.firstFail = time.Time{}
		attempt.blockedUntil = time.Time{}
		return false
	}

	// Record failed attempt
	if attempt.count == 0 {
		attempt.firstFail = now
	}
	attempt.count++

	// Check if exceeded max attempts
	if attempt.count >= rl.maxAttempts {
		attempt.blockedUntil = now.Add(rl.blockDuration)
		return true // Now blocked
	}

	return false // Not blocked yet
}

// cleanupLoop periodically removes old entries
func (rl *TokenValidationRateLimiter) cleanupLoop() {
	for {
		select {
		case <-rl.cleanupTicker.C:
			rl.cleanup()
		case <-rl.stopCleanup:
			rl.cleanupTicker.Stop()
			return
		}
	}
}

// cleanup removes entries that are no longer blocked and have no recent failures
func (rl *TokenValidationRateLimiter) cleanup() {
	now := time.Now()
	rl.attempts.Range(func(key, value interface{}) bool {
		attempt := value.(*validationAttempts)
		attempt.mu.Lock()
		// Remove if not blocked and no failures in last 10 minutes
		if now.After(attempt.blockedUntil) && (attempt.count == 0 || now.Sub(attempt.firstFail) > 10*time.Minute) {
			attempt.mu.Unlock()
			rl.attempts.Delete(key)
		} else {
			attempt.mu.Unlock()
		}
		return true
	})
}

// Stop stops the cleanup goroutine
func (rl *TokenValidationRateLimiter) Stop() {
	close(rl.stopCleanup)
}
