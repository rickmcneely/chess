package models

import (
	"chess-server/database"
	"time"
)

type Message struct {
	ID         int       `json:"id"`
	FromUserID int       `json:"from_user_id"`
	ToUserID   *int      `json:"to_user_id"`
	GameID     *int      `json:"game_id"`
	Content    string    `json:"content"`
	CreatedAt  time.Time `json:"created_at"`
	FromUser   string    `json:"from_username"`
}

func CreateMessage(fromUserID int, toUserID, gameID *int, content string) (*Message, error) {
	result, err := database.DB.Exec(
		"INSERT INTO messages (from_user_id, to_user_id, game_id, content) VALUES (?, ?, ?, ?)",
		fromUserID, toUserID, gameID, content,
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return GetMessageByID(int(id))
}

func GetMessageByID(id int) (*Message, error) {
	msg := &Message{}
	err := database.DB.QueryRow(`
		SELECT m.id, m.from_user_id, m.to_user_id, m.game_id, m.content, m.created_at, u.username
		FROM messages m
		JOIN users u ON m.from_user_id = u.id
		WHERE m.id = ?
	`, id).Scan(
		&msg.ID, &msg.FromUserID, &msg.ToUserID, &msg.GameID, &msg.Content, &msg.CreatedAt, &msg.FromUser,
	)
	if err != nil {
		return nil, err
	}
	return msg, nil
}

func GetGameMessages(gameID int) ([]*Message, error) {
	rows, err := database.DB.Query(`
		SELECT m.id, m.from_user_id, m.to_user_id, m.game_id, m.content, m.created_at, u.username
		FROM messages m
		JOIN users u ON m.from_user_id = u.id
		WHERE m.game_id = ?
		ORDER BY m.created_at ASC
	`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*Message
	for rows.Next() {
		msg := &Message{}
		err := rows.Scan(
			&msg.ID, &msg.FromUserID, &msg.ToUserID, &msg.GameID, &msg.Content, &msg.CreatedAt, &msg.FromUser,
		)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

func GetDirectMessages(userID int) ([]*Message, error) {
	rows, err := database.DB.Query(`
		SELECT m.id, m.from_user_id, m.to_user_id, m.game_id, m.content, m.created_at, u.username
		FROM messages m
		JOIN users u ON m.from_user_id = u.id
		WHERE m.to_user_id = ? AND m.game_id IS NULL
		ORDER BY m.created_at DESC
		LIMIT 100
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*Message
	for rows.Next() {
		msg := &Message{}
		err := rows.Scan(
			&msg.ID, &msg.FromUserID, &msg.ToUserID, &msg.GameID, &msg.Content, &msg.CreatedAt, &msg.FromUser,
		)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, nil
}
