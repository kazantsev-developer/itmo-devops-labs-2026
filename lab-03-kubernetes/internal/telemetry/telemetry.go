// Package telemetry provides shared logging, metrics, and tracing helpers for shop services
package telemetry

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
	Logger            *slog.Logger
	Tracer            trace.Tracer
	httpRequestsTotal *prometheus.CounterVec
	httpDuration      *prometheus.HistogramVec
)

func init() {
	Logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))

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

func InitTracer(serviceName string) (*sdktrace.TracerProvider, error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		Logger.Warn("OTEL_EXPORTER_OTLP_ENDPOINT не задан, трейсы отключены")
		Tracer = trace.NewNoopTracerProvider().Tracer("noop")
		return nil, nil
	}

	ctx := context.Background()
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithInsecure())
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName),
		)),
	)

	otel.SetTracerProvider(tp)
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		Logger.Error("OTel error", "error", err)
	}))

	Tracer = tp.Tracer("api-tracer")
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

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()

		ctx, span := Tracer.Start(r.Context(), fmt.Sprintf("HTTP %s %s", r.Method, r.URL.Path))
		defer span.End()

		traceID := span.SpanContext().TraceID().String()
		reqLogger := Logger.With(slog.String("trace_id", traceID))
		reqLogger.Info("Входящий запрос", "method", r.Method, "path", r.URL.Path)

		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rw, r.WithContext(ctx))

		duration := time.Since(start).Seconds()
		statusStr := fmt.Sprintf("%d", rw.statusCode)

		httpRequestsTotal.WithLabelValues(r.URL.Path, statusStr).Inc()
		httpDuration.WithLabelValues(r.URL.Path).Observe(duration)

		reqLogger.Info("Запрос обработан", "status", rw.statusCode, "duration_sec", duration)
	})
}