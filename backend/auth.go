package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
)

const sessionCookieName = "roomly_session"

type contextKey string

const userContextKey contextKey = "user"

type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

func currentUser(r *http.Request) (User, bool) {
	user, ok := r.Context().Value(userContextKey).(User)
	return user, ok
}

func isAdmin(user User) bool {
	return user.Role == "admin"
}

func authMiddleware(db *sql.DB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil || cookie.Value == "" {
			http.Error(w, "Требуется вход в систему", http.StatusUnauthorized)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		var user User
		err = db.QueryRowContext(
			ctx,
			`SELECT u.id, u.username, u.role
			 FROM sessions s
			 JOIN users u ON u.id = s.user_id
			 WHERE s.token = $1 AND s.expires_at > now()`,
			cookie.Value,
		).Scan(&user.ID, &user.Username, &user.Role)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				log.Printf("Ошибка проверки сессии: %v", err)
			}

			clearSessionCookie(w)
			http.Error(w, "Требуется вход в систему", http.StatusUnauthorized)
			return
		}

		requestCtx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(requestCtx))
	})
}

func loginHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}

		if err := decodeJSON(w, r, &input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		input.Username = strings.TrimSpace(input.Username)
		if input.Username == "" || input.Password == "" {
			http.Error(w, "Логин и пароль обязательны", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		var user User
		var passwordHash string
		err := db.QueryRowContext(
			ctx,
			"SELECT id, username, role, password_hash FROM users WHERE username = $1",
			input.Username,
		).Scan(&user.ID, &user.Username, &user.Role, &passwordHash)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "Неверный логин или пароль", http.StatusUnauthorized)
				return
			}

			log.Printf("Ошибка поиска пользователя: %v", err)
			http.Error(w, "Не удалось выполнить вход", http.StatusInternalServerError)
			return
		}

		if !passwordMatches(passwordHash, input.Password) {
			http.Error(w, "Неверный логин или пароль", http.StatusUnauthorized)
			return
		}

		if err := createSession(ctx, db, w, user.ID); err != nil {
			log.Printf("Ошибка создания сессии: %v", err)
			http.Error(w, "Не удалось создать сессию", http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, user)
	}
}

func registerHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}

		if err := decodeJSON(w, r, &input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		input.Username = strings.TrimSpace(input.Username)
		if err := validateCredentials(input.Username, input.Password); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		passwordHash, err := hashPassword(input.Password)
		if err != nil {
			log.Printf("Ошибка хэширования пароля: %v", err)
			http.Error(w, "Не удалось зарегистрировать пользователя", http.StatusInternalServerError)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		var user User
		err = db.QueryRowContext(
			ctx,
			`INSERT INTO users (username, password_hash, role)
			 VALUES ($1, $2, 'user')
			 RETURNING id, username, role`,
			input.Username,
			passwordHash,
		).Scan(&user.ID, &user.Username, &user.Role)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				http.Error(w, "Пользователь с таким логином уже существует", http.StatusConflict)
				return
			}

			log.Printf("Ошибка регистрации пользователя: %v", err)
			http.Error(w, "Не удалось зарегистрировать пользователя", http.StatusInternalServerError)
			return
		}

		if err := createSession(ctx, db, w, user.ID); err != nil {
			log.Printf("Ошибка создания сессии после регистрации: %v", err)
			http.Error(w, "Пользователь создан, но вход не выполнен", http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusCreated, user)
	}
}

func changePasswordHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := currentUser(r)
		if !ok {
			http.Error(w, "Требуется вход в систему", http.StatusUnauthorized)
			return
		}

		var input struct {
			CurrentPassword string `json:"currentPassword"`
			NewPassword     string `json:"newPassword"`
		}

		if err := decodeJSON(w, r, &input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if input.CurrentPassword == "" {
			http.Error(w, "Текущий пароль обязателен", http.StatusBadRequest)
			return
		}

		if err := validatePassword(input.NewPassword); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		var currentHash string
		err := db.QueryRowContext(
			ctx,
			"SELECT password_hash FROM users WHERE id = $1",
			user.ID,
		).Scan(&currentHash)
		if err != nil {
			log.Printf("Ошибка поиска пользователя для смены пароля: %v", err)
			http.Error(w, "Не удалось сменить пароль", http.StatusInternalServerError)
			return
		}

		if !passwordMatches(currentHash, input.CurrentPassword) {
			http.Error(w, "Текущий пароль указан неверно", http.StatusUnauthorized)
			return
		}

		newHash, err := hashPassword(input.NewPassword)
		if err != nil {
			log.Printf("Ошибка хэширования нового пароля: %v", err)
			http.Error(w, "Не удалось сменить пароль", http.StatusInternalServerError)
			return
		}

		_, err = db.ExecContext(
			ctx,
			"UPDATE users SET password_hash = $1 WHERE id = $2",
			newHash,
			user.ID,
		)
		if err != nil {
			log.Printf("Ошибка обновления пароля: %v", err)
			http.Error(w, "Не удалось сменить пароль", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func logoutHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie(sessionCookieName); err == nil {
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()

			if _, err := db.ExecContext(ctx, "DELETE FROM sessions WHERE token = $1", cookie.Value); err != nil {
				log.Printf("Ошибка удаления сессии: %v", err)
			}
		}

		clearSessionCookie(w)
		w.WriteHeader(http.StatusNoContent)
	}
}

func meHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		http.Error(w, "Требуется вход в систему", http.StatusUnauthorized)
		return
	}

	writeJSON(w, http.StatusOK, user)
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("некорректный JSON")
	}

	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("ожидается один JSON-объект")
	}

	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("Ошибка отправки JSON: %v", err)
	}
}

func randomToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func createSession(ctx context.Context, db *sql.DB, w http.ResponseWriter, userID int64) error {
	token, err := randomToken()
	if err != nil {
		return err
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	_, err = db.ExecContext(
		ctx,
		"INSERT INTO sessions (token, user_id, expires_at) VALUES ($1, $2, $3)",
		token,
		userID,
		expiresAt,
	)
	if err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
	})

	return nil
}

func validateCredentials(username string, password string) error {
	if username == "" || utf8.RuneCountInString(username) > 80 {
		return fmt.Errorf("логин должен содержать от 1 до 80 символов")
	}

	return validatePassword(password)
}

func validatePassword(password string) error {
	length := utf8.RuneCountInString(password)
	if length < 6 || length > 120 {
		return fmt.Errorf("пароль должен содержать от 6 до 120 символов")
	}

	return nil
}

func hashPassword(password string) (string, error) {
	salt, err := randomToken()
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256([]byte(salt + password))
	return "sha256$" + salt + "$" + hex.EncodeToString(sum[:]), nil
}

func passwordMatches(storedHash string, password string) bool {
	parts := strings.Split(storedHash, "$")
	if len(parts) != 3 || parts[0] != "sha256" {
		return false
	}

	sum := sha256.Sum256([]byte(parts[1] + password))
	expected := hex.EncodeToString(sum[:])

	return subtle.ConstantTimeCompare([]byte(expected), []byte(parts[2])) == 1
}
