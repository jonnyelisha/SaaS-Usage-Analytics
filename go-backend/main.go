package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

var (
	db  *sql.DB
	rdb *redis.Client
	ctx = context.Background()
)

func main() {
	pgConnStr := os.Getenv("DATABASE_URL")
	if pgConnStr == "" {
		pgConnStr = "postgres://user_name@localhost:port#/saas_analytics?sslmode=disable"
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

	// Public endpoints (no auth required)
	http.HandleFunc("/register", handleRegister)
	http.HandleFunc("/get-api-key", handleGetAPIKey)

	// Protected endpoints (require auth)
	http.Handle("/track", authMiddleware(http.HandlerFunc(handleTrack)))
	http.Handle("/health", authMiddleware(http.HandlerFunc(handleHealth)))
	http.Handle("/users", authMiddleware(http.HandlerFunc(handleGetUsers)))
	http.Handle("/products", authMiddleware(http.HandlerFunc(handleProducts)))
	http.Handle("/purchases", authMiddleware(http.HandlerFunc(handlePurchases)))

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

	// Insert user
	_, err := db.Exec(`INSERT INTO users (user_id, registered_at) VALUES ($1, NOW())`, userID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to register user: %v", err), http.StatusInternalServerError)
		return
	}

	// Generate API key
	apiKey, err := generateAPIKey()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to generate API key: %v", err), http.StatusInternalServerError)
		return
	}

	// Insert API key
	_, err = db.Exec(`INSERT INTO api_keys (user_id, api_key) VALUES ($1, $2)`, userID, apiKey)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to store API key: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(fmt.Sprintf(`{"user_id":"%s", "api_key":"%s"}`, userID, apiKey)))
}

func handleGetAPIKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		http.Error(w, "Missing user_id", http.StatusBadRequest)
		return
	}

	var apiKey string
	err := db.QueryRow(`SELECT api_key FROM api_keys WHERE user_id = $1`, userID).Scan(&apiKey)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}
		http.Error(w, fmt.Sprintf("Failed to query API key: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(fmt.Sprintf(`{"user_id":"%s", "api_key":"%s"}`, userID, apiKey)))
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

func handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := db.Ping(); err != nil {
		http.Error(w, "Postgres unavailable", http.StatusServiceUnavailable)
		return
	}

	if _, err := rdb.Ping(ctx).Result(); err != nil {
		http.Error(w, "Redis unavailable", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

type User struct {
	UserID       string `json:"user_id"`
	RegisteredAt string `json:"registered_at"`
}

func handleGetUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rows, err := db.Query(`SELECT user_id, registered_at FROM users ORDER BY registered_at DESC`)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to query users: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var users []User // Fixed: removed the erroneous "v"
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.UserID, &u.RegisteredAt); err != nil {
			http.Error(w, fmt.Sprintf("Failed to scan user: %v", err), http.StatusInternalServerError)
			return
		}
		users = append(users, u)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("Authorization")
		if apiKey == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		var exists bool
		err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM api_keys WHERE api_key=$1)`, apiKey).Scan(&exists)
		if err != nil || !exists {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func generateAPIKey() (string, error) {
	bytes := make([]byte, 16) // 128-bit key
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
func handlePurchases(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := r.URL.Query().Get("user_id")
	productID := r.URL.Query().Get("product_id")
	quantityStr := r.URL.Query().Get("quantity")
	if userID == "" || productID == "" {
		http.Error(w, "Missing user_id or product_id", http.StatusBadRequest)
		return
	}

	// validate product
	allowedProducts := map[string]bool{
		"apples":  true,
		"oranges": true,
		"bananas": true,
	}

	if !allowedProducts[productID] {
		http.Error(w, "Invalid product_id: must be apples, oranges, or bananas", http.StatusBadRequest)
		return
	}

	quantity := 1
	if quantityStr != "" {
		fmt.Sscanf(quantityStr, "%d", &quantity)
		if quantity <= 0 {
			quantity = 1
		}
	}

	_, err := db.Exec(`INSERT INTO purchases (user_id, product_id, quantity, purchased_at) VALUES ($1, $2, $3, NOW())`, userID, productID, quantity)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to record purchase: %v", err), http.StatusInternalServerError)
		return
	}

	_, err = rdb.IncrBy(ctx, fmt.Sprintf("sales:product:%s", productID), int64(quantity)).Result()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to increment product counter: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "✅ User %s bought %d of product %s", userID, quantity, productID)
}

func handleProducts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Define allowed products
	allowedProducts := []string{"apples", "oranges", "bananas"}

	results := make(map[string]int64)

	for _, product := range allowedProducts {
		count, err := rdb.Get(ctx, fmt.Sprintf("sales:product:%s", product)).Int64()
		if err != nil {
			// If not found in Redis yet, set to 0
			count = 0
		}
		results[product] = count
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}
