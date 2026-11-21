package main

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	passwordHashPrefix     = "pbkdf2_sha256"
	passwordHashIterations = 210000
	passwordSaltLen        = 16
	passwordKeyLen         = 32
	defaultTokenTTL        = 24 * time.Hour
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUserAlreadyExists  = errors.New("username already exists")
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidUser        = errors.New("invalid user data")
	ErrInvalidToken       = errors.New("invalid or expired auth token")

	usernameRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,31}$`)

	ephemeralAuthSecretOnce sync.Once
	ephemeralAuthSecret     string
)

// ActiveAuthSecret returns Config.AuthSecret or a process-local random fallback secret.
func ActiveAuthSecret() string {
	if s := strings.TrimSpace(Config.AuthSecret); s != "" {
		return s
	}
	ephemeralAuthSecretOnce.Do(func() {
		buf := make([]byte, 32)
		_, _ = rand.Read(buf)
		ephemeralAuthSecret = hex.EncodeToString(buf)
	})
	return ephemeralAuthSecret
}

func NormalizeAndValidateUsername(raw string) (string, error) {
	u := strings.ToLower(strings.TrimSpace(raw))
	if !usernameRegex.MatchString(u) {
		return "", fmt.Errorf("%w: username must be 3-32 chars (a-z, 0-9, ., _, -)", ErrInvalidUser)
	}
	return u, nil
}

func ValidatePassword(password string) error {
	if len(password) < 6 {
		return fmt.Errorf("%w: password must be at least 6 characters", ErrInvalidUser)
	}
	if len(password) > 128 {
		return fmt.Errorf("%w: password is too long", ErrInvalidUser)
	}
	return nil
}

func NormalizeRole(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "instructor":
		return "instructor"
	default:
		return "trainee"
	}
}

func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	salt := make([]byte, passwordSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	derived, err := pbkdf2.Key(sha256.New, password, salt, passwordHashIterations, passwordKeyLen)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"%s$%d$%s$%s",
		passwordHashPrefix,
		passwordHashIterations,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(derived),
	), nil
}

func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != passwordHashPrefix {
		return false
	}
	iters, err := strconv.Atoi(parts[1])
	if err != nil || iters < 10000 || iters > 2000000 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil || len(salt) == 0 {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(expected) == 0 {
		return false
	}
	actual, err := pbkdf2.Key(sha256.New, password, salt, iters, len(expected))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

// SignAuthToken produces a standard HS256 JWT using Go stdlib HMAC-SHA256.
func SignAuthToken(claims AuthClaims, secret string, ttl time.Duration) (string, error) {
	if strings.TrimSpace(secret) == "" {
		return "", errors.New("auth secret is empty")
	}
	if ttl <= 0 {
		ttl = defaultTokenTTL
	}
	if claims.Exp == 0 {
		claims.Exp = time.Now().Add(ttl).Unix()
	}
	headerJSON := []byte(`{"alg":"HS256","typ":"JWT"}`)
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signingInput := base64.RawURLEncoding.EncodeToString(headerJSON) + "." +
		base64.RawURLEncoding.EncodeToString(payloadJSON)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signingInput))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + sig, nil
}

func VerifyAuthToken(token string, secret string, now time.Time) (*AuthClaims, error) {
	token = strings.TrimSpace(token)
	parts := strings.Split(token, ".")
	if len(parts) != 3 || strings.TrimSpace(secret) == "" {
		return nil, ErrInvalidToken
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, ErrInvalidToken
	}
	var hdr struct {
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(headerBytes, &hdr); err != nil || hdr.Alg != "HS256" {
		return nil, ErrInvalidToken
	}

	signingInput := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signingInput))
	expectedSig := mac.Sum(nil)

	gotSig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || subtle.ConstantTimeCompare(gotSig, expectedSig) != 1 {
		return nil, ErrInvalidToken
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}
	var claims AuthClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, ErrInvalidToken
	}
	if claims.UserID == "" || claims.Username == "" || claims.Exp <= now.Unix() {
		return nil, ErrInvalidToken
	}
	claims.Role = NormalizeRole(claims.Role)
	return &claims, nil
}

func GeneratePrefixedID(prefix string) string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return fmt.Sprintf("%s_%d_%s", prefix, time.Now().Unix(), hex.EncodeToString(buf))
}
