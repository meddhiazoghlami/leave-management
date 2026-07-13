package auth_test

import (
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/meddhiazoghlami/leave-management/internal/auth"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

func TestNewToken(t *testing.T) {
	hex64 := regexp.MustCompile(`^[0-9a-f]{64}$`)

	seen := make(map[string]bool)
	for range 100 {
		tok, err := auth.NewToken()
		if err != nil {
			t.Fatalf("NewToken: %v", err)
		}
		if !hex64.MatchString(tok) {
			t.Fatalf("token %q is not 64 hex chars (256-bit)", tok)
		}
		if seen[tok] {
			t.Fatalf("duplicate token generated: %q", tok)
		}
		seen[tok] = true
	}
}

func findCookie(t *testing.T, w *httptest.ResponseRecorder, name string) (value string, maxAge int, httpOnly bool) {
	t.Helper()
	for _, ck := range w.Result().Cookies() {
		if ck.Name == name {
			return ck.Value, ck.MaxAge, ck.HttpOnly
		}
	}
	t.Fatalf("cookie %q not found in response", name)
	return "", 0, false
}

func TestSetSessionCookie(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	auth.SetSessionCookie(c, "the-token", time.Hour)

	val, maxAge, httpOnly := findCookie(t, w, auth.CookieName)
	if val != "the-token" {
		t.Errorf("cookie value = %q, want the-token", val)
	}
	if maxAge <= 0 {
		t.Errorf("Max-Age = %d, want positive (~3600)", maxAge)
	}
	if !httpOnly {
		t.Error("session cookie should be HttpOnly")
	}
}

func TestClearSessionCookie(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	auth.ClearSessionCookie(c)

	val, maxAge, httpOnly := findCookie(t, w, auth.CookieName)
	if val != "" {
		t.Errorf("cleared cookie value = %q, want empty", val)
	}
	if maxAge >= 0 {
		t.Errorf("cleared cookie Max-Age = %d, want negative (expired)", maxAge)
	}
	if !httpOnly {
		t.Error("cleared cookie should still be HttpOnly")
	}
}
