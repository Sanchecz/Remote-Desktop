package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestToolSurfaceDoesNotExposeApprovalOrShell(t *testing.T) {
	want := []string{"remoteit_cancel_action", "remoteit_create_action", "remoteit_get_action", "remoteit_get_device", "remoteit_list_devices", "remoteit_plan_action"}
	if got := sortedToolNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected MCP tools: %#v", got)
	}
	for _, name := range sortedToolNames() {
		if strings.Contains(name, "approve") || strings.Contains(name, "shell") || strings.Contains(name, "command") {
			t.Fatalf("unsafe MCP tool exposed: %s", name)
		}
	}
}

func TestInitializeAndToolsListProtocol(t *testing.T) {
	b := &bridge{}
	input := strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"2025-06-18\"}}\n{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/list\"}\n")
	var output bytes.Buffer
	if err := b.serve(input, &output); err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(&output)
	responses := 0
	for scanner.Scan() {
		var response map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response["jsonrpc"] != "2.0" || response["error"] != nil {
			t.Fatalf("invalid MCP response: %#v", response)
		}
		if responses == 0 {
			result, ok := response["result"].(map[string]any)
			if !ok || !strings.Contains(strings.ToLower(fmt.Sprint(result["instructions"])), "plan") || !strings.Contains(strings.ToLower(fmt.Sprint(result["instructions"])), "approval") {
				t.Fatalf("initialize response does not describe the guarded workflow: %#v", response)
			}
		}
		responses++
	}
	if responses != 2 {
		t.Fatalf("expected 2 responses, got %d", responses)
	}
}

func TestActionSchemaExposesExpandedGuardedCatalog(t *testing.T) {
	var plan toolDefinition
	for _, tool := range remoteItTools() {
		if tool.Name == "remoteit_plan_action" {
			plan = tool
			break
		}
	}
	properties, _ := plan.InputSchema["properties"].(map[string]any)
	action, _ := properties["action"].(map[string]any)
	values, _ := action["enum"].([]string)
	for _, expected := range []string{"file.download", "package.install", "local.group.add_member", "windows.vpn.upsert", "system.reboot", "script.execute"} {
		found := false
		for _, value := range values {
			found = found || value == expected
		}
		if !found {
			t.Fatalf("expanded action %s is missing from MCP schema: %#v", expected, values)
		}
	}
}

func TestDeviceToolUsesBearerTokenAndFiltersOnline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer rmt_mcp_test-token-value-12345678901234567890" {
			t.Fatalf("missing bearer token: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"devices":[{"name":"online","online":true},{"name":"offline","online":false}]}`))
	}))
	defer server.Close()
	b := &bridge{baseURL: server.URL, token: "rmt_mcp_test-token-value-12345678901234567890", client: server.Client()}
	result, err := b.callTool(t.Context(), "remoteit_list_devices", map[string]any{"onlineOnly": true})
	if err != nil {
		t.Fatal(err)
	}
	items := result.(map[string]any)
	if items["count"] != 1 {
		t.Fatalf("online filter failed: %#v", items)
	}
}

func TestCreateActionRequiresIdempotencyKeyBeforeNetwork(t *testing.T) {
	b := &bridge{}
	_, err := b.callTool(t.Context(), "remoteit_create_action", map[string]any{"deviceId": "123456789", "action": "diagnostic.system"})
	if err == nil || !strings.Contains(err.Error(), "idempotencyKey") {
		t.Fatalf("missing idempotency key was not rejected: %v", err)
	}
}
