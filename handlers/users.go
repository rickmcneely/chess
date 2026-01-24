package handlers

import (
	"chess-server/models"
	"net/http"
)

func HandleLeaderboard(w http.ResponseWriter, r *http.Request) {
	users, err := models.GetLeaderboard()
	if err != nil {
		sendJSON(w, map[string]interface{}{"success": false, "message": "Failed to get leaderboard"})
		return
	}

	sendJSON(w, map[string]interface{}{"success": true, "users": users})
}

func HandleOnlineUsers(w http.ResponseWriter, r *http.Request) {
	if GameHub == nil {
		sendJSON(w, map[string]interface{}{"success": true, "users": []OnlineUser{}})
		return
	}

	users := GameHub.GetOnlineUsers()
	sendJSON(w, map[string]interface{}{"success": true, "users": users})
}
