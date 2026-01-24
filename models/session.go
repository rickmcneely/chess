package models

import (
	"chess-server/database"
	"crypto/rand"
	"encoding/hex"
	"time"
)

type Session struct {
	ID        string    `json:"id"`
	UserID    int       `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

func generateSessionID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func CreateSession(userID int) (*Session, error) {
	sessionID, err := generateSessionID()
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().Add(24 * time.Hour)

	_, err = database.DB.Exec(
		"INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)",
		sessionID, userID, expiresAt,
	)
	if err != nil {
		return nil, err
	}

	return &Session{
		ID:        sessionID,
		UserID:    userID,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
	}, nil
}

func GetSession(sessionID string) (*Session, error) {
	session := &Session{}
	err := database.DB.QueryRow(`
		SELECT id, user_id, created_at, expires_at
		FROM sessions WHERE id = ? AND expires_at > ?
	`, sessionID, time.Now()).Scan(
		&session.ID, &session.UserID, &session.CreatedAt, &session.ExpiresAt,
	)
	if err != nil {
		return nil, err
	}
	return session, nil
}

func DeleteSession(sessionID string) error {
	_, err := database.DB.Exec("DELETE FROM sessions WHERE id = ?", sessionID)
	return err
}

func CleanExpiredSessions() error {
	_, err := database.DB.Exec("DELETE FROM sessions WHERE expires_at < ?", time.Now())
	return err
}
