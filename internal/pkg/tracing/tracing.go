package tracing

import (
	"context"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// Config holds OpenTelemetry tracing configuration.
type Config struct {
	Enabled    bool
	Endpoint   string  // OTLP HTTP endpoint (e.g., "http://jaeger:4318")
	SampleRate float64 // 0.0–1.0, fraction of traces to sample (1.0 = all)
	Env        string  // deployment environment (e.g., "development", "production")
}

// Provider wraps the OTel TracerProvider and exposes a clean shutdown method.
type Provider struct {
	tp *sdktrace.TracerProvider
}

// Shutdown flushes all pending spans and shuts down the tracer provider.
// Call this in main.go's graceful shutdown path.
func (p *Provider) Shutdown(ctx context.Context) {
	if p == nil || p.tp == nil {
		return
	}
	if err := p.tp.Shutdown(ctx); err != nil {
		slog.Error("failed to shutdown tracer provider", slog.Any("error", err))
	}
}

// InitTracer creates and registers a global OpenTelemetry TracerProvider.
//
// It exports spans via OTLP/HTTP to the configured endpoint (Jaeger 2.x natively
// supports OTLP, so no Jaeger-specific exporter is needed).
//
// Returns a Provider with a Shutdown method that must be called on app exit.
// If tracing is disabled, returns a no-op Provider and nil error.
func InitTracer(ctx context.Context, serviceName string, cfg Config) (*Provider, error) {
	if !cfg.Enabled {
		slog.Info("tracing disabled")
		return &Provider{}, nil
	}

	// Create OTLP/HTTP exporter — Jaeger 2.x accepts OTLP directly.
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(stripScheme(cfg.Endpoint)),
		otlptracehttp.WithInsecure(), // OK for local dev; TLS on GKE is handled by service mesh
	)
	if err != nil {
		return nil, fmt.Errorf("creating OTLP exporter: %w", err)
	}

	// Build resource attributes (who is sending the traces).
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String(serviceName),
		semconv.DeploymentEnvironmentKey.String(cfg.Env),
	)

	// Select sampler based on rate.
	var sampler sdktrace.Sampler
	if cfg.SampleRate >= 1.0 {
		sampler = sdktrace.AlwaysSample()
	} else if cfg.SampleRate <= 0 {
		sampler = sdktrace.NeverSample()
	} else {
		sampler = sdktrace.TraceIDRatioBased(cfg.SampleRate)
	}

	// Create the TracerProvider.
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	// Register as global so otel.Tracer() works anywhere.
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	slog.Info("tracing initialized",
		slog.String("endpoint", cfg.Endpoint),
		slog.Float64("sample_rate", cfg.SampleRate),
	)

	return &Provider{tp: tp}, nil
}

// Tracer returns a named tracer from the global provider.
// Use this to get a tracer for instrumenting specific packages.
func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}

// stripScheme removes http:// or https:// prefix for the OTLP client
// which expects host:port without scheme.
func stripScheme(endpoint string) string {
	for _, prefix := range []string{"https://", "http://"} {
		if len(endpoint) > len(prefix) && endpoint[:len(prefix)] == prefix {
			return endpoint[len(prefix):]
		}
	}
	return endpoint
}
