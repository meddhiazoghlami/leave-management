package obs_test

import (
	"context"
	"testing"

	"github.com/meddhiazoghlami/leave-management/internal/config"
	"github.com/meddhiazoghlami/leave-management/internal/obs"
)

// With no OTLP endpoint configured (a plain `go run` with no Tempo) tracing must
// degrade to a working no-op: a usable provider and a cleanup that's safe to
// call. It must never error or block trying to reach a collector that isn't there.
func TestInitTracingNoEndpointIsNoop(t *testing.T) {
	tp, cleanup, err := obs.InitTracing(context.Background(), config.Config{ServiceName: "test-svc"})
	if err != nil {
		t.Fatalf("InitTracing: %v", err)
	}
	if tp == nil {
		t.Fatal("nil tracer provider")
	}
	if cleanup == nil {
		t.Fatal("nil cleanup")
	}

	// The provider must be usable — spans just go nowhere.
	_, span := tp.Tracer("test").Start(context.Background(), "op")
	span.End()

	cleanup() // safe to call
}
