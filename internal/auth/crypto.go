package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"golang.org/x/crypto/argon2"
	"strconv"
	"strings"
	"time"
)

const passwordMemory = 64 * 1024

var passwordSlots = make(chan struct{}, 4)

func RandomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func Digest(token string) []byte { h := sha256.Sum256([]byte(token)); return h[:] }
func HashPassword(password string) (string, error) {
	if len(password) < 12 || len(password) > 128 || len(strings.TrimSpace(password)) < 12 {
		return "", fmt.Errorf("senha deve ter entre 12 e 128 bytes e não ser apenas espaços")
	}
	passwordSlots <- struct{}{}
	defer func() { <-passwordSlots }()
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, 3, passwordMemory, 1, 32)
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=3,p=1$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}
func VerifyPassword(encoded, password string) bool {
	if len(password) > 128 {
		return false
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" || parts[3] != "m=65536,t=3,p=1" {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) != 16 {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) != 32 {
		return false
	}
	passwordSlots <- struct{}{}
	defer func() { <-passwordSlots }()
	got := argon2.IDKey([]byte(password), salt, 3, passwordMemory, 1, 32)
	return subtle.ConstantTimeCompare(expected, got) == 1
}
func newAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func (s *Service) seal(text, purpose string) ([]byte, error) {
	a, _ := newAEAD(s.key)
	nonce := make([]byte, a.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return a.Seal(nonce, nonce, []byte(text), []byte(purpose)), nil
}
func (s *Service) open(payload []byte, purpose string) (string, error) {
	a, _ := newAEAD(s.key)
	n := a.NonceSize()
	if len(payload) < n {
		return "", ErrDenied
	}
	b, err := a.Open(nil, payload[:n], payload[n:], []byte(purpose))
	return string(b), err
}
func (s *Service) mac(purpose, value string) string {
	h := hmac.New(sha256.New, s.key)
	_, _ = h.Write([]byte(purpose + ":" + value))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
func (s *Service) CSRF(token string) string { return s.mac("csrf", token) }
func TOTP(secret string, at time.Time, digits int) (string, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil || len(key) < 20 || (digits != 6 && digits != 8) {
		return "", ErrDenied
	}
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(at.Unix()/30))
	h := hmac.New(sha1.New, key)
	_, _ = h.Write(counter[:])
	sum := h.Sum(nil)
	offset := sum[len(sum)-1] & 15
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	modulus := uint32(1000000)
	if digits == 8 {
		modulus = 100000000
	}
	return fmt.Sprintf("%0*d", digits, value%modulus), nil
}
func totpStep(secret, code string, now time.Time) (int64, bool) {
	if len(code) != 6 {
		return 0, false
	}
	if _, err := strconv.Atoi(code); err != nil {
		return 0, false
	}
	for _, shift := range []int{0, -1, 1} {
		at := now.Add(time.Duration(shift) * 30 * time.Second)
		expected, err := TOTP(secret, at, 6)
		if err == nil && hmac.Equal([]byte(expected), []byte(code)) {
			return at.Unix() / 30, true
		}
	}
	return 0, false
}
