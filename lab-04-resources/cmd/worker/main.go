// Package main is the entry point for the shop background worker
package main

import (
	"context"
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

const (
	pollInterval = 2 * time.Second
	jobDuration  = 500 * time.Millisecond
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	healthPort := getenv("HEALTH_PORT", "8081")

	tp, err := telemetry.InitTracer("shop-worker")
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

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.Handle("/metrics", promhttp.Handler())

	srv := &http.Server{
		Addr:              ":" + healthPort,
		Handler:           telemetry.Middleware(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		telemetry.Logger.Info("worker health-сервер запущен", "port", healthPort)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			telemetry.Logger.Error("health-сервер упал", "error", err)
			os.Exit(1)
		}
	}()

	go runLoop(ctx, pool)

	<-ctx.Done()
	telemetry.Logger.Info("получен сигнал остановки, гасим worker")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		telemetry.Logger.Error("ошибка при остановке", "error", err)
	}
}

func runLoop(ctx context.Context, pool *pgxpool.Pool) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			processOne(ctx, pool)
		}
	}
}

func processOne(ctx context.Context, pool *pgxpool.Pool) {
	order, err := db.ClaimNextOrder(ctx, pool)
	if err != nil {
		telemetry.Logger.Error("ошибка при выборке заказа", "error", err)
		return
	}
	if order == nil {
		return
	}

	telemetry.Logger.Info(
		"взят заказ в обработку",
		"id", order.ID,
		"description", order.Description,
	)

	select {
	case <-time.After(jobDuration):
	case <-ctx.Done():
		return
	}

	telemetry.Logger.Info("заказ обработан", "id", order.ID)
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
