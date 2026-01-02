package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

func EncryptID(id uint, key string) (string, error) {
	plaintext := []byte(fmt.Sprintf("%d", id))

	k := []byte(key)
	if len(k) != 16 && len(k) != 24 && len(k) != 32 {
		return "", fmt.Errorf("invalid key length: %d (must be 16/24/32)", len(k))
	}

	block, err := aes.NewCipher(k)
	if err != nil {
		return "", err
	}

	ciphertext := make([]byte, aes.BlockSize+len(plaintext))

	// IV random (Wajib untuk CFB/CTR, dll)
	iv := ciphertext[:aes.BlockSize]
	if _, err := rand.Read(iv); err != nil {
		return "", fmt.Errorf("failed to read random iv: %w", err)
	}

	stream := cipher.NewCFBEncrypter(block, iv)
	stream.XORKeyStream(ciphertext[aes.BlockSize:], plaintext)

	return base64.RawURLEncoding.EncodeToString(ciphertext), nil
}



func DecryptID(enc string, key string) (uint, error) {
	if enc == "" {
		return 0, fmt.Errorf("empty encrypted id")
	}

	// Coba decode base64 dulu
	ciphertext, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		// Fallback: bisa jadi plain number ("6" dll)
		var idPlain uint
		if _, err2 := fmt.Sscanf(enc, "%d", &idPlain); err2 == nil {
			return idPlain, nil
		}
		return 0, fmt.Errorf("decode base64 failed: %w", err)
	}

	// <<< PERBAIKAN UTAMA DI SINI >>>
	if len(ciphertext) < aes.BlockSize {
		// Mungkin ini plain number yang kebetulan lolos decode base64
		var idPlain uint
		if _, err2 := fmt.Sscanf(enc, "%d", &idPlain); err2 == nil {
			return idPlain, nil
		}
		return 0, fmt.Errorf("ciphertext too short: len=%d", len(ciphertext))
	}

	k := []byte(key)
	if len(k) != 16 && len(k) != 24 && len(k) != 32 {
		return 0, fmt.Errorf("invalid key length: %d (must be 16/24/32)", len(k))
	}

	block, err := aes.NewCipher(k)
	if err != nil {
		return 0, err
	}

	iv := ciphertext[:aes.BlockSize]
	body := ciphertext[aes.BlockSize:]

	plaintext := make([]byte, len(body))
	stream := cipher.NewCFBDecrypter(block, iv)
	stream.XORKeyStream(plaintext, body)

	var id uint
	if _, err := fmt.Sscanf(string(plaintext), "%d", &id); err != nil {
		return 0, fmt.Errorf("parse id failed: %w", err)
	}

	return id, nil
}

