package httpapi

import (
	"encoding/json"
	"net/http"

	"distributed-job-scheduler/internal/authsvc"
)

type meResponse struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	DisplayName   string `json:"display_name,omitempty"`
	CreatedAt     string `json:"created_at"`
}

func (s *Server) handleGetMe(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	user, err := s.store.GetUserByID(r.Context(), userID)
	if err != nil {
		internalError(w, err)
		return
	}
	resp := meResponse{
		ID:            user.ID.String(),
		Email:         user.Email,
		EmailVerified: user.EmailVerified,
		CreatedAt:     user.CreatedAt.Format(timeFormat),
	}
	if user.DisplayName != nil {
		resp.DisplayName = *user.DisplayName
	}
	writeJSON(w, http.StatusOK, resp)
}

type updateMeRequest struct {
	DisplayName string `json:"display_name"`
}

const maxDisplayNameLen = 60

func (s *Server) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	var req updateMeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if len(req.DisplayName) > maxDisplayNameLen {
		badRequest(w, "display_name must be 60 characters or fewer")
		return
	}
	if err := s.store.UpdateUserDisplayName(r.Context(), userID, req.DisplayName); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// handleChangePassword is the logged-in "account settings" counterpart to
// the OTP-based /auth/reset-password (for when you're locked out): this one
// requires proving you already know the current password, no email
// round-trip needed. Same as reset-password, a successful change revokes
// every refresh token for the user - including the caller's own, so the
// frontend should treat a 200 here as "log out and sign in again".
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if len(req.NewPassword) < minPasswordLen {
		badRequest(w, "password must be at least 8 characters")
		return
	}

	user, err := s.store.GetUserByID(r.Context(), userID)
	if err != nil {
		internalError(w, err)
		return
	}
	if !authsvc.CheckPassword(user.PasswordHash, req.CurrentPassword) {
		unauthorized(w, "current password is incorrect")
		return
	}

	hash, err := authsvc.HashPassword(req.NewPassword)
	if err != nil {
		internalError(w, err)
		return
	}
	if err := s.store.UpdateUserPassword(r.Context(), userID, hash); err != nil {
		internalError(w, err)
		return
	}
	_ = s.store.RevokeAllRefreshTokensForUser(r.Context(), userID)

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
