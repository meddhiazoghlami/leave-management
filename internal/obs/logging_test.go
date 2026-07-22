package obs_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/meddhiazoghlami/leave-management/internal/config"
	"github.com/meddhiazoghlami/leave-management/internal/obs"
)

type lokiPush struct {
	Streams []struct {
		Stream map[string]string `json:"stream"`
		Values [][2]string       `json:"values"`
	} `json:"streams"`
}

// With LokiURL set, records are batched and POSTed to Loki's push API, labelled
// by app + level, with the log line as the value. cleanup() must flush what's
// buffered.
func TestLoggerPushesToLoki(t *testing.T) {
	var mu sync.Mutex
	var bodies [][]byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/push" {
			t.Errorf("unexpected push path %q", r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, b)
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	logger, cleanup, err := obs.NewLogger(config.Config{LokiURL: srv.URL, ServiceName: "test-svc"})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	logger.Info("hello loki", "answer", 42)
	logger.Error("kaboom")
	cleanup() // stops the shipper and flushes the batch

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) == 0 {
		t.Fatal("Loki received no pushes")
	}

	var (
		sawApp, sawInfoLine, sawErrorStream bool
	)
	for _, b := range bodies {
		var p lokiPush
		if err := json.Unmarshal(b, &p); err != nil {
			t.Fatalf("bad push payload: %v\n%s", err, b)
		}
		for _, s := range p.Streams {
			if s.Stream["app"] == "test-svc" {
				sawApp = true
			}
			if s.Stream["level"] == "error" {
				sawErrorStream = true
			}
			for _, v := range s.Values {
				if strings.Contains(v[1], "hello loki") {
					sawInfoLine = true
				}
			}
		}
	}
	if !sawApp {
		t.Error(`no stream carried the app="test-svc" label`)
	}
	if !sawInfoLine {
		t.Error(`the "hello loki" line never reached Loki`)
	}
	if !sawErrorStream {
		t.Error(`errors were not sent to a level="error" stream`)
	}
}

// Without LokiURL the logger still works (stdout only) and its cleanup is a safe
// no-op.
func TestLoggerStdoutFallback(t *testing.T) {
	logger, cleanup, err := obs.NewLogger(config.Config{})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	logger.Info("no loki here") // must not panic or block
	cleanup()                   // must be safe to call
}
