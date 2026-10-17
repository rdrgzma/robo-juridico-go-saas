package auth

import (
	"bytes"
	"encoding/base32"
	"strings"
	"testing"
	"time"
)

func TestArgon2idPasswordVerification(t *testing.T) {
	pass := "uma-frase-segura-12345"
	a, err := HashPassword(pass)
	if err != nil {
		t.Fatal(err)
	}
	b, err := HashPassword(pass)
	if err != nil {
		t.Fatal(err)
	}
	if a == b || strings.Contains(a, pass) || !VerifyPassword(a, pass) || VerifyPassword(a, "incorreta") {
		t.Fatal("hash ou verificação inválidos")
	}
	if VerifyPassword(strings.Replace(a, "m=65536", "m=999999999", 1), pass) {
		t.Fatal("parâmetros arbitrários aceitos")
	}
	if _, err := HashPassword("curta"); err == nil {
		t.Fatal("senha curta aceita")
	}
}
func TestTOTPAgainstRFC6238SHA1Vectors(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	for _, tc := range []struct {
		unix int64
		code string
	}{{59, "94287082"}, {1111111109, "07081804"}, {1111111111, "14050471"}, {1234567890, "89005924"}, {2000000000, "69279037"}, {20000000000, "65353130"}} {
		code, err := TOTP(secret, time.Unix(tc.unix, 0), 8)
		if err != nil || code != tc.code {
			t.Fatal(tc.unix, code, err)
		}
	}
	if _, ok := totpStep(secret, "287082", time.Unix(59, 0)); !ok {
		t.Fatal("código de 6 dígitos inválido")
	}
	if _, ok := totpStep(secret, "000000", time.Unix(59, 0)); ok {
		t.Fatal("código incorreto aceito")
	}
}
func TestEncryptedPayloadBindsPurposeAndRejectsTampering(t *testing.T) {
	s := &Service{key: bytes.Repeat([]byte{7}, 32)}
	ciphertext, err := s.seal("segredo", "user:a")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, []byte("segredo")) {
		t.Fatal("plaintext armazenado")
	}
	plain, err := s.open(ciphertext, "user:a")
	if err != nil || plain != "segredo" {
		t.Fatal(err)
	}
	if _, err := s.open(ciphertext, "user:b"); err == nil {
		t.Fatal("propósito diferente aceito")
	}
	ciphertext[len(ciphertext)-1] ^= 1
	if _, err := s.open(ciphertext, "user:a"); err == nil {
		t.Fatal("ciphertext alterado aceito")
	}
}
