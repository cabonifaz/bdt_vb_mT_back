// Package security implementa los puertos criptográficos.
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// keyVersion permite rotar la clave: el byte inicial del texto cifrado indica con qué clave se cifró.
const keyVersion byte = 1

type AESCipher struct {
	aead     cipher.AEAD
	blindKey []byte
}

// NewAESCipher recibe claves de 32 bytes en base64 (DATA_ENCRYPTION_KEY y BLIND_INDEX_KEY).
func NewAESCipher(dataKeyB64, blindKeyB64 string) (*AESCipher, error) {
	dk, err := decodeKey(dataKeyB64)
	if err != nil {
		return nil, errors.New("DATA_ENCRYPTION_KEY inválida (32 bytes en base64)")
	}
	bk, err := decodeKey(blindKeyB64)
	if err != nil {
		return nil, errors.New("BLIND_INDEX_KEY inválida (32 bytes en base64)")
	}
	block, err := aes.NewCipher(dk)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &AESCipher{aead: aead, blindKey: bk}, nil
}

func decodeKey(b64 string) ([]byte, error) {
	k, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil || len(k) != 32 {
		return nil, errors.New("invalid key")
	}
	return k, nil
}

func (c *AESCipher) EncryptBytes(plain []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := make([]byte, 0, 1+len(nonce)+len(plain)+c.aead.Overhead())
	out = append(out, keyVersion)
	out = append(out, nonce...)
	return c.aead.Seal(out, nonce, plain, []byte{keyVersion}), nil
}

func (c *AESCipher) DecryptBytes(ct []byte) ([]byte, error) {
	ns := c.aead.NonceSize()
	if len(ct) < 1+ns+c.aead.Overhead() || ct[0] != keyVersion {
		return nil, errors.New("ciphertext inválido")
	}
	return c.aead.Open(nil, ct[1:1+ns], ct[1+ns:], []byte{keyVersion})
}

func (c *AESCipher) Encrypt(plain string) ([]byte, error) { return c.EncryptBytes([]byte(plain)) }

func (c *AESCipher) Decrypt(ct []byte) (string, error) {
	if len(ct) == 0 {
		return "", nil
	}
	b, err := c.DecryptBytes(ct)
	return string(b), err
}

// BlindIndex normaliza (NFKC, minúsculas, sin espacios) y calcula HMAC-SHA256.
func (c *AESCipher) BlindIndex(value string) string {
	v := strings.ToLower(norm.NFKC.String(strings.TrimFunc(value, unicode.IsSpace)))
	m := hmac.New(sha256.New, c.blindKey)
	m.Write([]byte(v))
	return hex.EncodeToString(m.Sum(nil))
}
