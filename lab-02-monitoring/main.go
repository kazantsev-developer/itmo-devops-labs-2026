package main

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	tp, err := initTracer()
	if err != nil {
		logger.Error("Не удалось запустить трейсер", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			logger.Error("Ошибка при остановке трейсера", "error", err)
		}
	}()

	http.HandleFunc("/health", telemetryMiddleware(healthHandler))
	http.HandleFunc("/fail", telemetryMiddleware(failHandler))
	http.HandleFunc("/slow", telemetryMiddleware(slowHandler))
	http.HandleFunc("/load", telemetryMiddleware(loadHandler))

	http.Handle("/metrics", promhttp.Handler())

	logger.Info("Сервер запущен на порту :" + port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		logger.Error("Сервер упал", "error", err)
		os.Exit(1)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok\n"))
}

func failHandler(w http.ResponseWriter, r *http.Request) {
	span := trace.SpanFromContext(r.Context())
	span.SetStatus(codes.Error, "искусственный сбой")

	traceID := span.SpanContext().TraceID().String()
	logger.With(slog.String("trace_id", traceID)).
		Error("Сгенерирована ошибка 500 по запросу /fail")

	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte("Internal Server Error\n"))
}

func slowHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	_, childSpan := tracer.Start(ctx, "slow-op")
	defer childSpan.End()

	sleepTime := time.Duration(rand.Intn(3)+1) * time.Second
	time.Sleep(sleepTime)

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Slept for %v\n", sleepTime)
}

func loadHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	for id := range 10 {
		go func() {
			_, loadSpan := tracer.Start(ctx, fmt.Sprintf("load-subtask-%d", id))
			defer loadSpan.End()
			time.Sleep(50 * time.Millisecond)
		}()
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("RPS load generated\n"))
}
