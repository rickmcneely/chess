package main

import (
	"chess-server/database"
	"chess-server/handlers"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gorilla/mux"
)

var baseDir string

func main() {
	exePath, err := os.Executable()
	if err != nil {
		log.Fatal("Failed to get executable path:", err)
	}
	baseDir = filepath.Dir(exePath)

	dbPath := filepath.Join(baseDir, "chess.db")
	if err := database.Initialize(dbPath); err != nil {
		log.Fatal("Failed to initialize database:", err)
	}

	// Run migrations for existing databases
	database.RunMigrations()

	// Create Computer user for AI games
	computerID, err := database.CreateComputerUser()
	if err != nil {
		log.Printf("Warning: Could not create Computer user: %v", err)
	} else {
		handlers.ComputerUserID = computerID
	}

	handlers.GameHub = handlers.NewHub()
	go handlers.GameHub.Run()

	r := mux.NewRouter()

	staticDir := filepath.Join(baseDir, "static")
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))

	indexPath := filepath.Join(baseDir, "static", "index.html")
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, indexPath)
	})

	r.HandleFunc("/api/login", handlers.HandleLogin).Methods("POST")
	r.HandleFunc("/api/register", handlers.HandleRegister).Methods("POST")
	r.HandleFunc("/api/logout", handlers.HandleLogout).Methods("POST")
	r.HandleFunc("/api/me", handlers.HandleMe).Methods("GET")

	r.HandleFunc("/api/admin/pending", handlers.AdminMiddleware(handlers.HandleGetPendingUsers)).Methods("GET")
	r.HandleFunc("/api/admin/approve", handlers.AdminMiddleware(handlers.HandleApproveUser)).Methods("POST")
	r.HandleFunc("/api/admin/reject", handlers.AdminMiddleware(handlers.HandleRejectUser)).Methods("POST")
	r.HandleFunc("/api/admin/settings", handlers.AdminMiddleware(handlers.HandleGetSettings)).Methods("GET")
	r.HandleFunc("/api/admin/settings", handlers.AdminMiddleware(handlers.HandleUpdateSettings)).Methods("POST")
	r.HandleFunc("/api/admin/users", handlers.AdminMiddleware(handlers.HandleGetAllUsers)).Methods("GET")
	r.HandleFunc("/api/admin/delete", handlers.AdminMiddleware(handlers.HandleDeleteUser)).Methods("POST")

	r.HandleFunc("/api/leaderboard", handlers.HandleLeaderboard).Methods("GET")
	r.HandleFunc("/api/online", handlers.HandleOnlineUsers).Methods("GET")

	r.HandleFunc("/api/game", handlers.HandleGetGame).Methods("GET")
	r.HandleFunc("/api/game/active", handlers.HandleGetActiveGame).Methods("GET")
	r.HandleFunc("/api/game/history", handlers.HandleGetGameHistory).Methods("GET")
	r.HandleFunc("/api/game/messages", handlers.HandleGetGameMessages).Methods("GET")
	r.HandleFunc("/api/messages", handlers.HandleGetDirectMessages).Methods("GET")

	r.HandleFunc("/ws", handlers.HandleWebSocket)

	log.Println("Chess server starting on port 88...")
	log.Println("Default admin credentials: admin / chess2024")
	log.Fatal(http.ListenAndServe(":88", r))
}
