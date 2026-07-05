package models

import (
	"chess-server/database"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID             int       `json:"id"`
	Username       string    `json:"username"`
	Email          string    `json:"email"`
	PasswordHash   string    `json:"-"`
	IsAdmin        bool      `json:"is_admin"`
	Approved       bool      `json:"approved"`
	Rating         int       `json:"rating"`
	GamesPlayed    int       `json:"games_played"`
	Wins           int       `json:"wins"`
	Losses         int       `json:"losses"`
	Draws          int       `json:"draws"`
	LastOpponentID *int      `json:"last_opponent_id"`
	LastColor      *string   `json:"last_color"`
	CreatedAt      time.Time `json:"created_at"`
}

func CreateUser(username, email, password string) (*User, error) {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	result, err := database.DB.Exec(
		"INSERT INTO users (username, email, password_hash) VALUES (?, ?, ?)",
		username, email, string(passwordHash),
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return GetUserByID(int(id))
}

func GetUserByID(id int) (*User, error) {
	user := &User{}
	err := database.DB.QueryRow(`
		SELECT id, username, email, password_hash, is_admin, approved, rating,
		       games_played, wins, losses, draws, last_opponent_id, last_color, created_at
		FROM users WHERE id = ?
	`, id).Scan(
		&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.IsAdmin, &user.Approved,
		&user.Rating, &user.GamesPlayed, &user.Wins, &user.Losses, &user.Draws,
		&user.LastOpponentID, &user.LastColor, &user.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func GetUserByUsername(username string) (*User, error) {
	user := &User{}
	err := database.DB.QueryRow(`
		SELECT id, username, email, password_hash, is_admin, approved, rating,
		       games_played, wins, losses, draws, last_opponent_id, last_color, created_at
		FROM users WHERE username = ?
	`, username).Scan(
		&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.IsAdmin, &user.Approved,
		&user.Rating, &user.GamesPlayed, &user.Wins, &user.Losses, &user.Draws,
		&user.LastOpponentID, &user.LastColor, &user.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (u *User) CheckPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password))
	return err == nil
}

func GetPendingUsers() ([]*User, error) {
	rows, err := database.DB.Query(`
		SELECT id, username, email, password_hash, is_admin, approved, rating,
		       games_played, wins, losses, draws, last_opponent_id, last_color, created_at
		FROM users WHERE approved = FALSE AND is_admin = FALSE
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		user := &User{}
		err := rows.Scan(
			&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.IsAdmin, &user.Approved,
			&user.Rating, &user.GamesPlayed, &user.Wins, &user.Losses, &user.Draws,
			&user.LastOpponentID, &user.LastColor, &user.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}

func GetAllUsers() ([]*User, error) {
	rows, err := database.DB.Query(`
		SELECT id, username, email, password_hash, is_admin, approved, rating,
		       games_played, wins, losses, draws, last_opponent_id, last_color, created_at
		FROM users ORDER BY rating DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		user := &User{}
		err := rows.Scan(
			&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.IsAdmin, &user.Approved,
			&user.Rating, &user.GamesPlayed, &user.Wins, &user.Losses, &user.Draws,
			&user.LastOpponentID, &user.LastColor, &user.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}

func ApproveUser(userID int) error {
	_, err := database.DB.Exec("UPDATE users SET approved = TRUE WHERE id = ?", userID)
	return err
}

func RejectUser(userID int) error {
	_, err := database.DB.Exec("DELETE FROM users WHERE id = ? AND approved = FALSE", userID)
	return err
}

func DeleteUser(userID int) error {
	// Don't allow deleting admin users
	var isAdmin bool
	err := database.DB.QueryRow("SELECT is_admin FROM users WHERE id = ?", userID).Scan(&isAdmin)
	if err != nil {
		return err
	}
	if isAdmin {
		return nil // Silently ignore admin deletion
	}

	// Delete user's messages
	database.DB.Exec("DELETE FROM messages WHERE from_user_id = ? OR to_user_id = ?", userID, userID)

	// Delete user's games
	database.DB.Exec("DELETE FROM games WHERE white_user_id = ? OR black_user_id = ?", userID, userID)

	// Delete the user
	_, err = database.DB.Exec("DELETE FROM users WHERE id = ? AND is_admin = FALSE", userID)
	return err
}

func (u *User) UpdateLastGame(opponentID int, color string) error {
	_, err := database.DB.Exec(
		"UPDATE users SET last_opponent_id = ?, last_color = ? WHERE id = ?",
		opponentID, color, u.ID,
	)
	return err
}

func (u *User) UpdateStats(win, loss, draw bool, newRating int) error {
	winsAdd, lossesAdd, drawsAdd := 0, 0, 0
	if win {
		winsAdd = 1
	} else if loss {
		lossesAdd = 1
	} else if draw {
		drawsAdd = 1
	}

	_, err := database.DB.Exec(`
		UPDATE users SET
			games_played = games_played + 1,
			wins = wins + ?,
			losses = losses + ?,
			draws = draws + ?,
			rating = ?
		WHERE id = ?
	`, winsAdd, lossesAdd, drawsAdd, newRating, u.ID)
	return err
}

func GetLeaderboard() ([]*User, error) {
	rows, err := database.DB.Query(`
		SELECT id, username, email, password_hash, is_admin, approved, rating,
		       games_played, wins, losses, draws, last_opponent_id, last_color, created_at
		FROM users WHERE approved = TRUE
		ORDER BY rating DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		user := &User{}
		err := rows.Scan(
			&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.IsAdmin, &user.Approved,
			&user.Rating, &user.GamesPlayed, &user.Wins, &user.Losses, &user.Draws,
			&user.LastOpponentID, &user.LastColor, &user.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}
