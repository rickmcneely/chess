package handlers

import (
	"chess-server/models"
	"encoding/json"
	"net/http"
)

func AdminMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.Header.Get("X-Session-ID")
		if sessionID == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		session, err := models.GetSession(sessionID)
		if err != nil {
			http.Error(w, "Invalid session", http.StatusUnauthorized)
			return
		}

		user, err := models.GetUserByID(session.UserID)
		if err != nil {
			http.Error(w, "User not found", http.StatusUnauthorized)
			return
		}

		if !user.IsAdmin {
			http.Error(w, "Admin access required", http.StatusForbidden)
			return
		}

		next(w, r)
	}
}

func HandleGetPendingUsers(w http.ResponseWriter, r *http.Request) {
	users, err := models.GetPendingUsers()
	if err != nil {
		sendJSON(w, map[string]interface{}{"success": false, "message": "Failed to get users"})
		return
	}

	sendJSON(w, map[string]interface{}{"success": true, "users": users})
}

func HandleApproveUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		UserID int `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSON(w, map[string]interface{}{"success": false, "message": "Invalid request"})
		return
	}

	if err := models.ApproveUser(req.UserID); err != nil {
		sendJSON(w, map[string]interface{}{"success": false, "message": "Failed to approve user"})
		return
	}

	sendJSON(w, map[string]interface{}{"success": true, "message": "User approved"})
}

func HandleRejectUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		UserID int `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSON(w, map[string]interface{}{"success": false, "message": "Invalid request"})
		return
	}

	if err := models.RejectUser(req.UserID); err != nil {
		sendJSON(w, map[string]interface{}{"success": false, "message": "Failed to reject user"})
		return
	}

	sendJSON(w, map[string]interface{}{"success": true, "message": "User rejected"})
}

func HandleGetSettings(w http.ResponseWriter, r *http.Request) {
	profanityFilter := models.IsProfanityFilterEnabled()
	profanityAutoWarn := models.IsProfanityAutoWarnEnabled()

	sendJSON(w, map[string]interface{}{
		"success":             true,
		"profanity_filter":    profanityFilter,
		"profanity_auto_warn": profanityAutoWarn,
	})
}

func HandleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ProfanityFilter   *bool `json:"profanity_filter"`
		ProfanityAutoWarn *bool `json:"profanity_auto_warn"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSON(w, map[string]interface{}{"success": false, "message": "Invalid request"})
		return
	}

	if req.ProfanityFilter != nil {
		if err := models.SetProfanityFilter(*req.ProfanityFilter); err != nil {
			sendJSON(w, map[string]interface{}{"success": false, "message": "Failed to update settings"})
			return
		}
	}

	if req.ProfanityAutoWarn != nil {
		if err := models.SetProfanityAutoWarn(*req.ProfanityAutoWarn); err != nil {
			sendJSON(w, map[string]interface{}{"success": false, "message": "Failed to update settings"})
			return
		}
	}

	sendJSON(w, map[string]interface{}{"success": true, "message": "Settings updated"})
}

func HandleGetAllUsers(w http.ResponseWriter, r *http.Request) {
	users, err := models.GetAllUsers()
	if err != nil {
		sendJSON(w, map[string]interface{}{"success": false, "message": "Failed to get users"})
		return
	}

	sendJSON(w, map[string]interface{}{"success": true, "users": users})
}

func HandleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		UserID int `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSON(w, map[string]interface{}{"success": false, "message": "Invalid request"})
		return
	}

	user, err := models.GetUserByID(req.UserID)
	if err != nil {
		sendJSON(w, map[string]interface{}{"success": false, "message": "User not found"})
		return
	}

	if user.IsAdmin {
		sendJSON(w, map[string]interface{}{"success": false, "message": "Cannot delete admin users"})
		return
	}

	if err := models.DeleteUser(req.UserID); err != nil {
		sendJSON(w, map[string]interface{}{"success": false, "message": "Failed to delete user"})
		return
	}

	sendJSON(w, map[string]interface{}{"success": true, "message": "User deleted"})
}
