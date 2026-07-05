package database

import (
	"database/sql"
	"log"

	_ "modernc.org/sqlite"
	"golang.org/x/crypto/bcrypt"
)

var DB *sql.DB

func Initialize(dbPath string) error {
	var err error
	DB, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}

	if err = DB.Ping(); err != nil {
		return err
	}

	if err = createTables(); err != nil {
		return err
	}

	if err = createDefaultAdmin(); err != nil {
		return err
	}

	if err = initializeSettings(); err != nil {
		return err
	}

	return nil
}

func createTables() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		email TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		is_admin BOOLEAN DEFAULT FALSE,
		approved BOOLEAN DEFAULT FALSE,
		rating INTEGER DEFAULT 1200,
		games_played INTEGER DEFAULT 0,
		wins INTEGER DEFAULT 0,
		losses INTEGER DEFAULT 0,
		draws INTEGER DEFAULT 0,
		last_opponent_id INTEGER DEFAULT NULL,
		last_color TEXT DEFAULT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS games (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		white_user_id INTEGER NOT NULL,
		black_user_id INTEGER NOT NULL,
		moves TEXT DEFAULT '[]',
		position_history TEXT DEFAULT '[]',
		fen TEXT DEFAULT 'rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1',
		status TEXT DEFAULT 'active',
		winner_id INTEGER DEFAULT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		ended_at DATETIME DEFAULT NULL,
		FOREIGN KEY (white_user_id) REFERENCES users(id),
		FOREIGN KEY (black_user_id) REFERENCES users(id),
		FOREIGN KEY (winner_id) REFERENCES users(id)
	);

	CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		from_user_id INTEGER NOT NULL,
		to_user_id INTEGER DEFAULT NULL,
		game_id INTEGER DEFAULT NULL,
		content TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (from_user_id) REFERENCES users(id),
		FOREIGN KEY (to_user_id) REFERENCES users(id),
		FOREIGN KEY (game_id) REFERENCES games(id)
	);

	CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		user_id INTEGER NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		expires_at DATETIME NOT NULL,
		FOREIGN KEY (user_id) REFERENCES users(id)
	);
	`

	_, err := DB.Exec(schema)
	return err
}

func createDefaultAdmin() error {
	var count int
	err := DB.QueryRow("SELECT COUNT(*) FROM users WHERE is_admin = TRUE").Scan(&count)
	if err != nil {
		return err
	}

	if count > 0 {
		return nil
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte("chess2024"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	_, err = DB.Exec(
		"INSERT INTO users (username, email, password_hash, is_admin, approved) VALUES (?, ?, ?, TRUE, TRUE)",
		"admin", "admin@chess.local", string(passwordHash),
	)
	if err != nil {
		log.Printf("Default admin already exists or error: %v", err)
	} else {
		log.Println("Created default admin user (admin/chess2024)")
	}

	return nil
}

func initializeSettings() error {
	_, err := DB.Exec(`
		INSERT OR IGNORE INTO settings (key, value) VALUES ('profanity_filter', 'true')
	`)
	return err
}

func CreateComputerUser() (int, error) {
	// Check if Computer user already exists
	var id int
	err := DB.QueryRow("SELECT id FROM users WHERE username = 'Computer'").Scan(&id)
	if err == nil {
		return id, nil // Already exists
	}

	// Create Computer user (no password needed, can't login)
	result, err := DB.Exec(
		"INSERT INTO users (username, email, password_hash, is_admin, approved) VALUES (?, ?, ?, FALSE, TRUE)",
		"Computer", "computer@chess.local", "AI_NO_LOGIN",
	)
	if err != nil {
		return 0, err
	}

	lastID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	log.Println("Created Computer user for AI games")
	return int(lastID), nil
}

func GetComputerUserID() int {
	var id int
	err := DB.QueryRow("SELECT id FROM users WHERE username = 'Computer'").Scan(&id)
	if err != nil {
		return 0
	}
	return id
}

func RunMigrations() error {
	// Add position_history column if it doesn't exist
	_, err := DB.Exec("ALTER TABLE games ADD COLUMN position_history TEXT DEFAULT '[]'")
	if err != nil {
		// Column likely already exists, ignore error
		log.Printf("Migration note: %v", err)
	}

	// Add clock columns for timed games
	clockMigrations := []string{
		"ALTER TABLE games ADD COLUMN time_control_ms INTEGER DEFAULT NULL",
		"ALTER TABLE games ADD COLUMN increment_ms INTEGER DEFAULT 0",
		"ALTER TABLE games ADD COLUMN white_time_remaining INTEGER DEFAULT NULL",
		"ALTER TABLE games ADD COLUMN black_time_remaining INTEGER DEFAULT NULL",
		"ALTER TABLE games ADD COLUMN last_move_at DATETIME DEFAULT NULL",
	}

	for _, migration := range clockMigrations {
		_, err := DB.Exec(migration)
		if err != nil {
			log.Printf("Migration note: %v", err)
		}
	}

	// Add delivered flag for store-and-forward messages
	_, err = DB.Exec("ALTER TABLE messages ADD COLUMN delivered BOOLEAN DEFAULT FALSE")
	if err != nil {
		log.Printf("Migration note: %v", err)
	}

	// Add read_at timestamp for read receipts
	_, err = DB.Exec("ALTER TABLE messages ADD COLUMN read_at DATETIME DEFAULT NULL")
	if err != nil {
		log.Printf("Migration note: %v", err)
	}

	return nil
}
