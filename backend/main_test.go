package main

import (
	"database/sql"
	"net/http"
	"testing"
)

func TestRoutesDoNotConflict(t *testing.T) {
	mux := newRouter(&sql.DB{})

	request, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	handler, pattern := mux.Handler(request)
	if handler == nil {
		t.Fatal("expected handler for root path")
	}
	if pattern == "" {
		t.Fatal("expected matched pattern for root path")
	}
}
