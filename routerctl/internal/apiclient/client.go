// Package apiclient talks to a running daemon's HTTP API (sink/api), for
// `routerctl --via`: the CLI then uses the daemon's device session
// instead of logging in itself (single-session devices allow one).
package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/eduardpaul/routerctl/routerctl/internal/service/commands"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/inventory"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/view"
	"github.com/eduardpaul/routerctl/routerctl/internal/sink/api"
)

// Client is an API client.
type Client struct {
	base  string
	token string
	http  *http.Client
}

// New returns a client for base ("http://host:9100").
func New(base, token string) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("--via: %q is not an http(s) URL", base)
	}
	return &Client{base: u.String(), token: token, http: &http.Client{Timeout: 4 * time.Minute}}, nil
}

// APIError is an error answer of the API.
type APIError struct {
	Status int
	Err    commands.Error
}

func (e *APIError) Error() string { return fmt.Sprintf("%s (%s)", e.Err.Message, e.Err.Code) }

func (c *Client) call(ctx context.Context, method, path string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+api.Prefix+path, rd)
	if err != nil {
		return err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		var eb api.ErrorBody
		if json.Unmarshal(data, &eb) == nil && eb.Error.Code != "" {
			if out != nil {
				// Some errors carry a full body (a commands.Result)
				_ = json.Unmarshal(data, out)
			}
			return &APIError{Status: resp.StatusCode, Err: eb.Error}
		}
		return fmt.Errorf("%s %s: HTTP %d", method, path, resp.StatusCode)
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// Info is GET /info.
func (c *Client) Info(ctx context.Context) (api.Info, error) {
	var out api.Info
	return out, c.call(ctx, "GET", "/info", nil, &out)
}

// Clients is GET /devices/{dev}/clients.
func (c *Client) Clients(ctx context.Context, dev string) ([]view.Client, error) {
	var out []view.Client
	return out, c.call(ctx, "GET", "/devices/"+url.PathEscape(dev)+"/clients", nil, &out)
}

// Actions is GET /devices/{dev}/actions.
func (c *Client) Actions(ctx context.Context, dev string) ([]commands.Action, error) {
	var out []commands.Action
	return out, c.call(ctx, "GET", "/devices/"+url.PathEscape(dev)+"/actions", nil, &out)
}

// Do is POST /devices/{dev}/actions/{action}. A refused or failed action
// is a Result with Error set, not an error.
func (c *Client) Do(ctx context.Context, dev, action string, args map[string]string) (commands.Result, error) {
	var out commands.Result
	err := c.call(ctx, "POST", "/devices/"+url.PathEscape(dev)+"/actions/"+url.PathEscape(action), args, &out)
	if ae, ok := err.(*APIError); ok {
		if out.Error == nil {
			e := ae.Err
			out.Device, out.Action, out.Error = dev, action, &e
		}
		return out, nil
	}
	return out, err
}

// Inventory is GET /inventory.
func (c *Client) Inventory(ctx context.Context, unknownOnly bool) ([]inventory.Entry, error) {
	var out []inventory.Entry
	q := ""
	if unknownOnly {
		q = "?known=false"
	}
	return out, c.call(ctx, "GET", "/inventory"+q, nil, &out)
}

// Patch is PATCH /inventory/{mac}.
func (c *Client) Patch(ctx context.Context, mac string, p api.InventoryPatch) (inventory.Entry, error) {
	var out inventory.Entry
	return out, c.call(ctx, "PATCH", "/inventory/"+url.PathEscape(mac), p, &out)
}

// Forget is DELETE /inventory/{mac}.
func (c *Client) Forget(ctx context.Context, mac string) error {
	return c.call(ctx, "DELETE", "/inventory/"+url.PathEscape(mac), nil, nil)
}
