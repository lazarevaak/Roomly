package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
)

type Booking struct {
	ID        int64     `json:"id"`
	RoomID    int64     `json:"roomId"`
	RoomName  string    `json:"roomName"`
	UserID    int64     `json:"userId"`
	Username  string    `json:"username"`
	Purpose   string    `json:"purpose"`
	StartsAt  time.Time `json:"startsAt"`
	EndsAt    time.Time `json:"endsAt"`
	CreatedAt time.Time `json:"createdAt"`
}

type bookingInput struct {
	RoomID   int64     `json:"roomId"`
	Purpose  string    `json:"purpose"`
	StartsAt time.Time `json:"startsAt"`
	EndsAt   time.Time `json:"endsAt"`
}

func bookingsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := currentUser(r)
		if !ok {
			http.Error(w, "Требуется вход в систему", http.StatusUnauthorized)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		query := `SELECT b.id, b.room_id, r.name, b.user_id, u.username,
		          b.purpose, b.starts_at, b.ends_at, b.created_at
		          FROM bookings b
		          JOIN rooms r ON r.id = b.room_id
		          JOIN users u ON u.id = b.user_id`
		args := []any{}

		if !isAdmin(user) {
			query += " WHERE b.user_id = $1"
			args = append(args, user.ID)
		}

		query += " ORDER BY b.starts_at, b.id"

		rows, err := db.QueryContext(ctx, query, args...)
		if err != nil {
			log.Printf("Ошибка запроса бронирований: %v", err)
			http.Error(w, "Не удалось получить бронирования", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		bookings := make([]Booking, 0)
		for rows.Next() {
			booking, err := scanBooking(rows)
			if err != nil {
				log.Printf("Ошибка чтения бронирования: %v", err)
				http.Error(w, "Ошибка чтения данных", http.StatusInternalServerError)
				return
			}

			bookings = append(bookings, booking)
		}

		if err := rows.Err(); err != nil {
			log.Printf("Ошибка получения строк бронирований: %v", err)
			http.Error(w, "Ошибка чтения данных", http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, bookings)
	}
}

func createBookingHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := currentUser(r)
		if !ok {
			http.Error(w, "Требуется вход в систему", http.StatusUnauthorized)
			return
		}

		input, ok := parseBookingInput(w, r)
		if !ok {
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		if hasBookingOverlap(ctx, db, input.RoomID, input.StartsAt, input.EndsAt, 0) {
			http.Error(w, "Комната уже забронирована на это время", http.StatusConflict)
			return
		}

		booking, err := queryBookingRow(
			db.QueryRowContext(
				ctx,
				`INSERT INTO bookings (room_id, user_id, purpose, starts_at, ends_at)
				 VALUES ($1, $2, $3, $4, $5)
				 RETURNING id, room_id, user_id, purpose, starts_at, ends_at, created_at`,
				input.RoomID,
				user.ID,
				input.Purpose,
				input.StartsAt,
				input.EndsAt,
			),
			db,
			ctx,
		)
		if err != nil {
			handleBookingWriteError(w, err, "Не удалось создать бронирование")
			return
		}

		writeJSON(w, http.StatusCreated, booking)
	}
}

func updateBookingHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := currentUser(r)
		if !ok {
			http.Error(w, "Требуется вход в систему", http.StatusUnauthorized)
			return
		}

		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			http.Error(w, "Некорректный ID бронирования", http.StatusBadRequest)
			return
		}

		input, ok := parseBookingInput(w, r)
		if !ok {
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		if !canAccessBooking(ctx, db, user, id) {
			http.Error(w, "Можно изменять только свои бронирования", http.StatusForbidden)
			return
		}

		if hasBookingOverlap(ctx, db, input.RoomID, input.StartsAt, input.EndsAt, id) {
			http.Error(w, "Комната уже забронирована на это время", http.StatusConflict)
			return
		}

		booking, err := queryBookingRow(
			db.QueryRowContext(
				ctx,
				`UPDATE bookings
				 SET room_id = $1, purpose = $2, starts_at = $3, ends_at = $4
				 WHERE id = $5
				 RETURNING id, room_id, user_id, purpose, starts_at, ends_at, created_at`,
				input.RoomID,
				input.Purpose,
				input.StartsAt,
				input.EndsAt,
				id,
			),
			db,
			ctx,
		)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "Бронирование не найдено", http.StatusNotFound)
				return
			}

			handleBookingWriteError(w, err, "Не удалось обновить бронирование")
			return
		}

		writeJSON(w, http.StatusOK, booking)
	}
}

func deleteBookingHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := currentUser(r)
		if !ok {
			http.Error(w, "Требуется вход в систему", http.StatusUnauthorized)
			return
		}

		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			http.Error(w, "Некорректный ID бронирования", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		if !canAccessBooking(ctx, db, user, id) {
			http.Error(w, "Можно удалять только свои бронирования", http.StatusForbidden)
			return
		}

		result, err := db.ExecContext(ctx, "DELETE FROM bookings WHERE id = $1", id)
		if err != nil {
			log.Printf("Ошибка удаления бронирования: %v", err)
			http.Error(w, "Не удалось удалить бронирование", http.StatusInternalServerError)
			return
		}

		deleted, err := result.RowsAffected()
		if err != nil {
			log.Printf("Ошибка результата удаления бронирования: %v", err)
			http.Error(w, "Не удалось получить результат удаления", http.StatusInternalServerError)
			return
		}

		if deleted == 0 {
			http.Error(w, "Бронирование не найдено", http.StatusNotFound)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func canAccessBooking(ctx context.Context, db *sql.DB, user User, bookingID int64) bool {
	if isAdmin(user) {
		return true
	}

	var ownerID int64
	err := db.QueryRowContext(
		ctx,
		"SELECT user_id FROM bookings WHERE id = $1",
		bookingID,
	).Scan(&ownerID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("Ошибка проверки владельца бронирования: %v", err)
		}
		return false
	}

	return ownerID == user.ID
}

func parseBookingInput(w http.ResponseWriter, r *http.Request) (bookingInput, bool) {
	var input bookingInput
	if err := decodeJSON(w, r, &input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return input, false
	}

	input.Purpose = strings.TrimSpace(input.Purpose)

	if input.RoomID <= 0 {
		http.Error(w, "Выберите переговорную комнату", http.StatusBadRequest)
		return input, false
	}

	if input.Purpose == "" || utf8.RuneCountInString(input.Purpose) > 200 {
		http.Error(w, "Цель должна содержать от 1 до 200 символов", http.StatusBadRequest)
		return input, false
	}

	if input.StartsAt.IsZero() || input.EndsAt.IsZero() || !input.StartsAt.Before(input.EndsAt) {
		http.Error(w, "Время начала должно быть раньше времени окончания", http.StatusBadRequest)
		return input, false
	}

	return input, true
}

func hasBookingOverlap(ctx context.Context, db *sql.DB, roomID int64, startsAt time.Time, endsAt time.Time, excludeID int64) bool {
	var exists bool
	err := db.QueryRowContext(
		ctx,
		`SELECT EXISTS (
			SELECT 1
			FROM bookings
			WHERE room_id = $1
			  AND id <> $4
			  AND starts_at < $3
			  AND ends_at > $2
		)`,
		roomID,
		startsAt,
		endsAt,
		excludeID,
	).Scan(&exists)
	if err != nil {
		log.Printf("Ошибка проверки пересечения бронирований: %v", err)
		return true
	}

	return exists
}

func queryBookingRow(row *sql.Row, db *sql.DB, ctx context.Context) (Booking, error) {
	var booking Booking
	if err := row.Scan(
		&booking.ID,
		&booking.RoomID,
		&booking.UserID,
		&booking.Purpose,
		&booking.StartsAt,
		&booking.EndsAt,
		&booking.CreatedAt,
	); err != nil {
		return booking, err
	}

	err := db.QueryRowContext(
		ctx,
		`SELECT r.name, u.username
		 FROM rooms r, users u
		 WHERE r.id = $1 AND u.id = $2`,
		booking.RoomID,
		booking.UserID,
	).Scan(&booking.RoomName, &booking.Username)

	return booking, err
}

func scanBooking(rows *sql.Rows) (Booking, error) {
	var booking Booking
	err := rows.Scan(
		&booking.ID,
		&booking.RoomID,
		&booking.RoomName,
		&booking.UserID,
		&booking.Username,
		&booking.Purpose,
		&booking.StartsAt,
		&booking.EndsAt,
		&booking.CreatedAt,
	)

	return booking, err
}

func handleBookingWriteError(w http.ResponseWriter, err error, message string) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		http.Error(w, "Указанная комната не найдена", http.StatusBadRequest)
		return
	}

	log.Printf("%s: %v", message, err)
	http.Error(w, message, http.StatusInternalServerError)
}

func (input *bookingInput) UnmarshalJSON(data []byte) error {
	var raw struct {
		RoomID   int64  `json:"roomId"`
		Purpose  string `json:"purpose"`
		StartsAt string `json:"startsAt"`
		EndsAt   string `json:"endsAt"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	startsAt, err := time.Parse(time.RFC3339, raw.StartsAt)
	if err != nil {
		return err
	}

	endsAt, err := time.Parse(time.RFC3339, raw.EndsAt)
	if err != nil {
		return err
	}

	input.RoomID = raw.RoomID
	input.Purpose = raw.Purpose
	input.StartsAt = startsAt
	input.EndsAt = endsAt

	return nil
}
