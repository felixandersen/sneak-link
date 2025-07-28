package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

type TokenClaims struct {
	IssuedAt  time.Time `json:"iat"`
	ExpiresAt time.Time `json:"exp"`
}

// GenerateToken creates a signed token
func GenerateToken(maxAge time.Duration, signingKey []byte) (string, error) {
	now := time.Now()
	claims := TokenClaims{
		IssuedAt:  now,
		ExpiresAt: now.Add(maxAge),
	}

	// Marshal claims to JSON
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("failed to marshal claims: %v", err)
	}

	// Encode claims as base64
	claimsB64 := base64.URLEncoding.EncodeToString(claimsJSON)

	// Create HMAC signature
	h := hmac.New(sha256.New, signingKey)
	h.Write([]byte(claimsB64))
	signature := base64.URLEncoding.EncodeToString(h.Sum(nil))

	// Return token as claims.signature
	return claimsB64 + "." + signature, nil
}

// ValidateToken verifies a token and returns the claims if valid
func ValidateToken(token string, signingKey []byte) (*TokenClaims, error) {
	// Split token into claims and signature
	parts := splitToken(token)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid token format")
	}

	claimsB64, signatureB64 := parts[0], parts[1]

	// Verify signature
	h := hmac.New(sha256.New, signingKey)
	h.Write([]byte(claimsB64))
	expectedSignature := base64.URLEncoding.EncodeToString(h.Sum(nil))

	if !hmac.Equal([]byte(signatureB64), []byte(expectedSignature)) {
		return nil, fmt.Errorf("invalid token signature")
	}

	// Decode claims
	claimsJSON, err := base64.URLEncoding.DecodeString(claimsB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode claims: %v", err)
	}

	var claims TokenClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, fmt.Errorf("failed to unmarshal claims: %v", err)
	}

	// Validate expiration
	if time.Now().After(claims.ExpiresAt) {
		return nil, fmt.Errorf("token expired")
	}

	return &claims, nil
}

func splitToken(token string) []string {
	var parts []string
	var current string
	
	for _, char := range token {
		if char == '.' {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(char)
		}
	}
	
	if current != "" {
		parts = append(parts, current)
	}
	
	return parts
}
