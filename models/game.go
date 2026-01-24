package models

import (
	"chess-server/database"
	"encoding/json"
	"time"
)

type Game struct {
	ID          int        `json:"id"`
	WhiteUserID int        `json:"white_user_id"`
	BlackUserID int        `json:"black_user_id"`
	Moves       []string   `json:"moves"`
	FEN         string     `json:"fen"`
	Status      string     `json:"status"`
	WinnerID    *int       `json:"winner_id"`
	CreatedAt   time.Time  `json:"created_at"`
	EndedAt     *time.Time `json:"ended_at"`
}

func CreateGame(whiteUserID, blackUserID int) (*Game, error) {
	result, err := database.DB.Exec(
		"INSERT INTO games (white_user_id, black_user_id) VALUES (?, ?)",
		whiteUserID, blackUserID,
	)
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
	var movesJSON string

	err := database.DB.QueryRow(`
		SELECT id, white_user_id, black_user_id, moves, fen, status, winner_id, created_at, ended_at
		FROM games WHERE id = ?
	`, id).Scan(
		&game.ID, &game.WhiteUserID, &game.BlackUserID, &movesJSON, &game.FEN,
		&game.Status, &game.WinnerID, &game.CreatedAt, &game.EndedAt,
	)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(movesJSON), &game.Moves); err != nil {
		game.Moves = []string{}
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

	_, err = database.DB.Exec(
		"UPDATE games SET moves = ?, fen = ? WHERE id = ?",
		string(movesJSON), newFEN, g.ID,
	)
	return err
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
	var movesJSON string

	err := database.DB.QueryRow(`
		SELECT id, white_user_id, black_user_id, moves, fen, status, winner_id, created_at, ended_at
		FROM games
		WHERE (white_user_id = ? OR black_user_id = ?) AND status = 'active'
		ORDER BY created_at DESC
		LIMIT 1
	`, userID, userID).Scan(
		&game.ID, &game.WhiteUserID, &game.BlackUserID, &movesJSON, &game.FEN,
		&game.Status, &game.WinnerID, &game.CreatedAt, &game.EndedAt,
	)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(movesJSON), &game.Moves); err != nil {
		game.Moves = []string{}
	}

	return game, nil
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
