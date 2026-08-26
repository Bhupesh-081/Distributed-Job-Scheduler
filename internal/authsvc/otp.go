package authsvc

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
)

// NewOTP returns a random 6-digit code to email to the user, plus its
// SHA-256 hash to persist (same "never store the secret itself" reasoning
// as refresh tokens, even though a 6-digit code is inherently low-entropy -
// that's mitigated by a short expiry and a capped attempt count, not by
// keeping the hash secret).
func NewOTP() (code string, hash string, err error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", "", err
	}
	code = fmt.Sprintf("%06d", n.Int64())
	return code, HashOTP(code), nil
}

func HashOTP(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}
