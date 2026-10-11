// Package main - ночная аналитика: жрёт CPU и память.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"shop/internal/telemetry"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var memoryHog [][]byte

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	port := getenv("PORT", "8082")
	cpuWorkers := getIntEnv("CPU_LOAD", 1)
	memChunkMB := getIntEnv("MEM_LOAD_MB", 32)
	memInterval := getDurationEnv("MEM_INTERVAL", 5*time.Second)

	tp, err := telemetry.InitTracer("shop-batch")
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

	for i := 0; i < cpuWorkers; i++ {
		go cpuBurn()
	}
	go memBurn(memChunkMB, memInterval)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.Handle("/metrics", promhttp.Handler())

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           telemetry.Middleware(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		telemetry.Logger.Info(
			"batch запущен",
			"port", port,
			"cpu_workers", cpuWorkers,
			"mem_chunk_mb", memChunkMB,
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			telemetry.Logger.Error("сервер упал", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	telemetry.Logger.Info("получен сигнал остановки, гасим batch")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

func cpuBurn() {
	for {
		x := 0.0
		for i := 0; i < 1_000_000; i++ {
			x += float64(i) * 1.000001
		}
		_ = x
	}
}

func memBurn(chunkMB int, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		chunk := make([]byte, chunkMB*1024*1024)
		for i := range chunk {
			chunk[i] = byte(i % 256)
		}
		memoryHog = append(memoryHog, chunk)
		telemetry.Logger.Info(
			"batch сожрал памяти",
			"chunk_mb", chunkMB,
			"chunks_total", len(memoryHog),
			"heap_mb", currentHeapMB(),
		)
	}
}

func currentHeapMB() int {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return int(m.HeapAlloc / 1024 / 1024)
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getIntEnv(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
		return n
	}
	return fallback
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	return fallback
}
