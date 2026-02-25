package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

)

// MCPServerConfig is the configuration for a single MCP server.
// Mirrors config.MCPServerConfig — defined here without importing config
// to keep the dependency graph clean.
type MCPServerConfig struct {
	Command string
	Args    []string
	Env     map[string]string
	URL     string
	Headers map[string]string
}

// ---------------------------------------------------------------------------
// MCPToolWrapper — wraps one discovered MCP tool as a Tool
// ---------------------------------------------------------------------------

// MCPToolWrapper wraps a single tool discovered from an MCP server.
type MCPToolWrapper struct {
	client      *MCPClient
	name        string
	origName    string
	description string
	parameters  json.RawMessage
}

func (w *MCPToolWrapper) Name() string                { return w.name }
func (w *MCPToolWrapper) Description() string         { return w.description }
func (w *MCPToolWrapper) Parameters() json.RawMessage { return w.parameters }

func (w *MCPToolWrapper) Execute(ctx context.Context, params map[string]any) (string, error) {
	return w.client.CallTool(ctx, w.origName, params)
}

// ---------------------------------------------------------------------------
// MCPClient — manages a connection to one MCP server (stdio or HTTP)
// ---------------------------------------------------------------------------

// MCPClient handles JSON-RPC communication with a single MCP server.
type MCPClient struct {
	name       string
	cfg        MCPServerConfig
	httpClient *http.Client

	// Stdio fields (non-nil when command-based)
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader

	mu     sync.Mutex
	nextID int64
	ready  atomic.Bool
}

func newMCPClient(name string, cfg MCPServerConfig) *MCPClient {
	return &MCPClient{
		name: name,
		cfg:  cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Connect starts the MCP server subprocess (or prepares HTTP) and initializes.
func (c *MCPClient) Connect(ctx context.Context) error {
	if c.cfg.Command != "" {
		return c.connectStdio(ctx)
	}
	if c.cfg.URL != "" {
		// HTTP MCP: no persistent connection needed; just mark ready.
		c.ready.Store(true)
		return nil
	}
	return fmt.Errorf("MCP server %q: no command or url configured", c.name)
}

func (c *MCPClient) connectStdio(ctx context.Context) error {
	args := c.cfg.Args
	c.cmd = exec.CommandContext(ctx, c.cfg.Command, args...)
	if c.cfg.Env != nil {
		for k, v := range c.cfg.Env {
			c.cmd.Env = append(c.cmd.Env, k+"="+v)
		}
	}

	stdinPipe, err := c.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdoutPipe, err := c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	c.stdin = stdinPipe
	c.stdout = bufio.NewReader(stdoutPipe)

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("start MCP server: %w", err)
	}

	// Initialize: send JSON-RPC initialize request.
	if err := c.initialize(ctx); err != nil {
		c.cmd.Process.Kill() //nolint:errcheck
		return fmt.Errorf("initialize: %w", err)
	}
	c.ready.Store(true)
	return nil
}

// ListTools returns the tools exposed by this MCP server.
func (c *MCPClient) ListTools(ctx context.Context) ([]map[string]any, error) {
	resp, err := c.call(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}
	var result struct {
		Tools []map[string]any `json:"tools"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, err
	}
	return result.Tools, nil
}

// CallTool invokes a named tool on the MCP server with the given arguments.
func (c *MCPClient) CallTool(ctx context.Context, toolName string, args map[string]any) (string, error) {
	payload := map[string]any{
		"name":      toolName,
		"arguments": args,
	}
	resp, err := c.call(ctx, "tools/call", payload)
	if err != nil {
		return "", err
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		return string(resp), nil
	}

	var parts []string
	for _, block := range result.Content {
		if block.Text != "" {
			parts = append(parts, block.Text)
		}
	}

	out := strings.Join(parts, "\n")
	if out == "" {
		out = "(no output)"
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// JSON-RPC plumbing
// ---------------------------------------------------------------------------

func (c *MCPClient) initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "crystaldolphin", "version": "1.0"},
	}
	_, err := c.call(ctx, "initialize", params)
	if err != nil {
		return err
	}
	// Send initialized notification (no response expected)
	notif := map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}
	data, _ := json.Marshal(notif)
	_, _ = fmt.Fprintf(c.stdin, "%s\n", data)
	return nil
}

func (c *MCPClient) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if c.cfg.URL != "" {
		return c.callHTTP(ctx, method, params)
	}
	return c.callStdio(ctx, method, params)
}

func (c *MCPClient) nextRequestID() int64 {
	return atomic.AddInt64(&c.nextID, 1)
}

func (c *MCPClient) callStdio(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.nextRequestID()
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		req["params"] = params
	}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, err := fmt.Fprintf(c.stdin, "%s\n", data); err != nil {
		return nil, fmt.Errorf("write to MCP stdin: %w", err)
	}

	// Read response lines until we get one with our id.
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		line, err := c.stdout.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read MCP stdout: %w", err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var resp map[string]any
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			continue // skip non-JSON lines (server log output)
		}
		// Check ID match.
		respID, _ := resp["id"]
		switch v := respID.(type) {
		case float64:
			if int64(v) != id {
				continue
			}
		case int64:
			if v != id {
				continue
			}
		default:
			continue
		}
		if errObj, ok := resp["error"]; ok {
			return nil, fmt.Errorf("MCP error: %v", errObj)
		}
		result, _ := json.Marshal(resp["result"])
		return json.RawMessage(result), nil
	}
}

func (c *MCPClient) callHTTP(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.nextRequestID()
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		req["params"] = params
	}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.URL, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range c.cfg.Headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var rpcResp map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, err
	}
	if errObj, ok := rpcResp["error"]; ok {
		return nil, fmt.Errorf("MCP error: %v", errObj)
	}
	result, _ := json.Marshal(rpcResp["result"])
	return json.RawMessage(result), nil
}

// ---------------------------------------------------------------------------
// ConnectMCPServers — top-level connection helper
// ---------------------------------------------------------------------------

// ConnectMCPServers connects to all configured MCP servers and registers
// their tools into the given Registry. Non-fatal: failed servers are logged
// and skipped. Returns a cleanup function that stops all subprocess servers.
func ConnectMCPServers(ctx context.Context, servers map[string]MCPServerConfig, availTools *ToolList) func() {
	var clients []*MCPClient

	for name, cfg := range servers {
		client := newMCPClient(name, cfg)
		if err := client.Connect(ctx); err != nil {
			slog.Error("MCP server connect failed", "server", name, "err", err)
			continue
		}

		tools, err := client.ListTools(ctx)
		if err != nil {
			slog.Error("MCP server list_tools failed", "server", name, "err", err)
			continue
		}

		for _, toolDef := range tools {
			toolName, _ := toolDef["name"].(string)
			if toolName == "" {
				continue
			}
			desc, _ := toolDef["description"].(string)
			inputSchema, _ := toolDef["inputSchema"].(map[string]any)
			if inputSchema == nil {
				inputSchema = map[string]any{"type": "object", "properties": map[string]any{}}
			}

			schemaBytes, _ := json.Marshal(inputSchema)

			wrapper := &MCPToolWrapper{
				client:      client,
				name:        "mcp_" + name + "_" + toolName,
				origName:    toolName,
				description: desc,
				parameters:  json.RawMessage(schemaBytes),
			}

			availTools.Add(wrapper)

			slog.Debug("MCP tool registered", "server", name, "tool", wrapper.name)
		}
		slog.Info("MCP server connected", "server", name, "tools", len(tools))
		clients = append(clients, client)
	}

	return func() {
		for _, c := range clients {
			if c.cmd != nil && c.cmd.Process != nil {
				c.cmd.Process.Kill() //nolint:errcheck
			}
		}
	}
}
