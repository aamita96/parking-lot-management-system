// Package auth issues and checks JWTs, hashes passwords (argon2id), email codes and gate keys (NFR-7).
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/argon2"
)

const (
	RoleOwner        = "OWNER"
	RoleAdmin        = "ADMIN"
	RoleAdminRefresh = "ADMIN_REFRESH"
)

type Claims struct {
	Role  string `json:"role"`
	Email string `json:"email,omitempty"`
	jwt.RegisteredClaims
}

type Tokens struct {
	secret []byte
}

func NewTokens(secret []byte) *Tokens { return &Tokens{secret: secret} }

// Issue signs a token for subject with role and lifetime.
func (t *Tokens) Issue(subject, role, email string, ttl time.Duration) (string, time.Time, error) {
	exp := time.Now().Add(ttl)
	c := Claims{
		Role:  role,
		Email: email,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			Issuer:    "plms-api",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(exp),
			ID:        randomHex(8),
		},
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(t.secret)
	return s, exp, err
}

// Parse validates the signature, expiry and expected role.
func (t *Tokens) Parse(token, role string) (*Claims, error) {
	c := &Claims{}
	_, err := jwt.ParseWithClaims(token, c, func(tk *jwt.Token) (any, error) {
		return t.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer("plms-api"))
	if err != nil {
		return nil, err
	}
	if c.Role != role {
		return nil, errors.New("wrong role")
	}
	return c, nil
}

// ---- argon2id password hashing (PHC string format) ----

const (
	argonTime    = 2
	argonMemory  = 64 * 1024
	argonThreads = 2
	argonKeyLen  = 32
)

func HashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(pw), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func VerifyPassword(pw, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var m uint32
	var t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(pw), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// ---- email codes ----

// NewCode returns a uniformly random 6-digit code.
func NewCode() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(1_000_000))
	return fmt.Sprintf("%06d", n.Int64())
}

// HashCode binds a code to its row and purpose with HMAC-SHA256, so the stored hash
// cannot be brute-forced offline without the server key.
func HashCode(secret []byte, scope, code string) string {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(scope + ":" + code))
	return hex.EncodeToString(m.Sum(nil))
}

func CodeMatches(secret []byte, scope, code, hash string) bool {
	return hmac.Equal([]byte(HashCode(secret, scope, code)), []byte(hash))
}

// ---- gate device API keys ----

// NewGateKey returns a high-entropy key (shown once) and its SHA-256 hash (stored).
func NewGateKey() (key, hash string) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	key = "plms_gk_" + base64.RawURLEncoding.EncodeToString(b)
	return key, HashGateKey(key)
}

func HashGateKey(key string) string {
	s := sha256.Sum256([]byte(key))
	return hex.EncodeToString(s[:])
}

// RandomToken returns n random bytes as URL-safe base64.
func RandomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
