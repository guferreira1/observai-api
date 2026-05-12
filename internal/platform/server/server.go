package server

import (
	"net/http"
	"time"

	"github.com/guferreira1/observai-api/internal/platform/config"
)

// New creates the HTTP server with production-safe defaults.
func New(cfg config.Config, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}
