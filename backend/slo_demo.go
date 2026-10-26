package main

import (
	"net/http"
	"os"
	"strings"
	"sync/atomic"
)

func withDemoFailures(next http.HandlerFunc) http.HandlerFunc {
	var calls atomic.Uint64

	return func(w http.ResponseWriter, r *http.Request) {
		if os.Getenv("SLO_DEMO_ENABLED") == "true" {
			mode, err := os.ReadFile("/etc/slo-demo/mode")

			if err == nil && strings.TrimSpace(string(mode)) == "errors" {
				if calls.Add(1)%3 == 0 {
					http.Error(
						w,
						"Демонстрационная ошибка сервиса",
						http.StatusServiceUnavailable,
					)
					return
				}
			}
		}

		next(w, r)
	}
}