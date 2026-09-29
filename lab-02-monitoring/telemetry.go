package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

var (
	logger            *slog.Logger
	tracer            trace.Tracer
	httpRequestsTotal *prometheus.CounterVec
	httpDuration      *prometheus.HistogramVec
)

func init() {
	logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))

	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Общее количество входящих HTTP запросов",
		},
		[]string{"path", "status"},
	)

	httpDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Гистограмма времени ответа сервера в секундах",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		},
		[]string{"path"},
	)

	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpDuration)
}

func initTracer() (*sdktrace.TracerProvider, error) {
	ctx := context.Background()

	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("api-service"),
		)),
	)

	otel.SetTracerProvider(tp)
	tracer = tp.Tracer("api-tracer")
	return tp, nil
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func telemetryMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			next(w, r)
			return
		}

		start := time.Now()

		ctx, span := tracer.Start(r.Context(), fmt.Sprintf("HTTP %s %s", r.Method, r.URL.Path))
		defer span.End()

		traceID := span.SpanContext().TraceID().String()

		reqLogger := logger.With(slog.String("trace_id", traceID))
		reqLogger.Info("Входящий запрос", "method", r.Method, "path", r.URL.Path)

		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next(rw, r.WithContext(ctx))

		duration := time.Since(start).Seconds()
		statusStr := fmt.Sprintf("%d", rw.statusCode)

		httpRequestsTotal.WithLabelValues(r.URL.Path, statusStr).Inc()
		httpDuration.WithLabelValues(r.URL.Path).Observe(duration)

		reqLogger.Info("Запрос успешно обработан", "status", rw.statusCode, "duration_sec", duration)
	}
}
