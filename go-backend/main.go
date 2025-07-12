package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/redis/go-redis/v9"
	_ "github.com/lib/pq"
)

var (
	db  *sql.DB
	rdb *redis.Client
	ctx = context.Background()
)

func main() {
	pgConnStr := os.Getenv("DATABASE_URL")
	if pgConnStr == "" {
		pgConnStr = "postgres://jonny@localhost:5434/saas_analytics?sslmode=disable"
	}

	var err error
	db, err = sql.Open("postgres", pgConnStr)
	if err != nil {
		log.Fatalf("Failed to connect to Postgres: %v", err)
	}
	err = db.Ping()
	if err != nil {
		log.Fatalf("Failed to ping Postgres: %v", err)
	}
	log.Println("✅ Connected to Postgres")

	rdb = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})

	_, err = rdb.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Println("✅ Connected to Redis")

	http.HandleFunc("/register", handleRegister)
	http.HandleFunc("/track", handleTrack)

	port := "8080"
	log.Printf("🚀 Server running on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		http.Error(w, "Missing user_id", http.StatusBadRequest)
		return
	}
	_, err := db.Exec(`INSERT INTO users (user_id, registered_at) VALUES ($1, NOW())`, userID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to register user: %v", err), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "User %s registered successfully", userID)
}

func handleTrack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	event := r.URL.Query().Get("event")
	userID := r.URL.Query().Get("user_id")
	if event == "" || userID == "" {
		http.Error(w, "Missing event or user_id", http.StatusBadRequest)
		return
	}
	_, err := db.Exec(`INSERT INTO events (user_id, event_type, occurred_at) VALUES ($1, $2, NOW())`, userID, event)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to log event: %v", err), http.StatusInternalServerError)
		return
	}
	_, err = rdb.Incr(ctx, fmt.Sprintf("counter:%s", event)).Result()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to increment Redis counter: %v", err), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "Event %s tracked for user %s", event, userID)
}