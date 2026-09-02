package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestEncryptGuacamoleJSONSignsAndEncryptsExpectedPayload(t *testing.T) {
	secret := []byte("0123456789abcdef")
	encoded, err := encryptGuacamoleJSON(secret, map[string]any{"username": "owner", "expires": int64(123), "connections": map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(encrypted)%aes.BlockSize != 0 {
		t.Fatalf("invalid encrypted JSON: size=%d err=%v", len(encrypted), err)
	}
	block, _ := aes.NewCipher(secret)
	message := append([]byte(nil), encrypted...)
	cipher.NewCBCDecrypter(block, make([]byte, aes.BlockSize)).CryptBlocks(message, message)
	padding := int(message[len(message)-1])
	if padding < 1 || padding > aes.BlockSize {
		t.Fatalf("invalid PKCS7 padding: %d", padding)
	}
	message = message[:len(message)-padding]
	if len(message) <= sha256.Size {
		t.Fatal("signed JSON is too short")
	}
	signature, plaintext := message[:sha256.Size], message[sha256.Size:]
	expected := hmac.New(sha256.New, secret)
	_, _ = expected.Write(plaintext)
	if !hmac.Equal(signature, expected.Sum(nil)) {
		t.Fatal("Guacamole JSON HMAC does not match")
	}
	var decoded map[string]any
	if json.Unmarshal(plaintext, &decoded) != nil || decoded["username"] != "owner" {
		t.Fatalf("unexpected decrypted payload: %s", plaintext)
	}
}
