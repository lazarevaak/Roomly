package main

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDeploymentIdentityOnResponses(t *testing.T) {
	t.Setenv("APP_VERSION", "v2")
	handler := deploymentMiddleware(newRouter(&sql.DB{}))
	for _, path := range []string{"/version", "/healthz", "/api/me"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Header().Get("X-App-Version") != "v2" || recorder.Header().Get("X-Pod-Name") == "" {
			t.Fatalf("%s missing release identity: %v", path, recorder.Header())
		}
	}
}

func TestReadinessFailsWhenDatabaseClosedButLivenessSurvives(t *testing.T) {
	db, err := sql.Open("pgx", "postgres://localhost/unused")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	handler := newRouter(db)
	for path, expected := range map[string]int{"/readyz": http.StatusServiceUnavailable, "/healthz": http.StatusOK} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != expected {
			t.Fatalf("%s: got %d, want %d", path, recorder.Code, expected)
		}
	}
}
