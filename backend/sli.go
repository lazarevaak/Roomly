package main

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var operationAttempts = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "business_operation_attempts_total",
		Help: "Completed business operations with 2xx or 5xx responses.",
	},
	[]string{"operation"},
)

func observeOperation(
	operation string,
	next http.HandlerFunc,
) http.HandlerFunc {
	counter := operationAttempts.WithLabelValues(operation)

	return func(w http.ResponseWriter, r *http.Request) {
		recorder := &statusWriter{
			ResponseWriter: w,
		}

		next(recorder, r)

		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}

		if (status >= 200 && status < 300) || status >= 500 {
			counter.Inc()
		}
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if code >= 100 && code < 200 {
		w.ResponseWriter.WriteHeader(code)
		return
	}

	if w.status != 0 {
		return
	}

	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}

	return w.ResponseWriter.Write(data)
}

func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
