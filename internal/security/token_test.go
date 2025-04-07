package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGenerateToken(t *testing.T) {
	tests := []struct {
		name               string
		secret             string
		siteID             string
		gtmID              string
		expirationDuration time.Duration
		wantErr            bool
	}{
		{
			name:               "Valid token generation",
			secret:             "test-secret",
			siteID:             "site123",
			gtmID:              "GTM-ABC123",
			expirationDuration: 10 * time.Minute,
			wantErr:            false,
		},
		{
			name:               "Empty secret",
			secret:             "",
			siteID:             "site123",
			gtmID:              "GTM-ABC123",
			expirationDuration: 10 * time.Minute,
			wantErr:            true,
		},
		{
			name:               "Zero expiration",
			secret:             "test-secret",
			siteID:             "site123",
			gtmID:              "GTM-ABC123",
			expirationDuration: 0,
			wantErr:            true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := GenerateToken(tt.secret, tt.siteID, tt.gtmID, tt.expirationDuration)

			// Check error expectation
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// If we expect an error, no need to check the token
			if tt.wantErr {
				return
			}

			// Verify token format (timestamp:signature)
			parts := strings.Split(token, ":")
			if len(parts) != 2 {
				t.Errorf("Token format incorrect, expected timestamp:signature, got %s", token)
				return
			}

			// Verify timestamp is recent
			timestamp := parts[0]
			if len(timestamp) <= 0 {
				t.Errorf("Token timestamp is empty")
				return
			}
		})
	}
}

func TestValidateToken(t *testing.T) {
	// Setup
	secret := "test-secret"
	siteID := "site123"
	gtmID := "GTM-ABC123"
	expiration := 10 * time.Minute

	// Generate a valid token
	token, err := GenerateToken(secret, siteID, gtmID, expiration)
	if err != nil {
		t.Fatalf("Failed to generate token for test: %v", err)
	}

	tests := []struct {
		name      string
		secret    string
		siteID    string
		gtmID     string
		token     string
		wantValid bool
		wantErr   bool
	}{
		{
			name:      "Valid token",
			secret:    secret,
			siteID:    siteID,
			gtmID:     gtmID,
			token:     token,
			wantValid: true,
			wantErr:   false,
		},
		{
			name:      "Invalid token format",
			secret:    secret,
			siteID:    siteID,
			gtmID:     gtmID,
			token:     "invalid-token",
			wantValid: false,
			wantErr:   true,
		},
		{
			name:      "Wrong site ID",
			secret:    secret,
			siteID:    "wrong-site",
			gtmID:     gtmID,
			token:     token,
			wantValid: false,
			wantErr:   false,
		},
		{
			name:      "Wrong GTM ID",
			secret:    secret,
			siteID:    siteID,
			gtmID:     "wrong-gtm",
			token:     token,
			wantValid: false,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ValidateToken returns (valid bool, err error)
			valid, err := ValidateToken(tt.secret, tt.siteID, tt.gtmID, tt.token)

			// Check if error matches expectation
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// If we expect an error, no need to check validity
			if tt.wantErr {
				return
			}

			// Check if validity matches expectation
			if valid != tt.wantValid {
				t.Errorf("ValidateToken() valid = %v, wantValid %v", valid, tt.wantValid)
			}
		})
	}
}

func TestTokenGenerationAndValidation(t *testing.T) {
	secret := "test-secret"
	siteID := "test-site"
	gtmID := "gtm-test"
	expiration := 10 * time.Minute

	token, err := GenerateToken(secret, siteID, gtmID, expiration)
	assert.NoError(t, err, "Token generation should not fail")
	assert.NotEmpty(t, token, "Generated token should not be empty")

	parts := strings.Split(token, ":")
	assert.Len(t, parts, 2, "Token should have two parts separated by colon")

	valid, err := ValidateToken(secret, siteID, gtmID, token)
	assert.NoError(t, err, "Validation of a fresh token should not fail")
	assert.True(t, valid, "Freshly generated token should be valid")
}

func TestTokenValidationExpired(t *testing.T) {
	secret := "test-secret"
	siteID := "test-site-expired"
	gtmID := "gtm-expired"
	expiration := -1 * time.Hour // Significantly negative expiration to make the token immediately invalid

	// Try to generate with invalid expiration and verify the error
	_, err := GenerateToken(secret, siteID, gtmID, expiration)
	assert.Error(t, err, "GenerateToken should return an error for non-positive expiration")
	assert.Contains(t, err.Error(), "expirationDuration must be positive")

	// Since generation fails, this test in its original form doesn't make sense.
	// Instead, we'll test that a generated token with a *valid* but *past*
	// expiration is correctly evaluated as expired.

	pastExpiration := -1 * time.Hour // Time in the past

	// Generate a token that should have expired an hour ago
	expiresAtPast := time.Now().Add(pastExpiration).Unix()
	messagePast := fmt.Sprintf("%s:%s:%d", siteID, gtmID, expiresAtPast)
	hPast := hmac.New(sha256.New, []byte(secret))
	hPast.Write([]byte(messagePast))
	signaturePast := hex.EncodeToString(hPast.Sum(nil))
	pastToken := fmt.Sprintf("%d:%s", expiresAtPast, signaturePast)

	// Validation of a token that has already expired
	valid, err := ValidateToken(secret, siteID, gtmID, pastToken)
	assert.Error(t, err, "Validation of past expired token should return an error")
	if err != nil { // Prevent panic when err == nil
		assert.Contains(t, err.Error(), "token has expired", "Error message should indicate expiration")
	}
	assert.False(t, valid, "Past expired token should be invalid")

	// Also test the case where we generate with normal expiration and wait
	tokenValidGen, errGen := GenerateToken(secret, siteID, gtmID, 5*time.Millisecond) // Short expiration
	assert.NoError(t, errGen)
	assert.NotEmpty(t, tokenValidGen)

	time.Sleep(10 * time.Millisecond) // Longer wait

	validAfterWait, errAfterWait := ValidateToken(secret, siteID, gtmID, tokenValidGen)
	assert.Error(t, errAfterWait, "Validation after waiting should return an error")
	if errAfterWait != nil {
		assert.Contains(t, errAfterWait.Error(), "token has expired")
	}
	assert.False(t, validAfterWait, "Token validated after wait should be invalid")
}

func TestValidateToken_InvalidFormat(t *testing.T) {
	secret := "test-secret"
	siteID := "test-site"
	gtmID := "gtm-test"
	invalidToken := "invalid-token-format"

	valid, err := ValidateToken(secret, siteID, gtmID, invalidToken)
	assert.Error(t, err) // Expect format error
	assert.False(t, valid)
}

func TestValidateToken_WrongSecret(t *testing.T) {
	secret := "test-secret"
	wrongSecret := "wrong-secret"
	siteID := "test-site"
	gtmID := "gtm-test"
	expiration := 5 * time.Minute

	token, err := GenerateToken(secret, siteID, gtmID, expiration)
	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	valid, err := ValidateToken(wrongSecret, siteID, gtmID, token)
	assert.NoError(t, err) // Error should not occur, validation just fails
	assert.False(t, valid)
}

func TestValidateToken_WrongSiteID(t *testing.T) {
	secret := "test-secret"
	siteID := "test-site"
	wrongSiteID := "wrong-site"
	gtmID := "gtm-test"
	expiration := 5 * time.Minute

	token, err := GenerateToken(secret, siteID, gtmID, expiration)
	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	valid, err := ValidateToken(secret, wrongSiteID, gtmID, token)
	assert.NoError(t, err)
	assert.False(t, valid)
}

func TestValidateToken_WrongGtmID(t *testing.T) {
	secret := "test-secret"
	siteID := "test-site"
	gtmID := "gtm-test"
	wrongGtmID := "wrong-gtm"
	expiration := 5 * time.Minute

	token, err := GenerateToken(secret, siteID, gtmID, expiration)
	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	valid, err := ValidateToken(secret, siteID, wrongGtmID, token)
	assert.NoError(t, err)
	assert.False(t, valid)
}

func TestGenerateToken_EmptySecret(t *testing.T) {
	_, err := GenerateToken("", "site", "gtm", 1*time.Minute)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "secret cannot be empty")
}

func TestValidateToken_EmptySecret(t *testing.T) {
	_, err := ValidateToken("", "site", "gtm", "123:abc")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "secret cannot be empty")
}

func TestGenerateToken_NonPositiveExpiration(t *testing.T) {
	_, err := GenerateToken("secret", "site", "gtm", 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expirationDuration must be positive")

	_, err = GenerateToken("secret", "site", "gtm", -1*time.Second)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expirationDuration must be positive")
}
