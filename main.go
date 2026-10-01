package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("переменная DATABASE_URL не задана")
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("ошибка настройки подключения: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("не удалось подключиться к PostgreSQL: %w", err)
	}

	log.Println("Подключение к PostgreSQL установлено")

	http.HandleFunc("/healthz", healthHandler)
	http.HandleFunc("GET /api/rooms", roomsHandler(db))
	http.HandleFunc("POST /api/rooms", createRoomHandler(db))
	http.HandleFunc("DELETE /api/rooms/{id}", deleteRoomHandler(db))
	http.HandleFunc("PUT /api/rooms/{id}", updateRoomHandler(db))

	log.Println("Сервер запущен: http://localhost:8080")
	return http.ListenAndServe(":8080", nil)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Сервер работает!")
}