package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// client manages JSON-RPC communication with a single MCP server (stdio or HTTP).
type client struct {
	name       string
	cfg        ServerConfig
	httpClient *http.Client

	// Stdio fields (non-nil when command-based)
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader

	mu     sync.Mutex
	nextID int64
	ready  atomic.Bool
}

func newClient(name string, cfg ServerConfig) *client {
	return &client{
		name: name,
		cfg:  cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// connect starts the MCP server subprocess (or prepares HTTP) and initializes.
func (c *client) connect(ctx context.Context) error {
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

func (c *client) connectStdio(ctx context.Context) error {
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

// listTools returns the tools exposed by this MCP server.
func (c *client) listTools(ctx context.Context) ([]map[string]any, error) {
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

// callTool invokes a named tool on the MCP server with the given arguments.
func (c *client) callTool(ctx context.Context, toolName string, args map[string]any) (string, error) {
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

func (c *client) initialize(ctx context.Context) error {
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

func (c *client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if c.cfg.URL != "" {
		return c.callHTTP(ctx, method, params)
	}
	return c.callStdio(ctx, method, params)
}

func (c *client) nextRequestID() int64 {
	return atomic.AddInt64(&c.nextID, 1)
}

func (c *client) callStdio(ctx context.Context, method string, params any) (json.RawMessage, error) {
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

func (c *client) callHTTP(ctx context.Context, method string, params any) (json.RawMessage, error) {
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
