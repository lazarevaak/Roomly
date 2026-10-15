package main

import (
	"log/slog"
	"os"
)

func setupLogging() {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
			if len(groups) == 0 {
				switch attr.Key {
				case slog.TimeKey:
					attr.Key = "timestamp"
				case slog.MessageKey:
					attr.Key = "message"
				}
			}

			return attr
		},
	})

	logger := slog.New(handler).With(
		"service.name", "meeting-rooms",
	)

	slog.SetDefault(logger)
}
