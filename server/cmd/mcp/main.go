package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	bridgeName            = "remoteit"
	bridgeVersion         = "1.0.17"
	defaultProtocol       = "2025-06-18"
	defaultRemoteItServer = "https://supportgenesis.ru"
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type bridge struct {
	baseURL string
	token   string
	client  *http.Client
}

type toolDefinition struct {
	Name        string         `json:"name"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations map[string]any `json:"annotations,omitempty"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "RemoteIt MCP:", err)
		os.Exit(1)
	}
}

func run() error {
	serverFlag := flag.String("server", envOr("REMOTEIT_URL", defaultRemoteItServer), "HTTPS address of the RemoteIt server")
	tokenFlag := flag.String("token", envOr("REMOTEIT_INTEGRATION_TOKEN", ""), "RemoteIt integration token")
	flag.Parse()
	baseURL, err := normalizeServerURL(*serverFlag)
	if err != nil {
		return err
	}
	token := strings.TrimSpace(*tokenFlag)
	if !strings.HasPrefix(token, "rmt_mcp_") || len(token) < 40 {
		return errors.New("set a valid REMOTEIT_INTEGRATION_TOKEN created by a RemoteIt owner or administrator")
	}
	b := &bridge{
		baseURL: baseURL,
		token:   token,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
	return b.serve(os.Stdin, os.Stdout)
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func normalizeServerURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("RemoteIt server must be a plain HTTPS origin")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", errors.New("RemoteIt server URL must not contain a path")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func (b *bridge) serve(input io.Reader, output io.Writer) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	encoder := json.NewEncoder(output)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var request rpcRequest
		if err := json.Unmarshal(line, &request); err != nil {
			if encodeErr := encoder.Encode(rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "Parse error"}}); encodeErr != nil {
				return encodeErr
			}
			continue
		}
		if len(request.ID) == 0 {
			continue
		}
		response := b.handle(context.Background(), request)
		if err := encoder.Encode(response); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (b *bridge) handle(ctx context.Context, request rpcRequest) rpcResponse {
	response := rpcResponse{JSONRPC: "2.0", ID: request.ID}
	switch request.Method {
	case "initialize":
		var parameters struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(request.Params, &parameters)
		protocol := parameters.ProtocolVersion
		if protocol == "" {
			protocol = defaultProtocol
		}
		response.Result = map[string]any{
			"protocolVersion": protocol,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]string{"name": bridgeName, "version": bridgeVersion},
			"instructions":    "RemoteIt exposes audited administration actions. Always plan first and prefer a typed action. Use script.execute only when the task cannot be represented by the typed catalog, never put secrets in its script, and show the operator the exact script. Mutating actions require separate approval in the RemoteIt web panel; critical actions require the owner and cannot be approved through MCP.",
		}
	case "ping":
		response.Result = map[string]any{}
	case "tools/list":
		response.Result = map[string]any{"tools": remoteItTools()}
	case "tools/call":
		var parameters struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(request.Params, &parameters); err != nil || strings.TrimSpace(parameters.Name) == "" {
			response.Error = &rpcError{Code: -32602, Message: "Invalid tools/call parameters"}
			break
		}
		result, err := b.callTool(ctx, parameters.Name, parameters.Arguments)
		if err != nil {
			response.Result = map[string]any{"content": []map[string]string{{"type": "text", "text": err.Error()}}, "isError": true}
			break
		}
		pretty, _ := json.MarshalIndent(result, "", "  ")
		response.Result = map[string]any{
			"content":           []map[string]string{{"type": "text", "text": string(pretty)}},
			"structuredContent": result,
			"isError":           false,
		}
	default:
		response.Error = &rpcError{Code: -32601, Message: "Method not found"}
	}
	return response
}

func remoteItTools() []toolDefinition {
	actionEnum := []string{
		"diagnostic.system",
		"diagnostic.network",
		"diagnostic.services",
		"service.restart",
		"process.terminate",
		"file.download",
		"package.install",
		"local.group.add_member",
		"windows.vpn.upsert",
		"system.reboot",
		"script.execute",
	}
	deviceProperty := map[string]any{"type": "string", "minLength": 1, "description": "RemoteIt device UUID or 9-digit Remote ID"}
	actionProperties := map[string]any{
		"deviceId": deviceProperty,
		"action":   map[string]any{"type": "string", "enum": actionEnum},
		"parameters": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
			"description":          "Exact typed parameters: {} for diagnostics/system.reboot; {name} for service.restart; {pid} for process.terminate; {url,sha256,fileName} for file.download; {packageId} for package.install; {member,group} for local.group.add_member; {name,serverAddress,tunnelType,authenticationMethod} for windows.vpn.upsert; {shell,script} for script.execute. The RemoteIt server and Agent reject missing, extra, or unsafe fields. Never put passwords, tokens, private keys, or other secrets in a script.",
		},
	}
	return []toolDefinition{
		{Name: "remoteit_list_devices", Title: "List RemoteIt devices", Description: "Lists devices visible to the authenticated RemoteIt owner or administrator, including online state, OS, IPs, load and Remote ID.", InputSchema: objectSchema(map[string]any{"onlineOnly": map[string]any{"type": "boolean", "default": false}, "query": map[string]any{"type": "string"}}, nil), Annotations: map[string]any{"readOnlyHint": true}},
		{Name: "remoteit_get_device", Title: "Get RemoteIt device", Description: "Returns current inventory and connectivity for one RemoteIt device.", InputSchema: objectSchema(map[string]any{"deviceId": deviceProperty}, []string{"deviceId"}), Annotations: map[string]any{"readOnlyHint": true}},
		{Name: "remoteit_plan_action", Title: "Plan RemoteIt action", Description: "Validates a typed action and returns exact steps, risk, approval requirement and rollback guidance without executing it.", InputSchema: objectSchema(actionProperties, []string{"deviceId", "action"}), Annotations: map[string]any{"readOnlyHint": true}},
		{Name: "remoteit_create_action", Title: "Create RemoteIt action", Description: "Creates an audited typed action. Diagnostics may queue immediately; any mutating action remains awaiting approval in the RemoteIt panel.", InputSchema: objectSchema(mergeProperties(actionProperties, map[string]any{"idempotencyKey": map[string]any{"type": "string", "maxLength": 128}}), []string{"deviceId", "action", "idempotencyKey"}), Annotations: map[string]any{"destructiveHint": false}},
		{Name: "remoteit_get_action", Title: "Get RemoteIt action", Description: "Reads status, output and audit metadata for an action created by this integration user.", InputSchema: objectSchema(map[string]any{"actionId": map[string]any{"type": "string", "minLength": 1}}, []string{"actionId"}), Annotations: map[string]any{"readOnlyHint": true}},
		{Name: "remoteit_cancel_action", Title: "Cancel RemoteIt action", Description: "Cancels this integration user's action while it is awaiting approval or still queued.", InputSchema: objectSchema(map[string]any{"actionId": map[string]any{"type": "string", "minLength": 1}}, []string{"actionId"}), Annotations: map[string]any{"destructiveHint": true}},
	}
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	schema := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func mergeProperties(left, right map[string]any) map[string]any {
	merged := make(map[string]any, len(left)+len(right))
	for key, value := range left {
		merged[key] = value
	}
	for key, value := range right {
		merged[key] = value
	}
	return merged
}

func (b *bridge) callTool(ctx context.Context, name string, arguments map[string]any) (any, error) {
	arguments = copyMap(arguments)
	switch name {
	case "remoteit_list_devices":
		var response struct {
			Devices []map[string]any `json:"devices"`
		}
		if err := b.request(ctx, http.MethodGet, "/api/integration/v1/devices", nil, &response); err != nil {
			return nil, err
		}
		query := strings.ToLower(strings.TrimSpace(stringArgument(arguments, "query")))
		onlineOnly, _ := arguments["onlineOnly"].(bool)
		filtered := make([]map[string]any, 0, len(response.Devices))
		for _, device := range response.Devices {
			online, _ := device["online"].(bool)
			if onlineOnly && !online {
				continue
			}
			if query != "" {
				haystack := strings.ToLower(fmt.Sprint(device["name"], " ", device["hostname"], " ", device["remoteId"], " ", device["publicIp"], " ", device["localIps"]))
				if !strings.Contains(haystack, query) {
					continue
				}
			}
			filtered = append(filtered, device)
		}
		return map[string]any{"devices": filtered, "count": len(filtered)}, nil
	case "remoteit_get_device":
		deviceID, err := requiredString(arguments, "deviceId")
		if err != nil {
			return nil, err
		}
		var response map[string]any
		if err := b.request(ctx, http.MethodGet, "/api/integration/v1/devices/"+url.PathEscape(deviceID), nil, &response); err != nil {
			return nil, err
		}
		return response, nil
	case "remoteit_plan_action", "remoteit_create_action":
		deviceID, err := requiredString(arguments, "deviceId")
		if err != nil {
			return nil, err
		}
		action, err := requiredString(arguments, "action")
		if err != nil {
			return nil, err
		}
		parameters, _ := arguments["parameters"].(map[string]any)
		body := map[string]any{"deviceId": deviceID, "action": action, "parameters": parameters}
		path := "/api/integration/v1/actions/plan"
		if name == "remoteit_create_action" {
			idempotencyKey, err := requiredString(arguments, "idempotencyKey")
			if err != nil {
				return nil, errors.New("idempotencyKey is required; generate a fresh stable key for this intended action")
			}
			body["idempotencyKey"] = idempotencyKey
			path = "/api/integration/v1/actions"
		}
		var response map[string]any
		if err := b.request(ctx, http.MethodPost, path, body, &response); err != nil {
			return nil, err
		}
		return response, nil
	case "remoteit_get_action", "remoteit_cancel_action":
		actionID, err := requiredString(arguments, "actionId")
		if err != nil {
			return nil, err
		}
		method := http.MethodGet
		path := "/api/integration/v1/actions/" + url.PathEscape(actionID)
		if name == "remoteit_cancel_action" {
			method = http.MethodPost
			path += "/cancel"
		}
		var response map[string]any
		if err := b.request(ctx, method, path, nil, &response); err != nil {
			return nil, err
		}
		return response, nil
	default:
		return nil, fmt.Errorf("unknown RemoteIt tool %q", name)
	}
}

func (b *bridge) request(ctx context.Context, method, path string, body any, output any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}
	request, err := http.NewRequestWithContext(ctx, method, b.baseURL+path, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+b.token)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := b.client.Do(request)
	if err != nil {
		return fmt.Errorf("RemoteIt request failed: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var problem struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &problem)
		if problem.Error == "" {
			problem.Error = strings.TrimSpace(string(data))
		}
		return fmt.Errorf("RemoteIt HTTP %d: %s", response.StatusCode, problem.Error)
	}
	if output == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("RemoteIt returned invalid JSON: %w", err)
	}
	return nil
}

func copyMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func requiredString(arguments map[string]any, key string) (string, error) {
	value := strings.TrimSpace(stringArgument(arguments, key))
	if value == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return value, nil
}

func stringArgument(arguments map[string]any, key string) string {
	value, _ := arguments[key].(string)
	return value
}

func sortedToolNames() []string {
	tools := remoteItTools()
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	return names
}
