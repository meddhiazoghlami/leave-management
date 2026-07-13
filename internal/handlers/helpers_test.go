package handlers_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

// form builds url.Values from alternating key, value pairs.
func form(kv ...string) url.Values {
	v := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		v.Set(kv[i], kv[i+1])
	}
	return v
}

// do sends a request (form-encoded if f != nil) through the handler with any
// cookies attached, and returns the recorder.
func do(r http.Handler, method, path string, f url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	var body io.Reader
	if f != nil {
		body = strings.NewReader(f.Encode())
	}
	req := httptest.NewRequest(method, path, body)
	if f != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for _, ck := range cookies {
		if ck != nil {
			req.AddCookie(ck)
		}
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// hasSessionCookie reports whether the response set a non-empty session cookie.
func hasSessionCookie(w *httptest.ResponseRecorder) bool {
	for _, ck := range w.Result().Cookies() {
		if ck.Name == "session" && ck.Value != "" && ck.MaxAge >= 0 {
			return true
		}
	}
	return false
}

// nextWeekday returns the next date on or after `from` whose weekday is `wd`,
// normalised to midnight UTC.
func nextWeekday(from time.Time, wd time.Weekday) time.Time {
	d := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	for d.Weekday() != wd {
		d = d.AddDate(0, 0, 1)
	}
	return d
}

func ymd(t time.Time) string { return t.Format("2006-01-02") }

func mustStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, want, w.Body.String())
	}
}
