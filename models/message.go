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
	Delivered  bool      `json:"delivered"`
	ReadAt     *time.Time `json:"read_at"`
}

func CreateMessage(fromUserID int, toUserID, gameID *int, content string) (*Message, error) {
	result, err := database.DB.Exec(
		"INSERT INTO messages (from_user_id, to_user_id, game_id, content, delivered) VALUES (?, ?, ?, ?, FALSE)",
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

// GetUndeliveredMessages gets all undelivered DMs for a user
func GetUndeliveredMessages(userID int) ([]*Message, error) {
	rows, err := database.DB.Query(`
		SELECT m.id, m.from_user_id, m.to_user_id, m.game_id, m.content, m.created_at, u.username
		FROM messages m
		JOIN users u ON m.from_user_id = u.id
		WHERE m.to_user_id = ? AND m.game_id IS NULL AND (m.delivered = FALSE OR m.delivered IS NULL)
		ORDER BY m.created_at ASC
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

// MarkMessageDelivered marks a message as delivered
func MarkMessageDelivered(messageID int) error {
	_, err := database.DB.Exec("UPDATE messages SET delivered = TRUE WHERE id = ?", messageID)
	return err
}

// MarkMessagesDelivered marks multiple messages as delivered
func MarkMessagesDelivered(messageIDs []int) error {
	for _, id := range messageIDs {
		MarkMessageDelivered(id)
	}
	return nil
}

// MarkMessageRead marks a message as read
func MarkMessageRead(messageID int) error {
	_, err := database.DB.Exec("UPDATE messages SET read_at = CURRENT_TIMESTAMP WHERE id = ?", messageID)
	return err
}

// MarkMessagesReadByUser marks all messages to a user from a sender as read
func MarkMessagesReadByUser(toUserID, fromUserID int) error {
	_, err := database.DB.Exec(`
		UPDATE messages SET read_at = CURRENT_TIMESTAMP
		WHERE to_user_id = ? AND from_user_id = ? AND read_at IS NULL
	`, toUserID, fromUserID)
	return err
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
