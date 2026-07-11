package auth

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Authenticator issues and validates session JWTs for the single app user.
type Authenticator struct {
	login    string
	password string
	secret   []byte
}

func New(login, password, secret string) *Authenticator {
	return &Authenticator{login: login, password: password, secret: []byte(secret)}
}

var ErrInvalidCredentials = errors.New("invalid credentials")

// Login verifies credentials (constant-time) and returns a signed JWT.
func (a *Authenticator) Login(login, password string) (string, error) {
	loginOK := subtle.ConstantTimeCompare([]byte(login), []byte(a.login)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(password), []byte(a.password)) == 1
	if !loginOK || !passOK {
		return "", ErrInvalidCredentials
	}
	claims := jwt.RegisteredClaims{
		Subject:   login,
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(a.secret)
}

func (a *Authenticator) parse(tokenStr string) error {
	_, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return a.secret, nil
	})
	return err
}

// Middleware rejects requests without a valid Bearer session token.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		token := strings.TrimPrefix(h, "Bearer ")
		if token == "" || token == h {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if err := a.parse(token); err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
