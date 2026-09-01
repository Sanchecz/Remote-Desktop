package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
	"time"
)

func actionTestKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return public, private
}

func signedAgentTestJob(t *testing.T, private ed25519.PrivateKey, cfg *config, envelope signedActionEnvelope) *remoteJob {
	t.Helper()
	payload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return &remoteJob{ID: envelope.ActionJobID, Type: "action", Payload: map[string]any{
		"signedEnvelope": base64.RawStdEncoding.EncodeToString(payload),
		"signature":      base64.RawStdEncoding.EncodeToString(ed25519.Sign(private, payload)),
	}}
}

func TestActionSigningKeyPinsAndRejectsRotation(t *testing.T) {
	first, _ := actionTestKey(t)
	second, _ := actionTestKey(t)
	cfg := &config{}
	encoded := base64.RawStdEncoding.EncodeToString(first)
	changed, err := applyActionSigningKey(cfg, encoded)
	if err != nil || !changed || cfg.ActionSigningPublicKey != encoded {
		t.Fatalf("first key was not pinned: changed=%v err=%v cfg=%#v", changed, err, cfg)
	}
	changed, err = applyActionSigningKey(cfg, encoded)
	if err != nil || changed {
		t.Fatalf("identical key should be stable: changed=%v err=%v", changed, err)
	}
	if _, err = applyActionSigningKey(cfg, base64.RawStdEncoding.EncodeToString(second)); err == nil {
		t.Fatal("silent signing-key rotation was accepted")
	}
}

func TestSignedActionVerificationRejectsTamperingWrongDeviceAndExpiry(t *testing.T) {
	public, private := actionTestKey(t)
	cfg := &config{DeviceID: "device-a", ActionSigningPublicKey: base64.RawStdEncoding.EncodeToString(public)}
	now := time.Now().Unix()
	parameters := map[string]any{}
	envelope := signedActionEnvelope{
		Version: 1, ActionJobID: "job-1", DeviceID: cfg.DeviceID, Action: "diagnostic.system",
		Parameters: json.RawMessage(`{}`), IssuedAt: now, ExpiresAt: now + 120,
		Nonce: strings.Repeat("n", 32), RequestHash: agentActionRequestHash(cfg.DeviceID, "diagnostic.system", parameters),
	}
	job := signedAgentTestJob(t, private, cfg, envelope)
	if _, _, err := verifySignedActionJob(cfg, job); err != nil {
		t.Fatalf("valid action was rejected: %v", err)
	}

	tampered := *job
	tampered.Payload = map[string]any{"signedEnvelope": job.Payload["signedEnvelope"], "signature": strings.Repeat("A", 86)}
	if _, _, err := verifySignedActionJob(cfg, &tampered); err == nil {
		t.Fatal("tampered signature was accepted")
	}

	wrongDevice := envelope
	wrongDevice.DeviceID = "device-b"
	wrongDevice.RequestHash = agentActionRequestHash("device-b", wrongDevice.Action, parameters)
	if _, _, err := verifySignedActionJob(cfg, signedAgentTestJob(t, private, cfg, wrongDevice)); err == nil {
		t.Fatal("action for a different device was accepted")
	}

	expired := envelope
	expired.IssuedAt = now - 200
	expired.ExpiresAt = now - 1
	if _, _, err := verifySignedActionJob(cfg, signedAgentTestJob(t, private, cfg, expired)); err == nil {
		t.Fatal("expired action was accepted")
	}
}

func TestSignedActionRejectsInjectionAndReplayNonceIsDetected(t *testing.T) {
	if _, err := decodeAndNormalizeActionParameters("service.restart", json.RawMessage(`{"name":"spooler; whoami"}`)); err == nil {
		t.Fatal("service-name injection was accepted")
	}
	cfg := &config{ActionNonces: []string{"nonce-already-used"}}
	if !actionNonceSeen(cfg, "nonce-already-used") || actionNonceSeen(cfg, "new-nonce") {
		t.Fatal("nonce replay cache returned an invalid result")
	}
}

func TestExpandedSignedActionsRejectUnsafeParameters(t *testing.T) {
	validSHA := strings.Repeat("b", 64)
	tests := []struct {
		name   string
		action string
		raw    string
	}{
		{"download requires HTTPS", "file.download", `{"url":"http://example.test/file","sha256":"` + validSHA + `","fileName":"file.bin"}`},
		{"download rejects traversal", "file.download", `{"url":"https://example.test/file","sha256":"` + validSHA + `","fileName":"../file.bin"}`},
		{"package rejects shell", "package.install", `{"packageId":"curl;whoami"}`},
		{"package rejects extras", "package.install", `{"packageId":"curl","extra":true}`},
		{"group rejects newline", "local.group.add_member", `{"member":"user","group":"admins\\nwhoami"}`},
		{"VPN rejects shell", "windows.vpn.upsert", `{"name":"Office","serverAddress":"vpn.example.test;whoami","tunnelType":"Ikev2","authenticationMethod":"Eap"}`},
		{"script rejects unsupported shell", "script.execute", `{"shell":"python","script":"print(1)"}`},
		{"script rejects extras", "script.execute", `{"shell":"powershell","script":"Write-Output ok","secret":"ignored"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := decodeAndNormalizeActionParameters(test.action, json.RawMessage(test.raw)); err == nil {
				t.Fatalf("unsafe parameters were accepted for %s", test.action)
			}
		})
	}
}

func TestAgentLANAndPrinterActionValidation(t *testing.T) {
	if _, err := decodeAndNormalizeActionParameters("diagnostic.lan_scan", json.RawMessage(`{"subnet":"192.168.10.0/24"}`)); err != nil {
		t.Fatalf("valid LAN scan rejected: %v", err)
	}
	if _, err := decodeAndNormalizeActionParameters("diagnostic.lan_scan", json.RawMessage(`{"subnet":"1.1.1.0/24"}`)); err == nil {
		t.Fatal("public LAN scan range accepted")
	}
	if runtime.GOOS == "windows" {
		if _, err := decodeAndNormalizeActionParameters("network.rdp.open", json.RawMessage(`{"host":"192.168.1.10","port":3389,"username":"admin"}`)); err != nil {
			t.Fatalf("valid RDP action rejected: %v", err)
		}
		if _, err := decodeAndNormalizeActionParameters("windows.scan_folder.configure", json.RawMessage(`{"path":"C:\\RemoteIt Scans","shareName":"RemoteItScans","principal":"Users"}`)); err != nil {
			t.Fatalf("valid scan folder rejected: %v", err)
		}
	}
}
