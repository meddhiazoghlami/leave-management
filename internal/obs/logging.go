package obs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/meddhiazoghlami/leave-management/internal/config"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"
)

// NewLogger builds the app's structured logger. It always writes JSON to stdout
// (so `docker logs` / the terminal stay useful); when cfg.LokiURL is set it also
// fans each record out to a Loki push handler. Every record is passed through a
// wrapper that stamps the active trace_id, so logs and traces line up in Grafana.
//
// The returned func flushes and stops the Loki shipper; call it on shutdown
// (Wire folds it into the app cleanup — hence the plain func() cleanup shape it
// expects). It's a no-op when Loki is disabled.
func NewLogger(cfg config.Config) (*slog.Logger, func(), error) {
	stdout := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})

	var base slog.Handler = stdout
	cleanup := func() {}

	if cfg.LokiURL != "" {
		shipper := newLokiShipper(cfg.LokiURL, cfg.ServiceName)
		base = fanout{handlers: []slog.Handler{stdout, newLokiHandler(shipper)}}
		cleanup = func() { _ = shipper.Close() }
	}

	return slog.New(traceContext{base}), cleanup, nil
}

// RequestLogger replaces Gin's default text logger with a structured one. The
// route label is the registered pattern (FullPath), not the resolved URL, to
// match the metrics labels and keep logs greppable per endpoint.
func RequestLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = c.Request.URL.Path
		}
		status := c.Writer.Status()

		level := slog.LevelInfo
		switch {
		case status >= 500:
			level = slog.LevelError
		case status >= 400:
			level = slog.LevelWarn
		}

		logger.LogAttrs(c.Request.Context(), level, "http request",
			slog.String("method", c.Request.Method),
			slog.String("route", route),
			slog.Int("status", status),
			slog.Duration("latency", time.Since(start)),
			slog.String("client_ip", c.ClientIP()),
			slog.Int("bytes", c.Writer.Size()),
		)
	}
}

// ─────────────────────────── trace correlation ───────────────────────────

// traceContext wraps a handler and, when the record's context carries an active
// span, adds a trace_id attribute. Applied once at the top so both stdout and
// Loki get the correlation without each handler re-implementing it.
type traceContext struct{ slog.Handler }

func (h traceContext) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.HasTraceID() {
		r = r.Clone()
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()))
	}
	return h.Handler.Handle(ctx, r)
}

func (h traceContext) WithAttrs(as []slog.Attr) slog.Handler {
	return traceContext{h.Handler.WithAttrs(as)}
}
func (h traceContext) WithGroup(name string) slog.Handler {
	return traceContext{h.Handler.WithGroup(name)}
}

// ───────────────────────────── fan-out handler ────────────────────────────

// fanout dispatches each record to every underlying handler (stdout + Loki).
type fanout struct{ handlers []slog.Handler }

func (f fanout) Enabled(ctx context.Context, l slog.Level) bool {
	for _, h := range f.handlers {
		if h.Enabled(ctx, l) {
			return true
		}
	}
	return false
}

func (f fanout) Handle(ctx context.Context, r slog.Record) error {
	var firstErr error
	for _, h := range f.handlers {
		if !h.Enabled(ctx, r.Level) {
			continue
		}
		// Clone per handler: JSONHandler consumes the record's attrs iterator.
		if err := h.Handle(ctx, r.Clone()); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (f fanout) WithAttrs(as []slog.Attr) slog.Handler {
	hs := make([]slog.Handler, len(f.handlers))
	for i, h := range f.handlers {
		hs[i] = h.WithAttrs(as)
	}
	return fanout{handlers: hs}
}

func (f fanout) WithGroup(name string) slog.Handler {
	hs := make([]slog.Handler, len(f.handlers))
	for i, h := range f.handlers {
		hs[i] = h.WithGroup(name)
	}
	return fanout{handlers: hs}
}

// ───────────────────────────── Loki handler ───────────────────────────────

// lokiHandler renders each record to a JSON line (reusing slog's own JSON
// encoder so attrs/groups Just Work) and hands it to the shipper's queue. The
// render buffer is shared across WithAttrs/WithGroup clones and guarded by
// renderMu, since concurrent goroutines may log at once.
type lokiHandler struct {
	shipper  *lokiShipper
	level    slog.Leveler
	renderMu *sync.Mutex
	buf      *bytes.Buffer
	json     slog.Handler
}

func newLokiHandler(s *lokiShipper) *lokiHandler {
	buf := &bytes.Buffer{}
	return &lokiHandler{
		shipper:  s,
		level:    slog.LevelInfo,
		renderMu: &sync.Mutex{},
		buf:      buf,
		json:     slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}),
	}
}

func (h *lokiHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.level.Level()
}

func (h *lokiHandler) Handle(ctx context.Context, r slog.Record) error {
	h.renderMu.Lock()
	h.buf.Reset()
	err := h.json.Handle(ctx, r)
	line := strings.TrimRight(h.buf.String(), "\n")
	h.renderMu.Unlock()
	if err != nil {
		return err
	}
	h.shipper.enqueue(r.Time, r.Level.String(), line)
	return nil
}

func (h *lokiHandler) WithAttrs(as []slog.Attr) slog.Handler {
	return &lokiHandler{shipper: h.shipper, level: h.level, renderMu: h.renderMu, buf: h.buf, json: h.json.WithAttrs(as)}
}

func (h *lokiHandler) WithGroup(name string) slog.Handler {
	return &lokiHandler{shipper: h.shipper, level: h.level, renderMu: h.renderMu, buf: h.buf, json: h.json.WithGroup(name)}
}

// ───────────────────────────── Loki shipper ───────────────────────────────

const (
	lokiFlushInterval = 2 * time.Second
	lokiMaxBatch      = 200
)

type lokiEntry struct {
	ts    time.Time
	level string
	line  string
}

// lokiShipper batches log lines and POSTs them to Loki's push API. A single
// background goroutine flushes on a ticker, on a full batch, or on Close. Push
// failures are logged to stderr and dropped — logging must never take the app
// down.
type lokiShipper struct {
	url    string // full push URL
	app    string
	client *http.Client

	mu      sync.Mutex
	entries []lokiEntry

	flushNow chan struct{}
	quit     chan struct{}
	wg       sync.WaitGroup
}

func newLokiShipper(baseURL, app string) *lokiShipper {
	s := &lokiShipper{
		url:      strings.TrimRight(baseURL, "/") + "/loki/api/v1/push",
		app:      app,
		client:   &http.Client{Timeout: 5 * time.Second},
		flushNow: make(chan struct{}, 1),
		quit:     make(chan struct{}),
	}
	s.wg.Add(1)
	go s.run()
	return s
}

func (s *lokiShipper) enqueue(ts time.Time, level, line string) {
	s.mu.Lock()
	s.entries = append(s.entries, lokiEntry{ts: ts, level: level, line: line})
	n := len(s.entries)
	s.mu.Unlock()
	if n >= lokiMaxBatch {
		select {
		case s.flushNow <- struct{}{}:
		default: // a flush is already pending
		}
	}
}

func (s *lokiShipper) run() {
	defer s.wg.Done()
	t := time.NewTicker(lokiFlushInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			s.flush()
		case <-s.flushNow:
			s.flush()
		case <-s.quit:
			s.flush()
			return
		}
	}
}

// Close stops the shipper and flushes whatever is buffered.
func (s *lokiShipper) Close() error {
	close(s.quit)
	s.wg.Wait()
	return nil
}

// Loki push payload: one stream per level, values are [ns-timestamp, line] pairs.
type lokiPush struct {
	Streams []lokiStream `json:"streams"`
}
type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][2]string       `json:"values"`
}

func (s *lokiShipper) flush() {
	s.mu.Lock()
	if len(s.entries) == 0 {
		s.mu.Unlock()
		return
	}
	batch := s.entries
	s.entries = nil
	s.mu.Unlock()

	byLevel := map[string][][2]string{}
	for _, e := range batch {
		ts := strconv.FormatInt(e.ts.UnixNano(), 10)
		byLevel[e.level] = append(byLevel[e.level], [2]string{ts, e.line})
	}

	streams := make([]lokiStream, 0, len(byLevel))
	for lvl, vals := range byLevel {
		streams = append(streams, lokiStream{
			Stream: map[string]string{"app": s.app, "level": strings.ToLower(lvl)},
			Values: vals,
		})
	}

	body, err := json.Marshal(lokiPush{Streams: streams})
	if err != nil {
		fmt.Fprintf(os.Stderr, "loki: marshal push: %v\n", err)
		return
	}

	req, err := http.NewRequest(http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "loki: build request: %v\n", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "loki: push failed: %v\n", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		fmt.Fprintf(os.Stderr, "loki: push rejected: %s\n", resp.Status)
	}
}
