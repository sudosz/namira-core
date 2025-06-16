package api

import (
	"net/http"

	"github.com/NaMiraNet/rayping/internal/core"
	"github.com/NaMiraNet/rayping/internal/logger"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

func NewRouter(c *core.Core, redisClient *redis.Client, callbackHandler CallbackHandler, logger *zap.Logger) *mux.Router {
	r := mux.NewRouter()
	h := NewHandler(c, redisClient, callbackHandler, logger)

	r.Use(corsMiddleware, loggingMiddleware)

	r.HandleFunc("/scan", h.handleScan).Methods(http.MethodPost)
	r.HandleFunc("/job/{id}", h.handleJobStatus).Methods(http.MethodGet)
	r.HandleFunc("/health", h.handleHealth).Methods(http.MethodGet)

	return r
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Info("request received",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.String("remote_addr", r.RemoteAddr),
		)
		next.ServeHTTP(w, r)
	})
}
