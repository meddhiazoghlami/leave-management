package obs

import (
	"context"
	"fmt"

	"github.com/meddhiazoghlami/leave-management/internal/config"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// InitTracing builds the OpenTelemetry tracer provider that ships spans to Tempo
// (or any OTLP/HTTP collector) at cfg.OTLPEndpoint. It returns the provider —
// handed to otelgin so the middleware never depends on global init order — and a
// cleanup that flushes buffered spans and shuts the exporter down.
//
// When cfg.OTLPEndpoint is empty (the default for a plain `go run` with no Tempo
// around) it returns a no-op provider and a no-op cleanup, so tracing simply
// does nothing rather than erroring or blocking on a dead collector.
func InitTracing(ctx context.Context, cfg config.Config) (trace.TracerProvider, func(), error) {
	// Always install a propagator so incoming/outgoing W3C traceparent headers
	// are honoured even in the no-op case — cheap and avoids surprises.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	if cfg.OTLPEndpoint == "" {
		tp := noop.NewTracerProvider()
		otel.SetTracerProvider(tp)
		return tp, func() {}, nil
	}

	// WithEndpointURL takes the full base URL (e.g. http://tempo:4318); the
	// exporter appends /v1/traces. WithInsecure allows plain HTTP for a local /
	// in-cluster collector (no TLS on the OTLP port here).
	exp, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(cfg.OTLPEndpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("otlp trace exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(cfg.ServiceName)),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("otel resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()), // learning setup: keep every trace
	)
	otel.SetTracerProvider(tp)

	cleanup := func() {
		// Best-effort flush + shutdown on the way out. Own context so it still
		// runs if the caller's ctx is already cancelled.
		_ = tp.Shutdown(context.Background())
	}
	return tp, cleanup, nil
}
