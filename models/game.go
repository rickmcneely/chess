package models

import (
	"chess-server/database"
	"encoding/json"
	"time"
)

type Game struct {
	ID                 int        `json:"id"`
	WhiteUserID        int        `json:"white_user_id"`
	BlackUserID        int        `json:"black_user_id"`
	Moves              []string   `json:"moves"`
	PositionHistory    []string   `json:"position_history"` // For threefold repetition
	FEN                string     `json:"fen"`
	Status             string     `json:"status"`
	WinnerID           *int       `json:"winner_id"`
	CreatedAt          time.Time  `json:"created_at"`
	EndedAt            *time.Time `json:"ended_at"`
	TimeControlMS      *int       `json:"time_control_ms"`      // Initial time in milliseconds (NULL = untimed)
	IncrementMS        int        `json:"increment_ms"`         // Time increment per move
	WhiteTimeRemaining *int       `json:"white_time_remaining"` // Current remaining time for white
	BlackTimeRemaining *int       `json:"black_time_remaining"` // Current remaining time for black
	LastMoveAt         *time.Time `json:"last_move_at"`         // Timestamp of last move
}

func CreateGame(whiteUserID, blackUserID int) (*Game, error) {
	return CreateGameWithClock(whiteUserID, blackUserID, nil, 0)
}

func CreateGameWithClock(whiteUserID, blackUserID int, timeControlMS *int, incrementMS int) (*Game, error) {
	var result interface{ LastInsertId() (int64, error) }
	var err error

	if timeControlMS != nil && *timeControlMS > 0 {
		// Timed game
		now := time.Now()
		result, err = database.DB.Exec(
			`INSERT INTO games (white_user_id, black_user_id, time_control_ms, increment_ms,
			 white_time_remaining, black_time_remaining, last_move_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			whiteUserID, blackUserID, *timeControlMS, incrementMS,
			*timeControlMS, *timeControlMS, now,
		)
	} else {
		// Untimed game
		result, err = database.DB.Exec(
			"INSERT INTO games (white_user_id, black_user_id) VALUES (?, ?)",
			whiteUserID, blackUserID,
		)
	}

	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return GetGameByID(int(id))
}

func GetGameByID(id int) (*Game, error) {
	game := &Game{}
	var movesJSON, positionHistoryJSON string

	err := database.DB.QueryRow(`
		SELECT id, white_user_id, black_user_id, moves, COALESCE(position_history, '[]'), fen, status, winner_id, created_at, ended_at,
		       time_control_ms, COALESCE(increment_ms, 0), white_time_remaining, black_time_remaining, last_move_at
		FROM games WHERE id = ?
	`, id).Scan(
		&game.ID, &game.WhiteUserID, &game.BlackUserID, &movesJSON, &positionHistoryJSON, &game.FEN,
		&game.Status, &game.WinnerID, &game.CreatedAt, &game.EndedAt,
		&game.TimeControlMS, &game.IncrementMS, &game.WhiteTimeRemaining, &game.BlackTimeRemaining, &game.LastMoveAt,
	)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(movesJSON), &game.Moves); err != nil {
		game.Moves = []string{}
	}
	if err := json.Unmarshal([]byte(positionHistoryJSON), &game.PositionHistory); err != nil {
		game.PositionHistory = []string{}
	}

	return game, nil
}

func (g *Game) AddMove(move string, newFEN string) error {
	g.Moves = append(g.Moves, move)
	g.FEN = newFEN

	movesJSON, err := json.Marshal(g.Moves)
	if err != nil {
		return err
	}

	positionHistoryJSON, err := json.Marshal(g.PositionHistory)
	if err != nil {
		return err
	}

	_, err = database.DB.Exec(
		"UPDATE games SET moves = ?, position_history = ?, fen = ? WHERE id = ?",
		string(movesJSON), string(positionHistoryJSON), newFEN, g.ID,
	)
	return err
}

func (g *Game) AddPositionToHistory(positionKey string) {
	g.PositionHistory = append(g.PositionHistory, positionKey)
}

func (g *Game) EndGame(status string, winnerID *int) error {
	g.Status = status
	g.WinnerID = winnerID
	now := time.Now()
	g.EndedAt = &now

	_, err := database.DB.Exec(
		"UPDATE games SET status = ?, winner_id = ?, ended_at = ? WHERE id = ?",
		status, winnerID, now, g.ID,
	)
	return err
}

func GetActiveGameForUser(userID int) (*Game, error) {
	game := &Game{}
	var movesJSON, positionHistoryJSON string

	err := database.DB.QueryRow(`
		SELECT id, white_user_id, black_user_id, moves, COALESCE(position_history, '[]'), fen, status, winner_id, created_at, ended_at,
		       time_control_ms, COALESCE(increment_ms, 0), white_time_remaining, black_time_remaining, last_move_at
		FROM games
		WHERE (white_user_id = ? OR black_user_id = ?) AND status = 'active'
		ORDER BY created_at DESC
		LIMIT 1
	`, userID, userID).Scan(
		&game.ID, &game.WhiteUserID, &game.BlackUserID, &movesJSON, &positionHistoryJSON, &game.FEN,
		&game.Status, &game.WinnerID, &game.CreatedAt, &game.EndedAt,
		&game.TimeControlMS, &game.IncrementMS, &game.WhiteTimeRemaining, &game.BlackTimeRemaining, &game.LastMoveAt,
	)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(movesJSON), &game.Moves); err != nil {
		game.Moves = []string{}
	}
	if err := json.Unmarshal([]byte(positionHistoryJSON), &game.PositionHistory); err != nil {
		game.PositionHistory = []string{}
	}

	return game, nil
}

func GetAllActiveGames() ([]*Game, error) {
	rows, err := database.DB.Query(`
		SELECT id, white_user_id, black_user_id, moves, COALESCE(position_history, '[]'), fen, status, winner_id, created_at, ended_at,
		       time_control_ms, COALESCE(increment_ms, 0), white_time_remaining, black_time_remaining, last_move_at
		FROM games
		WHERE status = 'active'
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var games []*Game
	for rows.Next() {
		game := &Game{}
		var movesJSON, positionHistoryJSON string
		err := rows.Scan(
			&game.ID, &game.WhiteUserID, &game.BlackUserID, &movesJSON, &positionHistoryJSON, &game.FEN,
			&game.Status, &game.WinnerID, &game.CreatedAt, &game.EndedAt,
			&game.TimeControlMS, &game.IncrementMS, &game.WhiteTimeRemaining, &game.BlackTimeRemaining, &game.LastMoveAt,
		)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(movesJSON), &game.Moves); err != nil {
			game.Moves = []string{}
		}
		if err := json.Unmarshal([]byte(positionHistoryJSON), &game.PositionHistory); err != nil {
			game.PositionHistory = []string{}
		}
		games = append(games, game)
	}
	return games, nil
}

func GetGameHistory(userID int) ([]*Game, error) {
	rows, err := database.DB.Query(`
		SELECT id, white_user_id, black_user_id, moves, fen, status, winner_id, created_at, ended_at
		FROM games
		WHERE (white_user_id = ? OR black_user_id = ?) AND status != 'active'
		ORDER BY created_at DESC
		LIMIT 50
	`, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var games []*Game
	for rows.Next() {
		game := &Game{}
		var movesJSON string
		err := rows.Scan(
			&game.ID, &game.WhiteUserID, &game.BlackUserID, &movesJSON, &game.FEN,
			&game.Status, &game.WinnerID, &game.CreatedAt, &game.EndedAt,
		)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(movesJSON), &game.Moves); err != nil {
			game.Moves = []string{}
		}
		games = append(games, game)
	}
	return games, nil
}

func (g *Game) IsPlayerTurn(userID int) bool {
	moveCount := len(g.Moves)
	if moveCount%2 == 0 {
		return userID == g.WhiteUserID
	}
	return userID == g.BlackUserID
}

func (g *Game) GetPlayerColor(userID int) string {
	if userID == g.WhiteUserID {
		return "white"
	}
	return "black"
}

// IsTimed returns true if this game has a time control
func (g *Game) IsTimed() bool {
	return g.TimeControlMS != nil && *g.TimeControlMS > 0
}

// GetActiveColor returns "white" or "black" based on whose turn it is
func (g *Game) GetActiveColor() string {
	if len(g.Moves)%2 == 0 {
		return "white"
	}
	return "black"
}

// GetPlayerTimeRemaining calculates the remaining time for a player,
// accounting for elapsed time since last move if it's their turn
func (g *Game) GetPlayerTimeRemaining(userID int) int {
	if !g.IsTimed() {
		return 0
	}

	isWhite := userID == g.WhiteUserID
	var remaining *int
	if isWhite {
		remaining = g.WhiteTimeRemaining
	} else {
		remaining = g.BlackTimeRemaining
	}

	if remaining == nil {
		return 0
	}

	// If it's this player's turn, subtract elapsed time since last move
	activeColor := g.GetActiveColor()
	isPlayerTurn := (isWhite && activeColor == "white") || (!isWhite && activeColor == "black")

	if isPlayerTurn && g.LastMoveAt != nil {
		elapsed := int(time.Since(*g.LastMoveAt).Milliseconds())
		result := *remaining - elapsed
		if result < 0 {
			return 0
		}
		return result
	}

	return *remaining
}

// GetWhiteTimeRemaining returns white's remaining time including elapsed time
func (g *Game) GetWhiteTimeRemaining() int {
	return g.GetPlayerTimeRemaining(g.WhiteUserID)
}

// GetBlackTimeRemaining returns black's remaining time including elapsed time
func (g *Game) GetBlackTimeRemaining() int {
	return g.GetPlayerTimeRemaining(g.BlackUserID)
}

// IsTimeExpired checks if a player has run out of time
func (g *Game) IsTimeExpired(userID int) bool {
	if !g.IsTimed() {
		return false
	}
	return g.GetPlayerTimeRemaining(userID) <= 0
}

// DeductTimeAndAddIncrement updates the clock after a move
// This should be called after a player makes a move
func (g *Game) DeductTimeAndAddIncrement(userID int) error {
	if !g.IsTimed() {
		return nil
	}

	now := time.Now()
	isWhite := userID == g.WhiteUserID

	// Calculate time used
	var elapsed int64 = 0
	if g.LastMoveAt != nil {
		elapsed = now.Sub(*g.LastMoveAt).Milliseconds()
	}

	// Update the player who moved
	var newTime int
	if isWhite {
		if g.WhiteTimeRemaining != nil {
			newTime = *g.WhiteTimeRemaining - int(elapsed) + g.IncrementMS
			if newTime < 0 {
				newTime = 0
			}
			g.WhiteTimeRemaining = &newTime
		}
	} else {
		if g.BlackTimeRemaining != nil {
			newTime = *g.BlackTimeRemaining - int(elapsed) + g.IncrementMS
			if newTime < 0 {
				newTime = 0
			}
			g.BlackTimeRemaining = &newTime
		}
	}

	g.LastMoveAt = &now

	// Persist to database
	_, err := database.DB.Exec(
		"UPDATE games SET white_time_remaining = ?, black_time_remaining = ?, last_move_at = ? WHERE id = ?",
		g.WhiteTimeRemaining, g.BlackTimeRemaining, now, g.ID,
	)
	return err
}
