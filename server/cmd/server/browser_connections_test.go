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

func TestGuacamoleRDPParametersPreserveQualityAndFastPaths(t *testing.T) {
	parameters := guacamoleConnectionParameters(browserConnectionInput{
		Protocol: "rdp", Username: "operator", Password: "temporary", Domain: "OFFICE",
	}, "app", 41234)
	want := map[string]string{
		"hostname": "app", "port": "41234", "username": "operator", "password": "temporary", "domain": "OFFICE",
		"resize-method": "display-update", "color-depth": "24", "enable-font-smoothing": "true",
		"disable-gfx": "false", "disable-bitmap-caching": "false", "disable-offscreen-caching": "false",
	}
	for key, expected := range want {
		if parameters[key] != expected {
			t.Fatalf("unexpected RDP parameter %s: got=%q want=%q", key, parameters[key], expected)
		}
	}
	if parameters["force-lossless"] != "" {
		t.Fatal("RDP must keep Guacamole adaptive compression instead of forcing a high-latency lossless stream")
	}
}

func TestGuacamoleSSHParametersDoNotLeakRDPOptions(t *testing.T) {
	parameters := guacamoleConnectionParameters(browserConnectionInput{Protocol: "ssh"}, "app", 40222)
	if parameters["enable-sftp"] != "true" {
		t.Fatal("SSH connection must keep SFTP support")
	}
	for _, key := range []string{"color-depth", "disable-gfx", "resize-method"} {
		if _, exists := parameters[key]; exists {
			t.Fatalf("SSH unexpectedly contains RDP parameter %s", key)
		}
	}
}
