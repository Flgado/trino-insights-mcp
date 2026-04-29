package rest

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Flgado/trino-insights-mcp/pkg/errors"
)

type Client struct {
	base      *url.URL
	http      *http.Client
	auth      Authenticator
	userAgent string
}

type Options struct {
	BaseURL               string
	Timeout               time.Duration
	InsecureSkipTLSVerify bool
	Auth                  Authenticator
	UserAgent             string
}

func NewClient(opts Options) (*Client, error) {
	if opts.BaseURL == "" {
		return nil, fmt.Errorf("rest: BaseURL is required")
	}

	u, err := url.Parse(opts.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("rest: invalid BaseURL %q: %w", opts.BaseURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("rest: BaseURL must be http or https, got %q", u.Scheme)
	}

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if opts.InsecureSkipTLSVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // operator opt-in
	}

	c := &Client{
		base:      u,
		http:      &http.Client{Timeout: timeout, Transport: transport},
		auth:      opts.Auth,
		userAgent: opts.UserAgent,
	}

	if c.userAgent == "" {
		c.userAgent = "trino-insights-mcp/0.1"
	}

	if c.auth == nil {
		c.auth = NoAuth{}
	}

	return c, nil
}

func (c *Client) BaseURL() string {
	return c.base.String()
}

func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	u := *c.base
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/" + path
	}

	u.Path = strings.TrimRight(c.base.Path, "/") + path
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}

	c.auth.Apply(req)
	return req, nil
}

func (c *Client) do(req *http.Request, op string, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return errors.NewTrinoRESTError(op, req.URL.String(), 0, "", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 299 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return errors.NewTrinoRESTError(op, req.URL.String(), resp.StatusCode, string(body), nil)
	}

	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return errors.NewTrinoRESTError(op, req.URL.String(), resp.StatusCode, "", err)
	}

	return nil
}

func (c *Client) GetJSON(ctx context.Context, path string, out any) error {
	op := "GET" + path
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return errors.NewTrinoRESTError(op, "", 0, "", err)
	}

	return c.do(req, op, out)
}

func (c *Client) PostJSON(ctx context.Context, path string, body any, out any) error {
	op := "POST" + path
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return errors.NewTrinoRESTError(op, "", 0, "", err)
		}
		rdr = bytes.NewReader(buf)
	}

	req, err := c.newRequest(ctx, http.MethodPost, path, rdr)
	if err != nil {
		return errors.NewTrinoRESTError(op, "", 0, "", err)
	}

	return c.do(req, op, out)
}
