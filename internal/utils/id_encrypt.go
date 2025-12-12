package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"fmt"
)

func EncryptID(id uint, key string) (string, error) {
	plaintext := []byte(fmt.Sprintf("%d", id))

	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", err
	}

	ciphertext := make([]byte, aes.BlockSize+len(plaintext))
	iv := ciphertext[:aes.BlockSize]

	stream := cipher.NewCFBEncrypter(block, iv)
	stream.XORKeyStream(ciphertext[aes.BlockSize:], plaintext)

	return base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

func DecryptID(enc string, key string) (uint, error) {
	ciphertext, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return 0, err
	}

	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return 0, err
	}

	iv := ciphertext[:aes.BlockSize]
	plaintext := ciphertext[aes.BlockSize:]

	stream := cipher.NewCFBDecrypter(block, iv)
	stream.XORKeyStream(plaintext, plaintext)

	var id uint
	_, err = fmt.Sscanf(string(plaintext), "%d", &id)
	if err != nil {
		return 0, err
	}

	return id, nil
}
