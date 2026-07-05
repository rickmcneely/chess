package handlers

import (
	"chess-server/models"
	"net/http"
	"strconv"
)

func HandleGetGame(w http.ResponseWriter, r *http.Request) {
	gameIDStr := r.URL.Query().Get("id")
	gameID, err := strconv.Atoi(gameIDStr)
	if err != nil {
		sendJSON(w, map[string]interface{}{"success": false, "message": "Invalid game ID"})
		return
	}

	game, err := models.GetGameByID(gameID)
	if err != nil {
		sendJSON(w, map[string]interface{}{"success": false, "message": "Game not found"})
		return
	}

	whiteUser, _ := models.GetUserByID(game.WhiteUserID)
	blackUser, _ := models.GetUserByID(game.BlackUserID)

	var whiteUsername, blackUsername string
	if whiteUser != nil {
		whiteUsername = whiteUser.Username
	}
	if blackUser != nil {
		blackUsername = blackUser.Username
	}

	sendJSON(w, map[string]interface{}{
		"success":        true,
		"game":           game,
		"white_username": whiteUsername,
		"black_username": blackUsername,
	})
}

func HandleGetActiveGame(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get("X-Session-ID")
	if sessionID == "" {
		sendJSON(w, map[string]interface{}{"success": false, "message": "Not authenticated"})
		return
	}

	session, err := models.GetSession(sessionID)
	if err != nil {
		sendJSON(w, map[string]interface{}{"success": false, "message": "Invalid session"})
		return
	}

	game, err := models.GetActiveGameForUser(session.UserID)
	if err != nil {
		sendJSON(w, map[string]interface{}{"success": true, "game": nil})
		return
	}

	whiteUser, _ := models.GetUserByID(game.WhiteUserID)
	blackUser, _ := models.GetUserByID(game.BlackUserID)

	var whiteUsername, blackUsername string
	if whiteUser != nil {
		whiteUsername = whiteUser.Username
	}
	if blackUser != nil {
		blackUsername = blackUser.Username
	}

	response := map[string]interface{}{
		"success":        true,
		"game":           game,
		"white_username": whiteUsername,
		"black_username": blackUsername,
	}

	// Include calculated remaining times for reconnect scenario
	if game.IsTimed() {
		response["white_time_remaining"] = game.GetWhiteTimeRemaining()
		response["black_time_remaining"] = game.GetBlackTimeRemaining()
	}

	sendJSON(w, response)
}

func HandleGetGameHistory(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get("X-Session-ID")
	if sessionID == "" {
		sendJSON(w, map[string]interface{}{"success": false, "message": "Not authenticated"})
		return
	}

	session, err := models.GetSession(sessionID)
	if err != nil {
		sendJSON(w, map[string]interface{}{"success": false, "message": "Invalid session"})
		return
	}

	games, err := models.GetGameHistory(session.UserID)
	if err != nil {
		sendJSON(w, map[string]interface{}{"success": false, "message": "Failed to get game history"})
		return
	}

	sendJSON(w, map[string]interface{}{"success": true, "games": games})
}

func HandleGetGameMessages(w http.ResponseWriter, r *http.Request) {
	gameIDStr := r.URL.Query().Get("game_id")
	gameID, err := strconv.Atoi(gameIDStr)
	if err != nil {
		sendJSON(w, map[string]interface{}{"success": false, "message": "Invalid game ID"})
		return
	}

	messages, err := models.GetGameMessages(gameID)
	if err != nil {
		sendJSON(w, map[string]interface{}{"success": false, "message": "Failed to get messages"})
		return
	}

	sendJSON(w, map[string]interface{}{"success": true, "messages": messages})
}

func HandleGetDirectMessages(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get("X-Session-ID")
	if sessionID == "" {
		sendJSON(w, map[string]interface{}{"success": false, "message": "Not authenticated"})
		return
	}

	session, err := models.GetSession(sessionID)
	if err != nil {
		sendJSON(w, map[string]interface{}{"success": false, "message": "Invalid session"})
		return
	}

	messages, err := models.GetDirectMessages(session.UserID)
	if err != nil {
		sendJSON(w, map[string]interface{}{"success": false, "message": "Failed to get messages"})
		return
	}

	sendJSON(w, map[string]interface{}{"success": true, "messages": messages})
}
