package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	// Логи идут в стандартный вывод.
	log.SetOutput(os.Stdout)

	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("переменная DATABASE_URL не задана")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("ошибка настройки подключения: %w", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	// Проверяем подключение к базе при запуске.
	pingCtx, pingCancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	err = db.PingContext(pingCtx)
	pingCancel()

	if err != nil {
		return fmt.Errorf("не удалось подключиться к PostgreSQL: %w", err)
	}

	log.Println("Подключение к PostgreSQL установлено")

	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", healthHandler)
	mux.HandleFunc("GET /api/rooms", roomsHandler(db))
	mux.HandleFunc("POST /api/rooms", createRoomHandler(db))
	mux.HandleFunc("PUT /api/rooms/{id}", updateRoomHandler(db))
	mux.HandleFunc("DELETE /api/rooms/{id}", deleteRoomHandler(db))

	mux.Handle("GET /", http.FileServer(http.Dir("./web")))

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Ctrl+C отправляет SIGINT, Docker при остановке — SIGTERM.
	stopCtx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	serverErrors := make(chan error, 1)

	go func() {
		log.Printf("Сервер запускается на порту %s", port)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("ошибка HTTP-сервера: %w", err)
		}
		return nil

	case <-stopCtx.Done():
		log.Println("Завершение работы сервера...")
	}

	// Даём текущим запросам до 10 секунд на завершение.
	shutdownCtx, shutdownCancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return fmt.Errorf("ошибка завершения сервера: %w", err)
	}

	log.Println("Сервер остановлен")
	return nil
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintln(w, "Сервер работает!")
}