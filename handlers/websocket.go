package handlers

import (
	"chess-server/chess"
	"chess-server/models"
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"

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
	Hub      *Hub
	Conn     *websocket.Conn
	Send     chan []byte
	UserID   int
	Username string
	GameID   int
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
	FromUserID   int    `json:"from_user_id"`
	FromUsername string `json:"from_username"`
	ToUserID     int    `json:"to_user_id"`
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
var pendingInvites = make(map[int]*GameInvite)
var inviteMutex sync.RWMutex

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
}

func (c *Client) readPump() {
	defer func() {
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
	}
}

func (c *Client) handleInvite(payload json.RawMessage) {
	log.Printf("Received invite from %s, payload: %s", c.Username, string(payload))

	var invite struct {
		ToUserID int `json:"to_user_id"`
	}
	if err := json.Unmarshal(payload, &invite); err != nil {
		log.Printf("Failed to unmarshal invite: %v", err)
		return
	}

	log.Printf("Invite to user ID: %d", invite.ToUserID)

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

	gameInvite := &GameInvite{
		FromUserID:   c.UserID,
		FromUsername: c.Username,
		ToUserID:     invite.ToUserID,
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

	log.Printf("Creating game between %d and %d", invite.FromUserID, c.UserID)

	whiteID, blackID := determineColors(invite.FromUserID, c.UserID)

	game, err := models.CreateGame(whiteID, blackID)
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
		"game_id":       game.ID,
		"white_user_id": whiteID,
		"black_user_id": blackID,
		"fen":           game.FEN,
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

	newBoard := board.MakeMove(move)
	newFEN := newBoard.ToFEN()

	game.AddMove(moveData.Move, newFEN)

	status := newBoard.GetGameStatus()
	var winnerID *int

	if status == "checkmate" {
		game.EndGame("checkmate", &c.UserID)
		winnerID = &c.UserID
		updateRatings(game, c.UserID)
	} else if status == "stalemate" {
		game.EndGame("stalemate", nil)
		updateRatings(game, 0)
	}

	movePayload, _ := json.Marshal(map[string]interface{}{
		"game_id":   game.ID,
		"move":      moveData.Move,
		"fen":       newFEN,
		"status":    status,
		"winner_id": winnerID,
	})
	msg := WSMessage{
		Type:    "game_update",
		Payload: movePayload,
	}
	data, _ := json.Marshal(msg)

	c.Hub.SendToUser(game.WhiteUserID, data)
	c.Hub.SendToUser(game.BlackUserID, data)

	if status == "checkmate" || status == "stalemate" {
		c.clearGameForPlayers(game)
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
	c.clearGameForPlayers(game)
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
