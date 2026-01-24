package handlers

import (
	"chess-server/models"
	"encoding/json"
	"net/http"
	"strings"
)

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Success   bool         `json:"success"`
	Message   string       `json:"message"`
	SessionID string       `json:"session_id,omitempty"`
	User      *models.User `json:"user,omitempty"`
}

func HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSON(w, AuthResponse{Success: false, Message: "Invalid request"})
		return
	}

	user, err := models.GetUserByUsername(req.Username)
	if err != nil {
		sendJSON(w, AuthResponse{Success: false, Message: "Invalid username or password"})
		return
	}

	if !user.CheckPassword(req.Password) {
		sendJSON(w, AuthResponse{Success: false, Message: "Invalid username or password"})
		return
	}

	if !user.Approved {
		sendJSON(w, AuthResponse{Success: false, Message: "Your account is pending approval"})
		return
	}

	session, err := models.CreateSession(user.ID)
	if err != nil {
		sendJSON(w, AuthResponse{Success: false, Message: "Failed to create session"})
		return
	}

	sendJSON(w, AuthResponse{
		Success:   true,
		Message:   "Login successful",
		SessionID: session.ID,
		User:      user,
	})
}

func HandleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSON(w, AuthResponse{Success: false, Message: "Invalid request"})
		return
	}

	if len(req.Username) < 3 {
		sendJSON(w, AuthResponse{Success: false, Message: "Username must be at least 3 characters"})
		return
	}

	if len(req.Email) < 5 || !strings.Contains(req.Email, "@") {
		sendJSON(w, AuthResponse{Success: false, Message: "Please enter a valid email address"})
		return
	}

	if len(req.Password) < 6 {
		sendJSON(w, AuthResponse{Success: false, Message: "Password must be at least 6 characters"})
		return
	}

	existing, _ := models.GetUserByUsername(req.Username)
	if existing != nil {
		sendJSON(w, AuthResponse{Success: false, Message: "Username already taken"})
		return
	}

	user, err := models.CreateUser(req.Username, req.Email, req.Password)
	if err != nil {
		sendJSON(w, AuthResponse{Success: false, Message: "Failed to create user"})
		return
	}

	sendJSON(w, AuthResponse{
		Success: true,
		Message: "Registration successful. Please wait for admin approval.",
		User:    user,
	})
}

func HandleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionID := r.Header.Get("X-Session-ID")
	if sessionID != "" {
		models.DeleteSession(sessionID)
	}

	sendJSON(w, AuthResponse{Success: true, Message: "Logged out"})
}

func HandleMe(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get("X-Session-ID")
	if sessionID == "" {
		sendJSON(w, AuthResponse{Success: false, Message: "Not authenticated"})
		return
	}

	session, err := models.GetSession(sessionID)
	if err != nil {
		sendJSON(w, AuthResponse{Success: false, Message: "Invalid session"})
		return
	}

	user, err := models.GetUserByID(session.UserID)
	if err != nil {
		sendJSON(w, AuthResponse{Success: false, Message: "User not found"})
		return
	}

	sendJSON(w, AuthResponse{
		Success:   true,
		SessionID: sessionID,
		User:      user,
	})
}

func sendJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
