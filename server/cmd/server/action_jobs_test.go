package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestActionParametersRejectInjectionAndFractionalPID(t *testing.T) {
	if _, err := normalizeActionParameters("service.restart", map[string]any{"name": `spooler; whoami`}); err == nil {
		t.Fatal("service-name command injection was accepted")
	}
	if _, err := normalizeActionParameters("service.restart", map[string]any{"name": "spooler\nwhoami"}); err == nil {
		t.Fatal("multiline service name was accepted")
	}
	if _, err := normalizeActionParameters("process.terminate", map[string]any{"pid": 12.5}); err == nil {
		t.Fatal("fractional PID was accepted")
	}
	if got, err := normalizeActionParameters("service.restart", map[string]any{"name": "RemoteIt Agent"}); err != nil || got["name"] != "RemoteIt Agent" {
		t.Fatalf("safe service name was rejected: %#v %v", got, err)
	}
	if _, err := normalizeActionParameters("service.restart", map[string]any{"name": "RemoteIt Agent", "extra": "ignored"}); err == nil {
		t.Fatal("extra signed action parameter was accepted")
	}
}

func TestExpandedActionParametersAreStrict(t *testing.T) {
	validSHA := strings.Repeat("a", 64)
	if _, err := normalizeActionParameters("file.download", map[string]any{"url": "http://example.test/file", "sha256": validSHA, "fileName": "file.bin"}); err == nil {
		t.Fatal("non-HTTPS download was accepted")
	}
	if _, err := normalizeActionParameters("file.download", map[string]any{"url": "https://example.test/file#fragment", "sha256": validSHA, "fileName": "file.bin"}); err == nil {
		t.Fatal("fragmented download URL was accepted")
	}
	if _, err := normalizeActionParameters("file.download", map[string]any{"url": "https://example.test/file", "sha256": validSHA, "fileName": "../file.bin"}); err == nil {
		t.Fatal("download path traversal was accepted")
	}
	if _, err := normalizeActionParameters("package.install", map[string]any{"packageId": "curl; whoami"}); err == nil {
		t.Fatal("package command injection was accepted")
	}
	if _, err := normalizeActionParameters("local.group.add_member", map[string]any{"member": "user", "group": "Administrators\nwhoami"}); err == nil {
		t.Fatal("local group command injection was accepted")
	}
	if _, err := normalizeActionParameters("windows.vpn.upsert", map[string]any{"name": "Office", "serverAddress": "vpn.example.test", "tunnelType": "Ikev2", "authenticationMethod": "Eap"}); err != nil {
		t.Fatalf("safe VPN profile was rejected: %v", err)
	}
	if _, err := normalizeActionParameters("windows.vpn.upsert", map[string]any{"name": "Office", "serverAddress": "vpn.example.test;whoami", "tunnelType": "Ikev2", "authenticationMethod": "Eap"}); err == nil {
		t.Fatal("unsafe VPN server was accepted")
	}
	if _, err := normalizeActionParameters("script.execute", map[string]any{"shell": "powershell", "script": "Write-Output 'ok'"}); err != nil {
		t.Fatalf("safe owner-approved script was rejected: %v", err)
	}
	if _, err := normalizeActionParameters("script.execute", map[string]any{"shell": "powershell", "script": "Write-Output 'ok'", "secret": "must-not-be-ignored"}); err == nil {
		t.Fatal("extra script parameter was accepted")
	}
	if _, err := normalizeActionParameters("script.execute", map[string]any{"shell": "powershell", "script": strings.Repeat("x", 16*1024+1)}); err == nil {
		t.Fatal("oversized script was accepted")
	}
}

func TestActionRequestHashDeterministicAndBoundToDevice(t *testing.T) {
	parametersA := map[string]any{"name": "RemoteIt Agent"}
	parametersB := map[string]any{"name": "RemoteIt Agent"}
	first := actionRequestHash("device-a", "service.restart", parametersA)
	second := actionRequestHash("device-a", "service.restart", parametersB)
	if first == "" || first != second {
		t.Fatalf("request hash is not deterministic: %q != %q", first, second)
	}
	if first == actionRequestHash("device-b", "service.restart", parametersB) {
		t.Fatal("request hash is not bound to the target device")
	}
}

func TestActionSignerProducesExactVerifiableEnvelope(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index + 1)
	}
	private := ed25519.NewKeyFromSeed(seed)
	signer := &actionSigner{private: private, public: private.Public().(ed25519.PublicKey)}
	now := time.Now().Unix()
	envelope := signedActionEnvelope{Version: 1, ActionJobID: "job", DeviceID: "device", Action: "diagnostic.system", Parameters: json.RawMessage(`{}`), IssuedAt: now, ExpiresAt: now + 120, Nonce: strings.Repeat("n", 32), RequestHash: "hash"}
	encoded, signatureText, err := signer.marshalAndSign(envelope)
	if err != nil {
		t.Fatal(err)
	}
	payload, payloadErr := base64.RawStdEncoding.DecodeString(encoded)
	signature, signatureErr := base64.RawStdEncoding.DecodeString(signatureText)
	if payloadErr != nil || signatureErr != nil || !ed25519.Verify(signer.public, payload, signature) {
		t.Fatalf("signed envelope did not verify: %v %v", payloadErr, signatureErr)
	}
	payload[0] ^= 1
	if ed25519.Verify(signer.public, payload, signature) {
		t.Fatal("tampered envelope still verified")
	}
}

func TestMutatingActionsAlwaysRequireApproval(t *testing.T) {
	for _, action := range []string{"service.restart", "process.terminate", "file.download", "package.install", "windows.vpn.upsert"} {
		definition := actionDefinitions[action]
		if !definition.ApprovalRequired || definition.Risk != "high" {
			t.Fatalf("mutating action %s is not approval-gated: %#v", action, definition)
		}
	}
	for _, action := range []string{"local.group.add_member", "system.reboot", "script.execute"} {
		definition := actionDefinitions[action]
		if !definition.ApprovalRequired || definition.Risk != "critical" {
			t.Fatalf("critical action %s is not approval-gated: %#v", action, definition)
		}
	}
	for _, action := range []string{"diagnostic.system", "diagnostic.network", "diagnostic.services"} {
		definition := actionDefinitions[action]
		if definition.ApprovalRequired || definition.Risk != "read" {
			t.Fatalf("diagnostic action %s has unexpected risk: %#v", action, definition)
		}
	}
}

func TestOnlyOwnerAndAdminCanUseCodexIntegration(t *testing.T) {
	tests := []struct {
		role string
		want bool
	}{
		{role: "owner", want: true},
		{role: "admin", want: true},
		{role: "technician", want: false},
		{role: "operator", want: false},
		{role: "viewer", want: false},
		{role: "", want: false},
	}
	for _, test := range tests {
		t.Run(test.role, func(t *testing.T) {
			if got := canUseCodexIntegration(test.role); got != test.want {
				t.Fatalf("canUseCodexIntegration(%q)=%v, want %v", test.role, got, test.want)
			}
		})
	}
}

func TestNormalizeLANPrinterAndScanActions(t *testing.T) {
	if result, err := normalizeActionParameters("diagnostic.lan_scan", map[string]any{"subnet": "192.168.50.0/24"}); err != nil || result["subnet"] != "192.168.50.0/24" {
		t.Fatalf("valid LAN scan rejected: %#v, %v", result, err)
	}
	for _, subnet := range []string{"8.8.8.0/24", "10.0.0.0/16", "not-a-network"} {
		if _, err := normalizeActionParameters("diagnostic.lan_scan", map[string]any{"subnet": subnet}); err == nil {
			t.Fatalf("unsafe LAN scan subnet accepted: %q", subnet)
		}
	}
	connection, err := normalizeActionParameters("network.rdp.open", map[string]any{"host": "192.168.1.20", "port": 3389.0, "username": "DOMAIN\\admin"})
	if err != nil || connection["port"] != int64(3389) {
		t.Fatalf("valid RDP action rejected: %#v, %v", connection, err)
	}
	if _, err := normalizeActionParameters("network.ssh.open", map[string]any{"host": "server;whoami", "port": 22, "username": "admin"}); err == nil {
		t.Fatal("unsafe SSH host accepted")
	}
	if _, err := normalizeActionParameters("windows.scan_folder.configure", map[string]any{"path": `C:\RemoteIt Scans`, "shareName": "RemoteItScans", "principal": `DOMAIN\scanner`}); err != nil {
		t.Fatalf("valid scan folder rejected: %v", err)
	}
	if _, err := normalizeActionParameters("windows.scan_folder.configure", map[string]any{"path": `C:\..\Windows`, "shareName": "RemoteItScans", "principal": "Users"}); err == nil {
		t.Fatal("traversing scan folder accepted")
	}
}
