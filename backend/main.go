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

	if len(os.Args) > 1 {
		if len(os.Args) == 2 && os.Args[1] == "migrate" {
			return migrate(db)
		}

		return fmt.Errorf("неизвестные аргументы: используйте migrate или запуск без аргументов")
	}

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           newRouter(db),
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
