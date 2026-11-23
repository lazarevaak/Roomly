package main

import (
	"log/slog"
	"net/http"
	"os"
)

// The build embeds a release identity; APP_VERSION can override it for local runs.
var buildVersion = "dev"

func appVersion() string {
	if value := os.Getenv("APP_VERSION"); value != "" {
		return value
	}
	return buildVersion
}

func podName() string {
	name, _ := os.Hostname()
	return name
}

func deploymentMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-App-Version", appVersion())
		w.Header().Set("X-Pod-Name", podName())
		if r.URL.Path != "/healthz" && r.URL.Path != "/readyz" {
			slog.Info("http_request", "version", appVersion(), "pod", podName(),
				"method", r.Method, "path", r.URL.Path,
				"request_id", r.Header.Get("X-Request-ID"), "audience", r.Header.Get("X-Audience"))
		}
		next.ServeHTTP(w, r)
	})
}
