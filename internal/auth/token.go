package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// MinSecretSize is the minimum HMAC key length for signing session tokens.
const MinSecretSize = 32

var (
	// ErrTokenInvalid means the token is malformed or its signature is wrong.
	ErrTokenInvalid = errors.New("auth: invalid token")
	// ErrTokenExpired means the token's exp has passed.
	ErrTokenExpired = errors.New("auth: token expired")
)

// Claims is the payload of a session token. Permissions are intentionally NOT
// included — they go stale; load them from storage per request instead.
type Claims struct {
	Subject   string `json:"sub"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

// Tokenizer issues and verifies HS256 (JWT) session tokens. Both the
// username/password and Telegram login paths mint the same token type.
type Tokenizer struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

// NewTokenizer requires a secret of at least MinSecretSize bytes.
func NewTokenizer(secret []byte, ttl time.Duration) (*Tokenizer, error) {
	if len(secret) < MinSecretSize {
		return nil, fmt.Errorf("auth: token secret must be at least %d bytes", MinSecretSize)
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &Tokenizer{secret: secret, ttl: ttl, now: time.Now}, nil
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

// Issue mints a signed token for subject/username/role, stamping iat/exp.
func (t *Tokenizer) Issue(subject, username, role string) (string, error) {
	now := t.now()
	claims := Claims{
		Subject:   subject,
		Username:  username,
		Role:      role,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(t.ttl).Unix(),
	}

	header, err := json.Marshal(jwtHeader{Alg: "HS256", Typ: "JWT"})
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	signingInput := encodeSegment(header) + "." + encodeSegment(payload)
	return signingInput + "." + encodeSegment(t.sign(signingInput)), nil
}

// Verify checks the signature and expiry and returns the claims.
func (t *Tokenizer) Verify(token string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, ErrTokenInvalid
	}

	expected := t.sign(parts[0] + "." + parts[1])
	got, err := decodeSegment(parts[2])
	if err != nil || subtle.ConstantTimeCompare(got, expected) != 1 {
		return Claims{}, ErrTokenInvalid
	}

	payload, err := decodeSegment(parts[1])
	if err != nil {
		return Claims{}, ErrTokenInvalid
	}
	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Claims{}, ErrTokenInvalid
	}
	if t.now().Unix() >= claims.ExpiresAt {
		return Claims{}, ErrTokenExpired
	}
	return claims, nil
}

func (t *Tokenizer) sign(input string) []byte {
	mac := hmac.New(sha256.New, t.secret)
	mac.Write([]byte(input))
	return mac.Sum(nil)
}

func encodeSegment(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeSegment(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}
