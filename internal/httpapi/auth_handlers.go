package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"time"

	"github.com/google/uuid"

	"distributed-job-scheduler/internal/authsvc"
	"distributed-job-scheduler/internal/store"
)

const minPasswordLen = 8

type authResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
}

func normalizeEmail(raw string) (string, error) {
	addr, err := mail.ParseAddress(raw)
	if err != nil {
		return "", err
	}
	return addr.Address, nil
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	email, err := normalizeEmail(req.Email)
	if err != nil {
		badRequest(w, "invalid email")
		return
	}
	if len(req.Password) < minPasswordLen {
		badRequest(w, "password must be at least 8 characters")
		return
	}

	hash, err := authsvc.HashPassword(req.Password)
	if err != nil {
		internalError(w, err)
		return
	}

	user, err := s.store.CreateUser(r.Context(), email, hash)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			conflict(w, "an account with this email already exists")
			return
		}
		internalError(w, err)
		return
	}

	s.sendOTP(r, user.ID, user.Email, store.OTPVerifyEmail,
		"Verify your email", "Your Job Scheduler verification code is:")

	s.respondWithNewSessionStatus(w, r, user.ID, http.StatusCreated)
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	email, err := normalizeEmail(req.Email)
	if err != nil {
		unauthorized(w, "invalid email or password")
		return
	}

	user, err := s.store.GetUserByEmail(r.Context(), email)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			unauthorized(w, "invalid email or password")
			return
		}
		internalError(w, err)
		return
	}
	if !authsvc.CheckPassword(user.PasswordHash, req.Password) {
		unauthorized(w, "invalid email or password")
		return
	}

	s.respondWithNewSession(w, r, user.ID)
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// handleRefresh rotates the refresh token: the presented token is revoked and
// a new one issued, so a stolen-and-replayed old token is a detectable reuse.
func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		badRequest(w, "refresh_token is required")
		return
	}

	hash := authsvc.HashRefreshToken(req.RefreshToken)
	rt, err := s.store.GetValidRefreshToken(r.Context(), hash)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			unauthorized(w, "invalid or expired refresh token")
			return
		}
		internalError(w, err)
		return
	}

	if err := s.store.RevokeRefreshToken(r.Context(), rt.ID); err != nil {
		internalError(w, err)
		return
	}

	s.respondWithNewSession(w, r, rt.UserID)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		badRequest(w, "refresh_token is required")
		return
	}
	if err := s.store.RevokeRefreshTokenByHash(r.Context(), authsvc.HashRefreshToken(req.RefreshToken)); err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) respondWithNewSession(w http.ResponseWriter, r *http.Request, userID uuid.UUID) {
	s.respondWithNewSessionStatus(w, r, userID, http.StatusOK)
}

func (s *Server) respondWithNewSessionStatus(w http.ResponseWriter, r *http.Request, userID uuid.UUID, status int) {
	access, err := s.tokens.GenerateAccessToken(userID)
	if err != nil {
		internalError(w, err)
		return
	}

	refresh, hash, err := authsvc.NewRefreshToken()
	if err != nil {
		internalError(w, err)
		return
	}
	if err := s.store.CreateRefreshToken(r.Context(), userID, hash, time.Now().Add(s.refreshTokenTTL)); err != nil {
		internalError(w, err)
		return
	}

	writeJSON(w, status, authResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
	})
}
