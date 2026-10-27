// Package tokens: utilidades criptográficas neutrales (sin estado) usadas por app y adaptadores.
package tokens

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
)

// Random genera n bytes aleatorios en base64url.
func Random(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// Hash: SHA-256 de tokens aleatorios de alta entropía (sesiones, invitaciones).
func Hash(token string) string {
	s := sha256.Sum256([]byte(token))
	return hex.EncodeToString(s[:])
}

// Keyed: HMAC-SHA256 con pepper del servidor (API keys).
func Keyed(pepper []byte, secret string) string {
	m := hmac.New(sha256.New, pepper)
	m.Write([]byte(secret))
	return hex.EncodeToString(m.Sum(nil))
}

func Equal(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }
