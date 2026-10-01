// Package main is the entry point for the shop API HTTP service
package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"shop/internal/db"
	"shop/internal/telemetry"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	port := getenv("PORT", "8080")

	tp, err := telemetry.InitTracer("api-service")
	if err != nil {
		telemetry.Logger.Error("не удалось запустить трейсер", "error", err)
		os.Exit(1)
	}
	if tp != nil {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = tp.Shutdown(shutdownCtx)
		}()
	}

	pool, err := db.Connect(ctx)
	if err != nil {
		telemetry.Logger.Error("не удалось подключиться к postgres", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := db.InitSchema(ctx, pool); err != nil {
		telemetry.Logger.Error("не удалось создать схему", "error", err)
		os.Exit(1)
	}
	telemetry.Logger.Info("схема БД готова")

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/order", orderHandler(pool))
	mux.HandleFunc("/orders", ordersHandler(pool))
	mux.Handle("/metrics", promhttp.Handler())

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           telemetry.Middleware(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		telemetry.Logger.Info("api запущен", "port", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			telemetry.Logger.Error("сервер упал", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	telemetry.Logger.Info("получен сигнал остановки, гасим сервер")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		telemetry.Logger.Error("ошибка при остановке", "error", err)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("HEALTH_FAIL") == "true" {
		http.Error(w, "unhealthy", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func orderHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Description string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.Description == "" {
			http.Error(w, "description is required", http.StatusBadRequest)
			return
		}

		id, err := db.CreateOrder(r.Context(), pool, req.Description)
		if err != nil {
			telemetry.Logger.Error("не удалось создать заказ", "error", err)
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]int64{"id": id})
	}
}

func ordersHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		orders, err := db.ListOrders(r.Context(), pool)
		if err != nil {
			telemetry.Logger.Error("не удалось прочитать заказы", "error", err)
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		if orders == nil {
			orders = []db.Order{}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(orders)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
