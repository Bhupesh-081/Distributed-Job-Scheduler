package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"distributed-job-scheduler/internal/authsvc"
	"distributed-job-scheduler/internal/store"
)

const otpTTL = 10 * time.Minute

// Errors are logged, not returned: a dead mail relay shouldn't fail
// registration/login, and forgot-password must respond identically whether
// or not the send actually worked (see handleForgotPassword).
func (s *Server) sendOTP(r *http.Request, userID uuid.UUID, email, purpose, subject, bodyIntro string) {
	code, hash, err := authsvc.NewOTP()
	if err != nil {
		slog.Error("generate otp", "error", err)
		return
	}
	if err := s.store.CreateOTP(r.Context(), userID, purpose, hash, time.Now().Add(otpTTL)); err != nil {
		slog.Error("create otp", "error", err)
		return
	}
	body := bodyIntro + "\n\n" + code + "\n\nThis code expires in 10 minutes."
	if err := s.mailer.Send(email, subject, body); err != nil {
		slog.Error("send otp email", "error", err, "purpose", purpose)
	}
}

type otpRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

func (s *Server) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req otpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	email, err := normalizeEmail(req.Email)
	if err != nil {
		badRequest(w, "invalid email")
		return
	}
	user, err := s.store.GetUserByEmail(r.Context(), email)
	if err != nil {
		badRequest(w, "invalid or expired code")
		return
	}
	if err := s.store.VerifyOTP(r.Context(), user.ID, store.OTPVerifyEmail, authsvc.HashOTP(req.Code)); err != nil {
		if errors.Is(err, store.ErrOTPInvalid) {
			badRequest(w, "invalid or expired code")
			return
		}
		internalError(w, err)
		return
	}
	if err := s.store.MarkUserEmailVerified(r.Context(), user.ID); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type emailRequest struct {
	Email string `json:"email"`
}

// handleResendVerification always responds 200 regardless of whether the
// address exists or is already verified, same non-enumeration reasoning as
// handleForgotPassword.
func (s *Server) handleResendVerification(w http.ResponseWriter, r *http.Request) {
	var req emailRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if email, err := normalizeEmail(req.Email); err == nil {
		if user, err := s.store.GetUserByEmail(r.Context(), email); err == nil && !user.EmailVerified {
			s.sendOTP(r, user.ID, user.Email, store.OTPVerifyEmail,
				"Verify your email", "Your Job Scheduler verification code is:")
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleForgotPassword always responds 200 whether or not the address has
// an account, so a caller can't use this endpoint to enumerate registered
// emails.
func (s *Server) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req emailRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if email, err := normalizeEmail(req.Email); err == nil {
		if user, err := s.store.GetUserByEmail(r.Context(), email); err == nil {
			s.sendOTP(r, user.ID, user.Email, store.OTPResetPassword,
				"Reset your password", "Your Job Scheduler password reset code is:")
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type resetPasswordRequest struct {
	Email       string `json:"email"`
	Code        string `json:"code"`
	NewPassword string `json:"new_password"`
}

func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	email, err := normalizeEmail(req.Email)
	if err != nil {
		badRequest(w, "invalid email")
		return
	}
	if len(req.NewPassword) < minPasswordLen {
		badRequest(w, "password must be at least 8 characters")
		return
	}

	user, err := s.store.GetUserByEmail(r.Context(), email)
	if err != nil {
		badRequest(w, "invalid or expired code")
		return
	}
	if err := s.store.VerifyOTP(r.Context(), user.ID, store.OTPResetPassword, authsvc.HashOTP(req.Code)); err != nil {
		if errors.Is(err, store.ErrOTPInvalid) {
			badRequest(w, "invalid or expired code")
			return
		}
		internalError(w, err)
		return
	}

	hash, err := authsvc.HashPassword(req.NewPassword)
	if err != nil {
		internalError(w, err)
		return
	}
	if err := s.store.UpdateUserPassword(r.Context(), user.ID, hash); err != nil {
		internalError(w, err)
		return
	}
	// A leaked-and-reset password shouldn't leave old sessions valid.
	_ = s.store.RevokeAllRefreshTokensForUser(r.Context(), user.ID)

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
