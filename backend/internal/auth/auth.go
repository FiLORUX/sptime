// Package auth provides authentication and authorization for the SPTime Web API.
// Supports JWT-based authentication with role-based access control.
package auth

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sptime/sptime/internal/config"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidToken       = errors.New("invalid token")
	ErrTokenExpired       = errors.New("token expired")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrForbidden          = errors.New("forbidden")
)

// Role represents a user role.
type Role string

const (
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleViewer   Role = "viewer"
)

// Claims represents JWT claims.
type Claims struct {
	Username string `json:"username"`
	Role     Role   `json:"role"`
	jwt.RegisteredClaims
}

// User represents an authenticated user.
type User struct {
	Username string `json:"username"`
	Role     Role   `json:"role"`
}

// Authenticator handles user authentication.
type Authenticator struct {
	users     map[string]config.UserConfig
	jwtSecret []byte
	jwtExpiry time.Duration
}

// NewAuthenticator creates a new Authenticator.
func NewAuthenticator(cfg config.WebConfig) *Authenticator {
	users := make(map[string]config.UserConfig)
	for _, u := range cfg.Users {
		users[u.Username] = u
	}

	return &Authenticator{
		users:     users,
		jwtSecret: []byte(cfg.JWTSecret),
		jwtExpiry: cfg.JWTExpiry,
	}
}

// Authenticate validates credentials and returns a JWT token.
func (a *Authenticator) Authenticate(username, password string) (string, error) {
	user, exists := a.users[username]
	if !exists {
		// Use constant time comparison to prevent timing attacks
		bcrypt.CompareHashAndPassword([]byte("$2a$10$dummy"), []byte(password))
		return "", ErrInvalidCredentials
	}

	// Compare password hash
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", ErrInvalidCredentials
	}

	// Generate JWT
	claims := &Claims{
		Username: username,
		Role:     Role(user.Role),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(a.jwtExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "sptime",
			Subject:   username,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(a.jwtSecret)
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, nil
}

// ValidateToken validates a JWT token and returns the claims.
func (a *Authenticator) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return a.jwtSecret, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// ExtractToken extracts the JWT token from an HTTP request.
func ExtractToken(r *http.Request) string {
	// Check Authorization header
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}

	// Check cookie
	cookie, err := r.Cookie("token")
	if err == nil {
		return cookie.Value
	}

	// Check query parameter (for WebSocket connections)
	return r.URL.Query().Get("token")
}

// HasPermission checks if a role has permission for an action.
func HasPermission(role Role, action string) bool {
	permissions := map[Role][]string{
		RoleAdmin: {
			"read", "write", "config", "admin",
		},
		RoleOperator: {
			"read", "write",
		},
		RoleViewer: {
			"read",
		},
	}

	perms, exists := permissions[role]
	if !exists {
		return false
	}

	for _, p := range perms {
		if p == action {
			return true
		}
	}

	return false
}

// HashPassword creates a bcrypt hash of a password.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// ComparePasswords securely compares two strings.
func ComparePasswords(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
