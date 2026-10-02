package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
)

type Room struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Capacity int    `json:"capacity"`
	Location string `json:"location"`
}

func roomsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		rows, err := db.QueryContext(
			ctx,
			"SELECT id, name, capacity, location FROM rooms ORDER BY id",
		)
		if err != nil {
			log.Printf("Ошибка запроса комнат: %v", err)
			http.Error(w, "Не удалось получить комнаты", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		rooms := make([]Room, 0)

		for rows.Next() {
			var room Room

			err := rows.Scan(
				&room.ID,
				&room.Name,
				&room.Capacity,
				&room.Location,
			)
			if err != nil {
				log.Printf("Ошибка чтения комнаты: %v", err)
				http.Error(w, "Ошибка чтения данных", http.StatusInternalServerError)
				return
			}

			rooms = append(rooms, room)
		}

		if err := rows.Err(); err != nil {
			log.Printf("Ошибка получения строк: %v", err)
			http.Error(w, "Ошибка чтения данных", http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, rooms)
	}
}

func createRoomHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := currentUser(r)
		if !ok || !isAdmin(user) {
			http.Error(w, "Управлять комнатами может только администратор", http.StatusForbidden)
			return
		}

		var input struct {
			Name     string `json:"name"`
			Capacity int    `json:"capacity"`
			Location string `json:"location"`
		}

		if err := decodeJSON(w, r, &input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		input.Name = strings.TrimSpace(input.Name)
		input.Location = strings.TrimSpace(input.Location)

		if input.Name == "" || utf8.RuneCountInString(input.Name) > 100 {
			http.Error(w, "Название должно содержать от 1 до 100 символов", http.StatusBadRequest)
			return
		}

		if input.Capacity <= 0 || input.Capacity > 2147483647 {
			http.Error(w, "Вместимость должна быть от 1 до 2147483647", http.StatusBadRequest)
			return
		}

		if input.Location == "" || utf8.RuneCountInString(input.Location) > 200 {
			http.Error(w, "Расположение должно содержать от 1 до 200 символов", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		var room Room

		err := db.QueryRowContext(
			ctx,
			`INSERT INTO rooms (name, capacity, location)
			 VALUES ($1, $2, $3)
			 RETURNING id, name, capacity, location`,
			input.Name,
			input.Capacity,
			input.Location,
		).Scan(
			&room.ID,
			&room.Name,
			&room.Capacity,
			&room.Location,
		)

		if err != nil {
			var pgErr *pgconn.PgError

			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				http.Error(w, "Комната с таким названием уже существует", http.StatusConflict)
				return
			}

			log.Printf("Ошибка создания комнаты: %v", err)
			http.Error(w, "Не удалось создать комнату", http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusCreated, room)
	}
}

func deleteRoomHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := currentUser(r)
		if !ok || !isAdmin(user) {
			http.Error(w, "Управлять комнатами может только администратор", http.StatusForbidden)
			return
		}

		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			http.Error(w, "Некорректный ID комнаты", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		result, err := db.ExecContext(
			ctx,
			"DELETE FROM rooms WHERE id = $1",
			id,
		)
		if err != nil {
			log.Printf("Ошибка удаления комнаты: %v", err)
			http.Error(w, "Не удалось удалить комнату", http.StatusInternalServerError)
			return
		}

		deleted, err := result.RowsAffected()
		if err != nil {
			log.Printf("Ошибка получения результата удаления: %v", err)
			http.Error(w, "Не удалось получить результат удаления", http.StatusInternalServerError)
			return
		}

		if deleted == 0 {
			http.Error(w, "Комната не найдена", http.StatusNotFound)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func updateRoomHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := currentUser(r)
		if !ok || !isAdmin(user) {
			http.Error(w, "Управлять комнатами может только администратор", http.StatusForbidden)
			return
		}

		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			http.Error(w, "Некорректный ID комнаты", http.StatusBadRequest)
			return
		}

		var input struct {
			Name     string `json:"name"`
			Capacity int    `json:"capacity"`
			Location string `json:"location"`
		}

		if err := decodeJSON(w, r, &input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		input.Name = strings.TrimSpace(input.Name)
		input.Location = strings.TrimSpace(input.Location)

		if input.Name == "" || utf8.RuneCountInString(input.Name) > 100 {
			http.Error(w, "Название должно содержать от 1 до 100 символов", http.StatusBadRequest)
			return
		}

		if input.Capacity <= 0 || input.Capacity > 2147483647 {
			http.Error(w, "Вместимость должна быть от 1 до 2147483647", http.StatusBadRequest)
			return
		}

		if input.Location == "" || utf8.RuneCountInString(input.Location) > 200 {
			http.Error(w, "Расположение должно содержать от 1 до 200 символов", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		var room Room

		err = db.QueryRowContext(
			ctx,
			`UPDATE rooms
			 SET name = $1, capacity = $2, location = $3
			 WHERE id = $4
			 RETURNING id, name, capacity, location`,
			input.Name,
			input.Capacity,
			input.Location,
			id,
		).Scan(&room.ID, &room.Name, &room.Capacity, &room.Location)

		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "Комната не найдена", http.StatusNotFound)
				return
			}

			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				http.Error(w, "Комната с таким названием уже существует", http.StatusConflict)
				return
			}

			log.Printf("Ошибка обновления комнаты: %v", err)
			http.Error(w, "Не удалось обновить комнату", http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, room)
	}
}
