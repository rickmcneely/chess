package handlers

import (
	"chess-server/chess"
	"chess-server/models"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Client struct {
	Hub            *Hub
	Conn           *websocket.Conn
	Send           chan []byte
	UserID         int
	Username       string
	GameID         int
	ObservingGameID int // Game being observed (0 = not observing)
}

type Hub struct {
	Clients    map[*Client]bool
	Broadcast  chan []byte
	Register   chan *Client
	Unregister chan *Client
	mutex      sync.RWMutex
}

type WSMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type OnlineUser struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	InGame   bool   `json:"in_game"`
}

type GameInvite struct {
	FromUserID      int    `json:"from_user_id"`
	FromUsername    string `json:"from_username"`
	ToUserID        int    `json:"to_user_id"`
	TimeControlMS   int    `json:"time_control_ms"`   // 0 = no clock
	IncrementMS     int    `json:"increment_ms"`
	TimeControlName string `json:"time_control_name"` // e.g., "5+0"
}

type GameMove struct {
	GameID int    `json:"game_id"`
	Move   string `json:"move"`
}

type ChatMessage struct {
	GameID  int    `json:"game_id"`
	Content string `json:"content"`
}

var GameHub *Hub
var ComputerUserID int
var pendingInvites = make(map[int]*GameInvite)
var inviteMutex sync.RWMutex

// Observer tracking
var gameObservers = make(map[int]map[*Client]bool) // gameID -> set of observer clients
var observerMutex sync.RWMutex

func NewHub() *Hub {
	return &Hub{
		Clients:    make(map[*Client]bool),
		Broadcast:  make(chan []byte),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.mutex.Lock()
			h.Clients[client] = true
			log.Printf("Registered client: %s, total clients: %d", client.Username, len(h.Clients))
			h.mutex.Unlock()
			go h.broadcastOnlineUsers()

		case client := <-h.Unregister:
			h.mutex.Lock()
			if _, ok := h.Clients[client]; ok {
				delete(h.Clients, client)
				close(client.Send)
			}
			h.mutex.Unlock()
			go h.broadcastOnlineUsers()

		case message := <-h.Broadcast:
			h.mutex.RLock()
			for client := range h.Clients {
				select {
				case client.Send <- message:
				default:
					close(client.Send)
					delete(h.Clients, client)
				}
			}
			h.mutex.RUnlock()
		}
	}
}

func (h *Hub) broadcastOnlineUsers() {
	users := h.GetOnlineUsers()
	log.Printf("Broadcasting online users: %d users connected", len(users))
	payload, _ := json.Marshal(users)
	msg := WSMessage{
		Type:    "online_users",
		Payload: payload,
	}
	data, _ := json.Marshal(msg)
	h.Broadcast <- data
}

func (h *Hub) GetOnlineUsers() []OnlineUser {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	// Use a map to deduplicate users
	userMap := make(map[int]OnlineUser)
	for client := range h.Clients {
		// If user already exists, keep the one that's in a game (if any)
		if existing, ok := userMap[client.UserID]; ok {
			if client.GameID > 0 && !existing.InGame {
				userMap[client.UserID] = OnlineUser{
					ID:       client.UserID,
					Username: client.Username,
					InGame:   true,
				}
			}
		} else {
			userMap[client.UserID] = OnlineUser{
				ID:       client.UserID,
				Username: client.Username,
				InGame:   client.GameID > 0,
			}
		}
	}

	var users []OnlineUser
	for _, user := range userMap {
		users = append(users, user)
	}
	return users
}

func (h *Hub) SendToUser(userID int, msg []byte) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	for client := range h.Clients {
		if client.UserID == userID {
			select {
			case client.Send <- msg:
			default:
			}
		}
	}
}

func (h *Hub) GetClientByUserID(userID int) *Client {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	for client := range h.Clients {
		if client.UserID == userID {
			return client
		}
	}
	return nil
}

func HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		http.Error(w, "Missing session", http.StatusUnauthorized)
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

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade failed:", err)
		return
	}

	log.Printf("WebSocket connected: %s (ID: %d)", user.Username, user.ID)

	client := &Client{
		Hub:      GameHub,
		Conn:     conn,
		Send:     make(chan []byte, 256),
		UserID:   user.ID,
		Username: user.Username,
	}

	// Start the write pump first so it can receive messages
	go client.writePump()
	go client.readPump()

	// Then register (which triggers broadcast)
	GameHub.Register <- client

	// Deliver any stored messages
	go client.deliverStoredMessages()
}

func (c *Client) deliverStoredMessages() {
	messages, err := models.GetUndeliveredMessages(c.UserID)
	if err != nil {
		log.Printf("Error getting undelivered messages for %s: %v", c.Username, err)
		return
	}

	if len(messages) == 0 {
		return
	}

	log.Printf("Delivering %d stored messages to %s", len(messages), c.Username)

	var deliveredIDs []int
	for _, msg := range messages {
		dmPayload, _ := json.Marshal(map[string]interface{}{
			"message_id":    msg.ID,
			"from_user_id":  msg.FromUserID,
			"from_username": msg.FromUser,
			"content":       msg.Content,
			"created_at":    msg.CreatedAt,
			"stored":        true,
		})
		wsMsg := WSMessage{Type: "direct_message", Payload: dmPayload}
		data, _ := json.Marshal(wsMsg)

		select {
		case c.Send <- data:
			deliveredIDs = append(deliveredIDs, msg.ID)
		default:
			log.Printf("Failed to deliver stored message %d to %s", msg.ID, c.Username)
		}
	}

	// Mark messages as delivered
	models.MarkMessagesDelivered(deliveredIDs)
}

func (c *Client) readPump() {
	defer func() {
		// Clean up observer state
		if c.ObservingGameID > 0 {
			c.handleStopObserving()
		}
		c.Hub.Unregister <- c
		c.Conn.Close()
	}()

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			log.Printf("ReadMessage error for %s: %v", c.Username, err)
			break
		}

		log.Printf("Received message from %s: %s", c.Username, string(message))

		var msg WSMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("Failed to unmarshal message: %v", err)
			continue
		}

		c.handleMessage(msg)
	}
}

func (c *Client) writePump() {
	defer c.Conn.Close()

	for message := range c.Send {
		if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
			break
		}
	}
}

func (c *Client) handleMessage(msg WSMessage) {
	switch msg.Type {
	case "invite":
		c.handleInvite(msg.Payload)
	case "accept_invite":
		c.handleAcceptInvite(msg.Payload)
	case "decline_invite":
		c.handleDeclineInvite(msg.Payload)
	case "move":
		c.handleMove(msg.Payload)
	case "chat":
		c.handleChat(msg.Payload)
	case "direct_message":
		c.handleDirectMessage(msg.Payload)
	case "resign":
		c.handleResign(msg.Payload)
	case "rejoin_game":
		c.handleRejoinGame(msg.Payload)
	case "play_ai":
		c.handlePlayAI(msg.Payload)
	case "get_active_games":
		c.handleGetActiveGames()
	case "observe_game":
		c.handleObserveGame(msg.Payload)
	case "stop_observing":
		c.handleStopObserving()
	case "mark_read":
		c.handleMarkRead(msg.Payload)
	}
}

func (c *Client) handleInvite(payload json.RawMessage) {
	log.Printf("Received invite from %s, payload: %s", c.Username, string(payload))

	var invite struct {
		ToUserID      int    `json:"to_user_id"`
		TimeControlMS int    `json:"time_control_ms"`
		IncrementMS   int    `json:"increment_ms"`
	}
	if err := json.Unmarshal(payload, &invite); err != nil {
		log.Printf("Failed to unmarshal invite: %v", err)
		return
	}

	log.Printf("Invite to user ID: %d with time control: %dms + %dms", invite.ToUserID, invite.TimeControlMS, invite.IncrementMS)

	if invite.ToUserID == c.UserID {
		log.Printf("User tried to invite themselves")
		return
	}

	targetClient := c.Hub.GetClientByUserID(invite.ToUserID)
	if targetClient == nil {
		log.Printf("Target user not found or not online")
		return
	}

	if targetClient.GameID > 0 {
		log.Printf("Target user is already in a game")
		return
	}

	// Format time control name
	timeControlName := "No Clock"
	if invite.TimeControlMS > 0 {
		minutes := invite.TimeControlMS / 60000
		incrementSec := invite.IncrementMS / 1000
		timeControlName = formatTimeControl(minutes, incrementSec)
	}

	gameInvite := &GameInvite{
		FromUserID:      c.UserID,
		FromUsername:    c.Username,
		ToUserID:        invite.ToUserID,
		TimeControlMS:   invite.TimeControlMS,
		IncrementMS:     invite.IncrementMS,
		TimeControlName: timeControlName,
	}

	inviteMutex.Lock()
	pendingInvites[invite.ToUserID] = gameInvite
	inviteMutex.Unlock()

	log.Printf("Sending invite to %s", targetClient.Username)

	invitePayload, _ := json.Marshal(gameInvite)
	msg := WSMessage{
		Type:    "game_invite",
		Payload: invitePayload,
	}
	data, _ := json.Marshal(msg)
	c.Hub.SendToUser(invite.ToUserID, data)
}

func formatTimeControl(minutes, incrementSec int) string {
	if incrementSec > 0 {
		return fmt.Sprintf("%d+%d", minutes, incrementSec)
	}
	return fmt.Sprintf("%d+0", minutes)
}

func (c *Client) handleAcceptInvite(payload json.RawMessage) {
	log.Printf("Accept invite from %s, payload: %s", c.Username, string(payload))

	var accept struct {
		FromUserID int `json:"from_user_id"`
	}
	if err := json.Unmarshal(payload, &accept); err != nil {
		log.Printf("Failed to unmarshal accept invite: %v", err)
		return
	}

	log.Printf("Accept invite from user ID: %d", accept.FromUserID)

	inviteMutex.Lock()
	invite, ok := pendingInvites[c.UserID]
	if !ok || invite.FromUserID != accept.FromUserID {
		log.Printf("Invite not found or mismatch. ok=%v, expected from=%d", ok, accept.FromUserID)
		inviteMutex.Unlock()
		return
	}
	delete(pendingInvites, c.UserID)
	inviteMutex.Unlock()

	log.Printf("Creating game between %d and %d with time control %dms + %dms",
		invite.FromUserID, c.UserID, invite.TimeControlMS, invite.IncrementMS)

	whiteID, blackID := determineColors(invite.FromUserID, c.UserID)

	// Create game with time control
	var game *models.Game
	var err error
	if invite.TimeControlMS > 0 {
		timeControl := invite.TimeControlMS
		game, err = models.CreateGameWithClock(whiteID, blackID, &timeControl, invite.IncrementMS)
	} else {
		game, err = models.CreateGame(whiteID, blackID)
	}

	if err != nil {
		log.Println("Failed to create game:", err)
		return
	}

	fromClient := c.Hub.GetClientByUserID(invite.FromUserID)
	if fromClient != nil {
		fromClient.GameID = game.ID
	}
	c.GameID = game.ID

	whiteColor := "white"
	blackColor := "black"

	if user1, _ := models.GetUserByID(whiteID); user1 != nil {
		user1.UpdateLastGame(blackID, whiteColor)
	}
	if user2, _ := models.GetUserByID(blackID); user2 != nil {
		user2.UpdateLastGame(whiteID, blackColor)
	}

	gameData := map[string]interface{}{
		"game_id":          game.ID,
		"white_user_id":    whiteID,
		"black_user_id":    blackID,
		"fen":              game.FEN,
		"time_control_ms":  invite.TimeControlMS,
		"increment_ms":     invite.IncrementMS,
		"white_time_ms":    invite.TimeControlMS,
		"black_time_ms":    invite.TimeControlMS,
		"time_control_name": invite.TimeControlName,
	}
	gamePayload, _ := json.Marshal(gameData)
	msg := WSMessage{
		Type:    "game_start",
		Payload: gamePayload,
	}
	data, _ := json.Marshal(msg)

	log.Printf("Sending game_start to users %d and %d", invite.FromUserID, c.UserID)
	c.Hub.SendToUser(invite.FromUserID, data)
	c.Hub.SendToUser(c.UserID, data)

	// Start clock goroutine if timed game
	if invite.TimeControlMS > 0 {
		go runGameClock(c.Hub, game.ID)
	}

	go c.Hub.broadcastOnlineUsers()
}

func determineColors(user1ID, user2ID int) (whiteID, blackID int) {
	user1, _ := models.GetUserByID(user1ID)
	user2, _ := models.GetUserByID(user2ID)

	if user1 != nil && user1.LastOpponentID != nil && *user1.LastOpponentID == user2ID {
		if user1.LastColor != nil && *user1.LastColor == "white" {
			return user2ID, user1ID
		}
		return user1ID, user2ID
	}

	if user2 != nil && user2.LastOpponentID != nil && *user2.LastOpponentID == user1ID {
		if user2.LastColor != nil && *user2.LastColor == "white" {
			return user1ID, user2ID
		}
		return user2ID, user1ID
	}

	if user1ID%2 == 0 {
		return user1ID, user2ID
	}
	return user2ID, user1ID
}

func (c *Client) handleDeclineInvite(payload json.RawMessage) {
	var decline struct {
		FromUserID int `json:"from_user_id"`
	}
	if err := json.Unmarshal(payload, &decline); err != nil {
		return
	}

	inviteMutex.Lock()
	delete(pendingInvites, c.UserID)
	inviteMutex.Unlock()

	declinePayload, _ := json.Marshal(map[string]string{
		"message": c.Username + " declined your game invitation",
	})
	msg := WSMessage{
		Type:    "invite_declined",
		Payload: declinePayload,
	}
	data, _ := json.Marshal(msg)
	c.Hub.SendToUser(decline.FromUserID, data)
}

func (c *Client) handleRejoinGame(payload json.RawMessage) {
	var data struct {
		GameID int `json:"game_id"`
	}
	if err := json.Unmarshal(payload, &data); err != nil {
		log.Printf("Failed to unmarshal rejoin_game: %v", err)
		return
	}

	game, err := models.GetGameByID(data.GameID)
	if err != nil {
		log.Printf("Rejoin: game not found: %v", err)
		return
	}

	// Verify user is part of this game
	if game.WhiteUserID != c.UserID && game.BlackUserID != c.UserID {
		log.Printf("Rejoin: user %d not in game %d", c.UserID, data.GameID)
		return
	}

	// Verify game is active
	if game.Status != "active" {
		log.Printf("Rejoin: game %d not active (status: %s)", data.GameID, game.Status)
		return
	}

	c.GameID = data.GameID
	log.Printf("User %s rejoined game %d", c.Username, data.GameID)
}

func (c *Client) handlePlayAI(payload json.RawMessage) {
	var data struct {
		Difficulty    int `json:"difficulty"`
		TimeControlMS int `json:"time_control_ms"`
		IncrementMS   int `json:"increment_ms"`
	}
	if err := json.Unmarshal(payload, &data); err != nil {
		log.Printf("Failed to unmarshal play_ai: %v", err)
		return
	}

	if ComputerUserID == 0 {
		log.Printf("Computer user not initialized")
		errPayload, _ := json.Marshal(map[string]string{"error": "AI not available"})
		msg := WSMessage{Type: "error", Payload: errPayload}
		msgData, _ := json.Marshal(msg)
		c.Send <- msgData
		return
	}

	// Check if user already has an active game
	existingGame, _ := models.GetActiveGameForUser(c.UserID)
	if existingGame != nil {
		errPayload, _ := json.Marshal(map[string]string{"error": "You already have an active game"})
		msg := WSMessage{Type: "error", Payload: errPayload}
		msgData, _ := json.Marshal(msg)
		c.Send <- msgData
		return
	}

	// Randomly assign colors
	var whiteUserID, blackUserID int
	playerColor := "white"
	if rand.Intn(2) == 0 {
		whiteUserID = c.UserID
		blackUserID = ComputerUserID
		playerColor = "white"
	} else {
		whiteUserID = ComputerUserID
		blackUserID = c.UserID
		playerColor = "black"
	}

	// Create game with or without time control
	var game *models.Game
	var err error
	if data.TimeControlMS > 0 {
		timeControl := data.TimeControlMS
		game, err = models.CreateGameWithClock(whiteUserID, blackUserID, &timeControl, data.IncrementMS)
	} else {
		game, err = models.CreateGame(whiteUserID, blackUserID)
	}

	if err != nil {
		log.Printf("Failed to create AI game: %v", err)
		return
	}

	c.GameID = game.ID

	// Store AI difficulty in a map for this game
	aiDifficultyMutex.Lock()
	aiGameDifficulty[game.ID] = data.Difficulty
	aiDifficultyMutex.Unlock()

	// Prepare game start payload
	startData := map[string]interface{}{
		"game_id":        game.ID,
		"white_user_id":  whiteUserID,
		"black_user_id":  blackUserID,
		"white_username": getUsernameByID(whiteUserID),
		"black_username": getUsernameByID(blackUserID),
		"player_color":   playerColor,
		"fen":            game.FEN,
		"vs_ai":          true,
	}

	// Add clock info if timed
	if data.TimeControlMS > 0 {
		startData["time_control_ms"] = data.TimeControlMS
		startData["increment_ms"] = data.IncrementMS
		startData["white_time_ms"] = data.TimeControlMS
		startData["black_time_ms"] = data.TimeControlMS
		startData["time_control_name"] = formatTimeControl(data.TimeControlMS/60000, data.IncrementMS/1000)
	}

	// Send game start to player
	startPayload, _ := json.Marshal(startData)
	msg := WSMessage{Type: "game_start", Payload: startPayload}
	msgData, _ := json.Marshal(msg)
	c.Send <- msgData

	// Start clock for timed AI games (only affects the human player's clock)
	if data.TimeControlMS > 0 {
		go runGameClock(c.Hub, game.ID)
	}

	// If AI plays white, make the first move
	if whiteUserID == ComputerUserID {
		board := chess.ParseFEN(game.FEN)
		c.makeAIMove(game, board)
	}

	go c.Hub.broadcastOnlineUsers()
}

func (c *Client) makeAIMove(game *models.Game, board *chess.Board) {
	aiDifficultyMutex.RLock()
	difficulty := aiGameDifficulty[game.ID]
	aiDifficultyMutex.RUnlock()

	if difficulty == 0 {
		difficulty = 2 // Default medium difficulty
	}

	ai := chess.NewAIEngine(difficulty)
	aiMove := ai.GetBestMove(board)

	if aiMove.From == "" {
		log.Printf("AI has no legal moves")
		return
	}

	moveStr := chess.MoveToString(aiMove)
	log.Printf("AI plays: %s (depth %d)", moveStr, difficulty)

	// Add current position to history
	game.AddPositionToHistory(board.PositionKey())

	newBoard := board.MakeMove(aiMove)
	newFEN := newBoard.ToFEN()

	game.AddMove(moveStr, newFEN)

	// AI uses minimal time - just update the clock timestamp
	// This gives the AI a small time penalty but keeps the game fair
	if game.IsTimed() {
		game.DeductTimeAndAddIncrement(ComputerUserID)
	}

	// Check game status
	status := newBoard.GetGameStatusWithHistory(game.PositionHistory)
	var winnerID *int

	if status == "checkmate" {
		game.EndGame("checkmate", &ComputerUserID)
		winnerID = &ComputerUserID
		updateRatings(game, ComputerUserID)
		stopGameClock(game.ID)
	} else if status == "stalemate" || strings.HasPrefix(status, "draw_") {
		game.EndGame(status, nil)
		updateRatings(game, 0)
		stopGameClock(game.ID)
	}

	// Send AI move to player with clock info
	movePayloadData := map[string]interface{}{
		"game_id":   game.ID,
		"move":      moveStr,
		"fen":       newFEN,
		"status":    status,
		"winner_id": winnerID,
	}

	// Add clock info if timed game
	if game.IsTimed() {
		movePayloadData["white_time_ms"] = game.GetWhiteTimeRemaining()
		movePayloadData["black_time_ms"] = game.GetBlackTimeRemaining()
	}

	movePayload, _ := json.Marshal(movePayloadData)
	msg := WSMessage{Type: "game_update", Payload: movePayload}
	data, _ := json.Marshal(msg)

	// Send to the human player
	if game.WhiteUserID == ComputerUserID {
		c.Hub.SendToUser(game.BlackUserID, data)
	} else {
		c.Hub.SendToUser(game.WhiteUserID, data)
	}
	broadcastToObservers(c.Hub, game.ID, data)

	if status == "checkmate" || status == "stalemate" || strings.HasPrefix(status, "draw_") {
		c.clearGameForPlayers(game)
		cleanupGameObservers(game.ID)
	}
}

func getUsernameByID(userID int) string {
	if userID == ComputerUserID {
		return "Computer"
	}
	user, err := models.GetUserByID(userID)
	if err != nil {
		return "Unknown"
	}
	return user.Username
}

// AI difficulty storage
var aiGameDifficulty = make(map[int]int)
var aiDifficultyMutex sync.RWMutex

// Game clock management
var activeGameClocks = make(map[int]chan struct{})
var clockMutex sync.RWMutex

func runGameClock(hub *Hub, gameID int) {
	// Create stop channel for this game
	stopChan := make(chan struct{})
	clockMutex.Lock()
	// Check if clock already running for this game
	if existingChan, exists := activeGameClocks[gameID]; exists {
		clockMutex.Unlock()
		close(existingChan) // Stop existing clock
		clockMutex.Lock()
	}
	activeGameClocks[gameID] = stopChan
	clockMutex.Unlock()

	log.Printf("Clock started for game %d", gameID)

	defer func() {
		if r := recover(); r != nil {
			log.Printf("Clock panic for game %d: %v", gameID, r)
		}
		clockMutex.Lock()
		delete(activeGameClocks, gameID)
		clockMutex.Unlock()
		log.Printf("Clock goroutine ended for game %d", gameID)
	}()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	errorCount := 0
	maxErrors := 10

	for {
		select {
		case <-stopChan:
			log.Printf("Clock stopped for game %d via stop channel", gameID)
			return
		case <-ticker.C:
			game, err := models.GetGameByID(gameID)
			if err != nil {
				errorCount++
				log.Printf("Clock error fetching game %d: %v (error %d/%d)", gameID, err, errorCount, maxErrors)
				if errorCount >= maxErrors {
					log.Printf("Too many errors, stopping clock for game %d", gameID)
					return
				}
				continue
			}
			errorCount = 0 // Reset error count on success

			if game.Status != "active" {
				log.Printf("Game %d status is %s, stopping clock", gameID, game.Status)
				return
			}

			if !game.IsTimed() {
				log.Printf("Game %d is not timed, stopping clock", gameID)
				return
			}

			// Check for time expiration
			activeColor := game.GetActiveColor()
			var activeUserID int
			if activeColor == "white" {
				activeUserID = game.WhiteUserID
			} else {
				activeUserID = game.BlackUserID
			}

			whiteTime := game.GetWhiteTimeRemaining()
			blackTime := game.GetBlackTimeRemaining()

			if game.IsTimeExpired(activeUserID) {
				// Time expired - end the game
				var winnerID int
				if activeColor == "white" {
					winnerID = game.BlackUserID
				} else {
					winnerID = game.WhiteUserID
				}

				log.Printf("Game %d: %s ran out of time", gameID, activeColor)
				game.EndGame("timeout", &winnerID)
				updateRatings(game, winnerID)

				// Notify players and observers
				timeoutPayload, _ := json.Marshal(map[string]interface{}{
					"game_id":   game.ID,
					"status":    "timeout",
					"winner_id": winnerID,
					"timeout":   activeUserID,
				})
				msg := WSMessage{Type: "game_update", Payload: timeoutPayload}
				data, _ := json.Marshal(msg)
				hub.SendToUser(game.WhiteUserID, data)
				hub.SendToUser(game.BlackUserID, data)
				broadcastToObservers(hub, game.ID, data)

				// Clear game for players
				hub.mutex.Lock()
				for client := range hub.Clients {
					if client.GameID == game.ID {
						client.GameID = 0
					}
				}
				hub.mutex.Unlock()
				hub.broadcastOnlineUsers()

				return
			}

			// Broadcast clock update to players and observers
			clockPayload, _ := json.Marshal(map[string]interface{}{
				"game_id":       game.ID,
				"white_time_ms": whiteTime,
				"black_time_ms": blackTime,
				"active_color":  activeColor,
			})
			clockMsg := WSMessage{Type: "clock_update", Payload: clockPayload}
			clockData, _ := json.Marshal(clockMsg)
			hub.SendToUser(game.WhiteUserID, clockData)
			hub.SendToUser(game.BlackUserID, clockData)
			broadcastToObservers(hub, game.ID, clockData)
		}
	}
}

func stopGameClock(gameID int) {
	clockMutex.RLock()
	stopChan, exists := activeGameClocks[gameID]
	clockMutex.RUnlock()

	if exists {
		close(stopChan)
	}
}

func (c *Client) handleMove(payload json.RawMessage) {
	log.Printf("handleMove from %s: %s", c.Username, string(payload))

	var moveData GameMove
	if err := json.Unmarshal(payload, &moveData); err != nil {
		log.Printf("Failed to unmarshal move: %v", err)
		return
	}

	log.Printf("Move data: gameID=%d, move=%s, client.GameID=%d", moveData.GameID, moveData.Move, c.GameID)

	if c.GameID != moveData.GameID {
		log.Printf("GameID mismatch: client=%d, move=%d", c.GameID, moveData.GameID)
		return
	}

	game, err := models.GetGameByID(moveData.GameID)
	if err != nil {
		log.Printf("Failed to get game: %v", err)
		return
	}
	if game.Status != "active" {
		log.Printf("Game not active: %s", game.Status)
		return
	}

	log.Printf("Game: white=%d, black=%d, moves=%d", game.WhiteUserID, game.BlackUserID, len(game.Moves))

	if !game.IsPlayerTurn(c.UserID) {
		log.Printf("Not player's turn: userID=%d", c.UserID)
		return
	}

	// Check for time expiration before processing move
	if game.IsTimed() && game.IsTimeExpired(c.UserID) {
		log.Printf("Player %d's time has expired", c.UserID)
		errPayload, _ := json.Marshal(map[string]string{"error": "Your time has expired"})
		msg := WSMessage{Type: "error", Payload: errPayload}
		data, _ := json.Marshal(msg)
		c.Send <- data
		return
	}

	board := chess.ParseFEN(game.FEN)
	move := chess.ParseMove(moveData.Move)

	log.Printf("Validating move: %s -> %s", move.From, move.To)

	if !board.IsValidMove(move) {
		log.Printf("Invalid move: %s", moveData.Move)
		errPayload, _ := json.Marshal(map[string]string{"error": "Invalid move"})
		msg := WSMessage{Type: "error", Payload: errPayload}
		data, _ := json.Marshal(msg)
		c.Send <- data
		return
	}

	log.Printf("Move is valid, applying...")

	// Add current position to history before making the move
	game.AddPositionToHistory(board.PositionKey())

	newBoard := board.MakeMove(move)
	newFEN := newBoard.ToFEN()

	game.AddMove(moveData.Move, newFEN)

	// Update clock if timed game (deduct time and add increment)
	if game.IsTimed() {
		game.DeductTimeAndAddIncrement(c.UserID)
	}

	// Check game status with position history for draw detection
	status := newBoard.GetGameStatusWithHistory(game.PositionHistory)
	var winnerID *int

	if status == "checkmate" {
		game.EndGame("checkmate", &c.UserID)
		winnerID = &c.UserID
		updateRatings(game, c.UserID)
		stopGameClock(game.ID)
	} else if status == "stalemate" || strings.HasPrefix(status, "draw_") {
		game.EndGame(status, nil)
		updateRatings(game, 0)
		stopGameClock(game.ID)
	}

	// Include clock info in move update
	movePayloadData := map[string]interface{}{
		"game_id":   game.ID,
		"move":      moveData.Move,
		"fen":       newFEN,
		"status":    status,
		"winner_id": winnerID,
	}

	// Add clock times if timed game
	if game.IsTimed() {
		movePayloadData["white_time_ms"] = game.GetWhiteTimeRemaining()
		movePayloadData["black_time_ms"] = game.GetBlackTimeRemaining()
	}

	movePayload, _ := json.Marshal(movePayloadData)
	msg := WSMessage{
		Type:    "game_update",
		Payload: movePayload,
	}
	data, _ := json.Marshal(msg)

	c.Hub.SendToUser(game.WhiteUserID, data)
	c.Hub.SendToUser(game.BlackUserID, data)
	broadcastToObservers(c.Hub, game.ID, data)

	if status == "checkmate" || status == "stalemate" || strings.HasPrefix(status, "draw_") {
		c.clearGameForPlayers(game)
		cleanupGameObservers(game.ID)
		return
	}

	// If playing against AI and game is still active, make AI move
	if game.WhiteUserID == ComputerUserID || game.BlackUserID == ComputerUserID {
		c.makeAIMove(game, newBoard)
	}
}

func (c *Client) handleResign(payload json.RawMessage) {
	var resignData struct {
		GameID int `json:"game_id"`
	}
	if err := json.Unmarshal(payload, &resignData); err != nil {
		return
	}

	if c.GameID != resignData.GameID {
		return
	}

	game, err := models.GetGameByID(resignData.GameID)
	if err != nil || game.Status != "active" {
		return
	}

	var winnerID int
	if c.UserID == game.WhiteUserID {
		winnerID = game.BlackUserID
	} else {
		winnerID = game.WhiteUserID
	}

	game.EndGame("resigned", &winnerID)
	updateRatings(game, winnerID)
	stopGameClock(game.ID)

	resignPayload, _ := json.Marshal(map[string]interface{}{
		"game_id":   game.ID,
		"status":    "resigned",
		"winner_id": winnerID,
		"resigned":  c.UserID,
	})
	msg := WSMessage{
		Type:    "game_update",
		Payload: resignPayload,
	}
	data, _ := json.Marshal(msg)

	c.Hub.SendToUser(game.WhiteUserID, data)
	c.Hub.SendToUser(game.BlackUserID, data)
	broadcastToObservers(c.Hub, game.ID, data)
	c.clearGameForPlayers(game)
	cleanupGameObservers(game.ID)
}

func (c *Client) clearGameForPlayers(game *models.Game) {
	c.Hub.mutex.Lock()
	for client := range c.Hub.Clients {
		if client.GameID == game.ID {
			client.GameID = 0
		}
	}
	c.Hub.mutex.Unlock()
	c.Hub.broadcastOnlineUsers()
}

func updateRatings(game *models.Game, winnerID int) {
	white, _ := models.GetUserByID(game.WhiteUserID)
	black, _ := models.GetUserByID(game.BlackUserID)

	if white == nil || black == nil {
		return
	}

	k := 32.0
	expectedWhite := 1.0 / (1.0 + pow10((float64(black.Rating)-float64(white.Rating))/400.0))
	expectedBlack := 1.0 - expectedWhite

	var scoreWhite, scoreBlack float64
	if winnerID == 0 {
		scoreWhite = 0.5
		scoreBlack = 0.5
	} else if winnerID == game.WhiteUserID {
		scoreWhite = 1.0
		scoreBlack = 0.0
	} else {
		scoreWhite = 0.0
		scoreBlack = 1.0
	}

	newWhiteRating := int(float64(white.Rating) + k*(scoreWhite-expectedWhite))
	newBlackRating := int(float64(black.Rating) + k*(scoreBlack-expectedBlack))

	white.UpdateStats(winnerID == white.ID, winnerID == black.ID, winnerID == 0, newWhiteRating)
	black.UpdateStats(winnerID == black.ID, winnerID == white.ID, winnerID == 0, newBlackRating)
}

func pow10(x float64) float64 {
	result := 1.0
	base := 10.0
	for i := 0; i < int(x); i++ {
		result *= base
	}
	frac := x - float64(int(x))
	if frac > 0 {
		result *= 1 + frac*2.302585
	}
	return result
}

// Observer handlers
func (c *Client) handleGetActiveGames() {
	games := getActiveGamesWithPlayers()
	payload, _ := json.Marshal(games)
	msg := WSMessage{Type: "active_games", Payload: payload}
	data, _ := json.Marshal(msg)
	c.Send <- data
}

func getActiveGamesWithPlayers() []map[string]interface{} {
	rows, err := models.GetAllActiveGames()
	if err != nil {
		log.Printf("Error getting active games: %v", err)
		return []map[string]interface{}{}
	}

	var games []map[string]interface{}
	for _, game := range rows {
		whiteUser, _ := models.GetUserByID(game.WhiteUserID)
		blackUser, _ := models.GetUserByID(game.BlackUserID)

		var whiteUsername, blackUsername string
		if whiteUser != nil {
			whiteUsername = whiteUser.Username
		}
		if blackUser != nil {
			blackUsername = blackUser.Username
		}

		// Count observers
		observerMutex.RLock()
		observerCount := len(gameObservers[game.ID])
		observerMutex.RUnlock()

		gameInfo := map[string]interface{}{
			"game_id":        game.ID,
			"white_user_id":  game.WhiteUserID,
			"black_user_id":  game.BlackUserID,
			"white_username": whiteUsername,
			"black_username": blackUsername,
			"move_count":     len(game.Moves),
			"observer_count": observerCount,
		}

		if game.IsTimed() {
			gameInfo["time_control_ms"] = game.TimeControlMS
			gameInfo["increment_ms"] = game.IncrementMS
			gameInfo["white_time_ms"] = game.GetWhiteTimeRemaining()
			gameInfo["black_time_ms"] = game.GetBlackTimeRemaining()
		}

		games = append(games, gameInfo)
	}

	return games
}

func (c *Client) handleObserveGame(payload json.RawMessage) {
	var data struct {
		GameID int `json:"game_id"`
	}
	if err := json.Unmarshal(payload, &data); err != nil {
		log.Printf("Failed to unmarshal observe_game: %v", err)
		return
	}

	// Stop observing current game if any
	c.handleStopObserving()

	// Get game info
	game, err := models.GetGameByID(data.GameID)
	if err != nil {
		errPayload, _ := json.Marshal(map[string]string{"error": "Game not found"})
		msg := WSMessage{Type: "error", Payload: errPayload}
		msgData, _ := json.Marshal(msg)
		c.Send <- msgData
		return
	}

	if game.Status != "active" {
		errPayload, _ := json.Marshal(map[string]string{"error": "Game is not active"})
		msg := WSMessage{Type: "error", Payload: errPayload}
		msgData, _ := json.Marshal(msg)
		c.Send <- msgData
		return
	}

	// Can't observe your own game
	if game.WhiteUserID == c.UserID || game.BlackUserID == c.UserID {
		errPayload, _ := json.Marshal(map[string]string{"error": "You are playing in this game"})
		msg := WSMessage{Type: "error", Payload: errPayload}
		msgData, _ := json.Marshal(msg)
		c.Send <- msgData
		return
	}

	// Add to observers
	observerMutex.Lock()
	if gameObservers[data.GameID] == nil {
		gameObservers[data.GameID] = make(map[*Client]bool)
	}
	gameObservers[data.GameID][c] = true
	c.ObservingGameID = data.GameID
	observerMutex.Unlock()

	log.Printf("User %s started observing game %d", c.Username, data.GameID)

	// Get player usernames
	whiteUser, _ := models.GetUserByID(game.WhiteUserID)
	blackUser, _ := models.GetUserByID(game.BlackUserID)
	var whiteUsername, blackUsername string
	if whiteUser != nil {
		whiteUsername = whiteUser.Username
	}
	if blackUser != nil {
		blackUsername = blackUser.Username
	}

	// Send game state to observer
	gameData := map[string]interface{}{
		"game_id":        game.ID,
		"white_user_id":  game.WhiteUserID,
		"black_user_id":  game.BlackUserID,
		"white_username": whiteUsername,
		"black_username": blackUsername,
		"fen":            game.FEN,
		"moves":          game.Moves,
		"status":         game.Status,
	}

	if game.IsTimed() {
		gameData["time_control_ms"] = game.TimeControlMS
		gameData["increment_ms"] = game.IncrementMS
		gameData["white_time_ms"] = game.GetWhiteTimeRemaining()
		gameData["black_time_ms"] = game.GetBlackTimeRemaining()
	}

	gamePayload, _ := json.Marshal(gameData)
	msg := WSMessage{Type: "observe_game_state", Payload: gamePayload}
	msgData, _ := json.Marshal(msg)
	c.Send <- msgData

	// Broadcast updated observer count
	broadcastObserverCount(c.Hub, data.GameID)
}

func (c *Client) handleStopObserving() {
	if c.ObservingGameID == 0 {
		return
	}

	gameID := c.ObservingGameID
	observerMutex.Lock()
	if observers, ok := gameObservers[gameID]; ok {
		delete(observers, c)
		if len(observers) == 0 {
			delete(gameObservers, gameID)
		}
	}
	c.ObservingGameID = 0
	observerMutex.Unlock()

	log.Printf("User %s stopped observing game %d", c.Username, gameID)

	// Broadcast updated observer count
	broadcastObserverCount(c.Hub, gameID)
}

func broadcastToObservers(hub *Hub, gameID int, data []byte) {
	observerMutex.RLock()
	observers := gameObservers[gameID]
	observerMutex.RUnlock()

	for client := range observers {
		select {
		case client.Send <- data:
		default:
		}
	}
}

func (c *Client) handleMarkRead(payload json.RawMessage) {
	var data struct {
		FromUserID int `json:"from_user_id"`
	}
	if err := json.Unmarshal(payload, &data); err != nil {
		return
	}

	// Mark all messages from this sender as read
	models.MarkMessagesReadByUser(c.UserID, data.FromUserID)

	// Notify sender that messages were read
	readPayload, _ := json.Marshal(map[string]interface{}{
		"reader_id":       c.UserID,
		"reader_username": c.Username,
	})
	msg := WSMessage{Type: "messages_read", Payload: readPayload}
	msgData, _ := json.Marshal(msg)
	c.Hub.SendToUser(data.FromUserID, msgData)
}

func cleanupGameObservers(gameID int) {
	observerMutex.Lock()
	if observers, ok := gameObservers[gameID]; ok {
		for client := range observers {
			client.ObservingGameID = 0
			// Notify observer that game ended
			payload, _ := json.Marshal(map[string]interface{}{
				"game_id": gameID,
				"message": "Game has ended",
			})
			msg := WSMessage{Type: "observe_game_ended", Payload: payload}
			data, _ := json.Marshal(msg)
			select {
			case client.Send <- data:
			default:
			}
		}
		delete(gameObservers, gameID)
	}
	observerMutex.Unlock()
}

func broadcastObserverCount(hub *Hub, gameID int) {
	observerMutex.RLock()
	count := len(gameObservers[gameID])
	observerMutex.RUnlock()

	payload, _ := json.Marshal(map[string]interface{}{
		"game_id":        gameID,
		"observer_count": count,
	})
	msg := WSMessage{Type: "observer_count", Payload: payload}
	data, _ := json.Marshal(msg)

	// Send to players
	game, err := models.GetGameByID(gameID)
	if err == nil {
		hub.SendToUser(game.WhiteUserID, data)
		hub.SendToUser(game.BlackUserID, data)
	}

	// Send to observers
	broadcastToObservers(hub, gameID, data)
}

func (c *Client) handleChat(payload json.RawMessage) {
	var chatData ChatMessage
	if err := json.Unmarshal(payload, &chatData); err != nil {
		return
	}

	if c.GameID != chatData.GameID {
		return
	}

	content := chatData.Content
	hasProfanity := containsProfanity(content)
	if models.IsProfanityFilterEnabled() {
		content = filterProfanity(content)
	}

	// Send warning if profanity was detected
	if hasProfanity {
		c.sendProfanityWarning()
	}

	gameID := chatData.GameID
	models.CreateMessage(c.UserID, nil, &gameID, content)

	chatPayload, _ := json.Marshal(map[string]interface{}{
		"game_id":       chatData.GameID,
		"from_user_id":  c.UserID,
		"from_username": c.Username,
		"content":       content,
	})
	msg := WSMessage{
		Type:    "game_chat",
		Payload: chatPayload,
	}
	data, _ := json.Marshal(msg)

	game, _ := models.GetGameByID(chatData.GameID)
	if game != nil {
		c.Hub.SendToUser(game.WhiteUserID, data)
		c.Hub.SendToUser(game.BlackUserID, data)
	}
}

func (c *Client) handleDirectMessage(payload json.RawMessage) {
	var dmData struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(payload, &dmData); err != nil {
		return
	}

	content := dmData.Content
	hasProfanity := containsProfanity(content)
	if models.IsProfanityFilterEnabled() {
		content = filterProfanity(content)
	}

	// Send warning if profanity was detected
	if hasProfanity {
		c.sendProfanityWarning()
	}

	atPattern := regexp.MustCompile(`@(\w+)`)
	matches := atPattern.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) >= 2 {
			targetUsername := match[1]
			targetUser, err := models.GetUserByUsername(targetUsername)
			if err != nil || targetUser == nil {
				continue
			}

			models.CreateMessage(c.UserID, &targetUser.ID, nil, content)

			dmPayload, _ := json.Marshal(map[string]interface{}{
				"from_user_id":  c.UserID,
				"from_username": c.Username,
				"content":       content,
			})
			msg := WSMessage{
				Type:    "direct_message",
				Payload: dmPayload,
			}
			data, _ := json.Marshal(msg)
			c.Hub.SendToUser(targetUser.ID, data)
		}
	}
}

var profanityList = []string{
	"fuck", "shit", "ass", "damn", "bitch", "crap", "piss", "dick", "cock",
	"pussy", "asshole", "bastard", "slut", "whore", "cunt", "fag", "nigger",
	"retard", "douche",
}

func containsProfanity(content string) bool {
	lower := strings.ToLower(content)
	for _, word := range profanityList {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}

func (c *Client) sendProfanityWarning() {
	if !models.IsProfanityAutoWarnEnabled() {
		return
	}

	warningPayload, _ := json.Marshal(map[string]interface{}{
		"from_username": "Admin",
		"from_user_id":  0,
		"content":       "Warning: Please refrain from using profanity. Continued use may result in account action.",
	})
	msg := WSMessage{
		Type:    "direct_message",
		Payload: warningPayload,
	}
	data, _ := json.Marshal(msg)
	c.Send <- data
}

func filterProfanity(content string) string {
	lower := strings.ToLower(content)
	result := content

	for _, word := range profanityList {
		if strings.Contains(lower, word) {
			replacement := strings.Repeat("*", len(word))
			re := regexp.MustCompile("(?i)" + regexp.QuoteMeta(word))
			result = re.ReplaceAllString(result, replacement)
			lower = strings.ToLower(result)
		}
	}

	return result
}
