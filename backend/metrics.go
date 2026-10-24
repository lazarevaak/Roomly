package main

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	roomsCreatedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "rooms_created_total",
		Help: "Total number of successfully created rooms.",
	})

	bookingsCreatedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "bookings_created_total",
		Help: "Total number of successfully created bookings.",
	})

	bookingsUpdatedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "bookings_updated_total",
		Help: "Total number of successfully updated bookings.",
	})

	bookingsCancelledTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "bookings_cancelled_total",
		Help: "Total number of successfully cancelled bookings.",
	})
)

func newMetricsServer() *http.Server {
	mux := http.NewServeMux()

	mux.Handle("/metrics", promhttp.HandlerFor(
		prometheus.DefaultGatherer,
		promhttp.HandlerOpts{
			EnableOpenMetrics: true,
		},
	))

	return &http.Server{
		Addr:              ":9090",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}
