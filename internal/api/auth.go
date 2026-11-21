package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cusconf/cusconf/internal/store"
)

const sessionTTL = 7 * 24 * time.Hour

type auth struct {
	password string
	secret   []byte
}

// loadAuth reads the admin password from CUSCONF_ADMIN_PASSWORD, or generates
// one on first start and persists it (printed once to the log).
func loadAuth(st *store.Store) (*auth, error) {
	secret, err := st.Setting("session_secret")
	if err != nil {
		return nil, err
	}
	if secret == "" {
		secret = randHex(32)
		if err := st.SetSetting("session_secret", secret); err != nil {
			return nil, err
		}
	}
	pw := os.Getenv("CUSCONF_ADMIN_PASSWORD")
	if pw == "" {
		pw, err = st.Setting("admin_password")
		if err != nil {
			return nil, err
		}
		if pw == "" {
			pw = randHex(8)
			if err := st.SetSetting("admin_password", pw); err != nil {
				return nil, err
			}
			slog.Warn("generated admin password (set CUSCONF_ADMIN_PASSWORD to override)", "password", pw)
		}
	}
	return &auth{password: pw, secret: []byte(secret)}, nil
}

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (a *auth) check(pw string) bool {
	return subtle.ConstantTimeCompare([]byte(pw), []byte(a.password)) == 1
}

func (a *auth) sign(exp int64) string {
	m := hmac.New(sha256.New, a.secret)
	fmt.Fprintf(m, "admin|%d", exp)
	return strconv.FormatInt(exp, 10) + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func (a *auth) issue() (string, int64) {
	exp := time.Now().Add(sessionTTL).Unix()
	return a.sign(exp), exp
}

func (a *auth) valid(tok string) bool {
	expS, _, ok := strings.Cut(tok, ".")
	if !ok {
		return false
	}
	exp, err := strconv.ParseInt(expS, 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}
	return hmac.Equal([]byte(tok), []byte(a.sign(exp)))
}

func (a *auth) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !a.valid(tok) {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}
