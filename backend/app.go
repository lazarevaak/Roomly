package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"
)

type App struct {
	db *sql.DB
}

func newApp(db *sql.DB) App {
	return App{db: db}
}

func newRouter(db *sql.DB) *http.ServeMux {
	return newApp(db).routes()
}

func (app App) routes() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", healthHandler)
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if err := app.db.PingContext(ctx); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintln(w, "ready")
	})
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"version": appVersion(), "pod": podName()})
	})
	mux.HandleFunc("POST /api/login", loginHandler(app.db))
	mux.HandleFunc("POST /api/register", registerHandler(app.db))

	protected := http.NewServeMux()
	protected.HandleFunc("POST /api/logout", logoutHandler(app.db))
	protected.HandleFunc("GET /api/me", meHandler)
	protected.HandleFunc("PUT /api/password", changePasswordHandler(app.db))
	protected.HandleFunc("GET /api/rooms", roomsHandler(app.db))

	protected.HandleFunc(
		"POST /api/rooms",
		observeOperation("room_create", createRoomHandler(app.db)),
	)

	protected.HandleFunc("PUT /api/rooms/{id}", updateRoomHandler(app.db))
	protected.HandleFunc("DELETE /api/rooms/{id}", deleteRoomHandler(app.db))
	protected.HandleFunc("GET /api/bookings", bookingsHandler(app.db))

	protected.HandleFunc(
		"POST /api/bookings",
		observeOperation("booking_create", createBookingHandler(app.db)),
	)

	protected.HandleFunc("PUT /api/bookings/{id}", updateBookingHandler(app.db))

	protected.HandleFunc(
		"DELETE /api/bookings/{id}",
		observeOperation("booking_cancel", deleteBookingHandler(app.db)),
	)

	mux.Handle("/api/", authMiddleware(app.db, protected))
	mux.Handle("/", http.FileServer(http.Dir("./web")))

	return mux
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintln(w, "Сервер работает!")
}
