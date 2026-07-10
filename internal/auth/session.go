package auth

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// CookieName is the session cookie key.
const CookieName = "session"

// NewToken returns a 256-bit random, URL-safe session token. crypto/rand (not
// math/rand) so tokens are unguessable.
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// SetSessionCookie writes the session token as an HttpOnly, Lax-SameSite cookie.
// Secure is false so it works over plain http in local dev; a production build
// behind TLS would flip that to true.
func SetSessionCookie(c *gin.Context, token string, ttl time.Duration) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(CookieName, token, int(ttl.Seconds()), "/", "", false, true)
}

// ClearSessionCookie expires the cookie on the client (used on logout / invalid
// session).
func ClearSessionCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(CookieName, "", -1, "/", "", false, true)
}
