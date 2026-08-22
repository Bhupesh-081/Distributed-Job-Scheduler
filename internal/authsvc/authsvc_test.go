package authsvc

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct-horse")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(hash, "correct-horse") {
		t.Fatal("expected matching password to verify")
	}
	if CheckPassword(hash, "wrong-password") {
		t.Fatal("expected mismatched password to fail")
	}
}

func TestAccessTokenRoundTrip(t *testing.T) {
	issuer := NewTokenIssuer("test-secret-at-least-32-characters", time.Minute)
	userID := uuid.New()

	token, err := issuer.GenerateAccessToken(userID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := issuer.ParseAccessToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if got != userID {
		t.Fatalf("got %s, want %s", got, userID)
	}
}

func TestAccessTokenExpired(t *testing.T) {
	issuer := NewTokenIssuer("test-secret-at-least-32-characters", -time.Minute)
	token, err := issuer.GenerateAccessToken(uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := issuer.ParseAccessToken(token); err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken for expired token, got %v", err)
	}
}

func TestRefreshTokenHashIsDeterministicAndUnique(t *testing.T) {
	tokenA, hashA, err := NewRefreshToken()
	if err != nil {
		t.Fatal(err)
	}
	tokenB, hashB, err := NewRefreshToken()
	if err != nil {
		t.Fatal(err)
	}
	if tokenA == tokenB || hashA == hashB {
		t.Fatal("expected distinct tokens/hashes")
	}
	if HashRefreshToken(tokenA) != hashA {
		t.Fatal("hash must be deterministic for lookup by hash to work")
	}
}
