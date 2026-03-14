package mcp

import (
	"context"
	"sync"
)

type Client struct {
	session   *session
	cancel    context.CancelFunc
	requestMu sync.Mutex
	closeOnce sync.Once
	closeErr  error
}

func NewClient(cfg Config) (*Client, error) {
	baseCtx, cancel := context.WithCancel(context.Background())
	s, err := startSession(baseCtx, cfg)
	if err != nil {
		cancel()
		return nil, err
	}
	return &Client{
		session: s,
		cancel:  cancel,
	}, nil
}

func (c *Client) ListTools() ([]map[string]any, error) {
	c.requestMu.Lock()
	defer c.requestMu.Unlock()

	var tools []map[string]any
	var cursor string
	for {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		var result toolsListResult
		if err := c.session.request("tools/list", params, &result); err != nil {
			return nil, err
		}
		tools = append(tools, result.Tools...)
		if result.NextCursor == "" {
			return tools, nil
		}
		cursor = result.NextCursor
	}
}

func (c *Client) CallTool(name string, arguments map[string]any) (CallResult, error) {
	c.requestMu.Lock()
	defer c.requestMu.Unlock()

	var result map[string]any
	if err := c.session.request("tools/call", map[string]any{
		"name":      name,
		"arguments": arguments,
	}, &result); err != nil {
		return CallResult{}, err
	}

	isError, _ := result["isError"].(bool)
	return CallResult{Result: result, IsError: isError}, nil
}

func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		c.cancel()
		c.closeErr = c.session.close()
	})
	return c.closeErr
}
