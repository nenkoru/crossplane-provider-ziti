/*
Copyright 2025 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package client provides a HTTP client for the Ziti Edge Management API.
package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// managementPath is the path prefix of the Edge Management API.
	managementPath = "/edge/management/v1"

	// sessionHeader carries the API session token.
	sessionHeader = "zt-session"

	requestTimeout = 30 * time.Second
)

// Config holds the configuration for connecting to a Ziti Controller.
type Config struct {
	// Host is the Ziti Controller URL (e.g. https://controller.example.com:1280).
	// It must use https: credentials and API sessions are never sent in the
	// clear. A trailing /edge/management/v1 is accepted and ignored.
	Host string

	// Username for password-based authentication.
	Username string

	// Password for password-based authentication.
	Password string

	// Cert is the PEM-encoded client certificate for certificate authentication.
	Cert string

	// Key is the PEM-encoded private key for certificate authentication.
	Key string

	// CA is the PEM-encoded CA bundle used to verify the controller certificate.
	// The system roots are used when it is empty.
	CA string

	// InsecureSkipTLSVerify disables verification of the controller certificate.
	InsecureSkipTLSVerify bool
}

// Client is a client for the Ziti Edge Management API. It authenticates
// lazily and transparently re-authenticates when its API session expires.
type Client struct {
	base string
	cfg  Config
	http *http.Client

	mu    sync.Mutex
	token string

	// ahead is how far the clock of the controller is ahead of the local
	// one, once a response has told the time.
	ahead atomic.Pointer[time.Duration]
}

// New creates a Client from the supplied config. It does not contact the
// controller; authentication happens on the first request.
func New(cfg Config) (*Client, error) {
	base, err := baseURL(cfg.Host)
	if err != nil {
		return nil, err
	}

	usesCert := cfg.Cert != "" || cfg.Key != ""
	if !usesCert && (cfg.Username == "" || cfg.Password == "") {
		return nil, errors.New("either username and password or cert and key are required")
	}

	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: cfg.InsecureSkipTLSVerify, //nolint:gosec // Only disabled when the ProviderConfig explicitly asks for it.
	}

	if cfg.CA != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(cfg.CA)) {
			return nil, errors.New("ca does not contain a PEM-encoded certificate")
		}
		tlsConfig.RootCAs = pool
	}

	if usesCert {
		pair, err := tls.X509KeyPair([]byte(cfg.Cert), []byte(cfg.Key))
		if err != nil {
			return nil, fmt.Errorf("cannot parse client certificate and key: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{pair}
	}

	return &Client{
		base: base,
		cfg:  cfg,
		http: &http.Client{
			Transport: &http.Transport{
				Proxy:           http.ProxyFromEnvironment,
				TLSClientConfig: tlsConfig,
			},
			Timeout: requestTimeout,
		},
	}, nil
}

// baseURL validates the controller URL and returns the management API root.
func baseURL(host string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(host))
	if err != nil {
		return "", fmt.Errorf("invalid host URL: %w", err)
	}
	if u.Scheme != "https" || u.Host == "" {
		return "", fmt.Errorf("host must be an https URL, got %q", host)
	}

	path := strings.TrimSuffix(strings.TrimRight(u.Path, "/"), managementPath)
	return u.Scheme + "://" + u.Host + strings.TrimRight(path, "/") + managementPath, nil
}

// Get returns the entity with the supplied ID.
func (c *Client) Get(ctx context.Context, collection, id string) (json.RawMessage, error) {
	return c.do(ctx, http.MethodGet, entityPath(collection, id), nil)
}

// Find returns the entities of a collection matching the supplied filter.
// Only the first page of results is returned.
func (c *Client) Find(ctx context.Context, collection, filter string) ([]json.RawMessage, error) {
	data, err := c.do(ctx, http.MethodGet, "/"+collection+"?filter="+url.QueryEscape(filter), nil)
	if err != nil {
		return nil, err
	}

	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("cannot parse %s list: %w", collection, err)
	}
	return items, nil
}

// Create creates an entity and returns its ID.
func (c *Client) Create(ctx context.Context, collection string, body any) (string, error) {
	data, err := c.do(ctx, http.MethodPost, "/"+collection, body)
	if err != nil {
		return "", err
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &created); err != nil {
		return "", fmt.Errorf("cannot parse create response: %w", err)
	}
	if created.ID == "" {
		return "", errors.New("create response does not contain an id")
	}
	return created.ID, nil
}

// Patch updates the supplied fields of an entity.
func (c *Client) Patch(ctx context.Context, collection, id string, body any) error {
	_, err := c.do(ctx, http.MethodPatch, entityPath(collection, id), body)
	return err
}

// Put replaces an entity: fields that are left out of the body are reset to
// their defaults.
func (c *Client) Put(ctx context.Context, collection, id string, body any) error {
	_, err := c.do(ctx, http.MethodPut, entityPath(collection, id), body)
	return err
}

// Delete deletes an entity.
func (c *Client) Delete(ctx context.Context, collection, id string) error {
	_, err := c.do(ctx, http.MethodDelete, entityPath(collection, id), nil)
	return err
}

// Act asks the controller to carry out an action on an entity, such as
// "refresh" on an enrollment. An action that takes no settings has a nil body.
func (c *Client) Act(ctx context.Context, collection, id, action string, body any) error {
	_, err := c.do(ctx, http.MethodPost, entityPath(collection, id)+"/"+action, body)
	return err
}

// FindByName returns the entity with the supplied name, which is unique
// within a collection, or nil if there is no such entity.
func (c *Client) FindByName(ctx context.Context, collection, name string) (json.RawMessage, error) {
	items, err := c.Find(ctx, collection, "name="+quote(name))
	if err != nil || len(items) == 0 {
		return nil, err
	}
	return items[0], nil
}

// ResolveID returns the ID of the entity with the supplied name. A value that
// does not match a name is returned as is if an entity with that ID exists.
func (c *Client) ResolveID(ctx context.Context, collection, nameOrID string) (string, error) {
	named, err := c.FindByName(ctx, collection, nameOrID)
	if err != nil {
		return "", err
	}

	if named != nil {
		var item struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(named, &item); err != nil {
			return "", fmt.Errorf("cannot parse %s list: %w", collection, err)
		}
		return item.ID, nil
	}

	if _, err := c.Get(ctx, collection, nameOrID); err != nil {
		if IsNotFound(err) {
			return "", fmt.Errorf("no entity named %q in %s", nameOrID, collection)
		}
		return "", err
	}
	return nameOrID, nil
}

func entityPath(collection, id string) string {
	return "/" + collection + "/" + url.PathEscape(id)
}

// quote returns s as a string literal of the Ziti filter language.
func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// do sends an authenticated request and returns the data field of the
// response envelope. An expired API session is renewed once.
func (c *Client) do(ctx context.Context, method, path string, body any) (json.RawMessage, error) {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return nil, fmt.Errorf("cannot encode request body: %w", err)
		}
	}

	token, err := c.session(ctx, "")
	if err != nil {
		return nil, err
	}

	data, err := c.send(ctx, method, path, token, payload)
	if !isUnauthorized(err) {
		return data, err
	}

	if token, err = c.session(ctx, token); err != nil {
		return nil, err
	}
	return c.send(ctx, method, path, token, payload)
}

// session returns the API session token, authenticating if there is none or
// if the current one is the supplied stale token.
func (c *Client) session(ctx context.Context, stale string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && c.token != stale {
		return c.token, nil
	}

	token, err := c.authenticate(ctx)
	if err != nil {
		c.token = ""
		return "", err
	}

	c.token = token
	return token, nil
}

func (c *Client) authenticate(ctx context.Context) (string, error) {
	method, credentials := "password", map[string]string{"username": c.cfg.Username, "password": c.cfg.Password}
	if c.cfg.Cert != "" {
		method, credentials = "cert", map[string]string{}
	}

	payload, err := json.Marshal(credentials)
	if err != nil {
		return "", fmt.Errorf("cannot encode credentials: %w", err)
	}

	data, err := c.send(ctx, http.MethodPost, "/authenticate?method="+method, "", payload)
	if err != nil {
		// The cause may echo the request, never include it for authentication.
		var apiErr *Error
		if errors.As(err, &apiErr) {
			return "", fmt.Errorf("cannot authenticate to the Ziti controller: %d %s", apiErr.StatusCode, apiErr.Code)
		}
		return "", fmt.Errorf("cannot authenticate to the Ziti controller: %w", err)
	}

	var session struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(data, &session); err != nil {
		return "", fmt.Errorf("cannot parse authentication response: %w", err)
	}
	if session.Token == "" {
		return "", errors.New("authentication response does not contain a token")
	}
	return session.Token, nil
}

func (c *Client) send(ctx context.Context, method, path, token string, payload []byte) (json.RawMessage, error) {
	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set(sessionHeader, token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck // Nothing useful to do with a close error on a read body.
	c.observeClock(resp.Header.Get("Date"))

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cannot read response: %w", err)
	}
	return data(resp.StatusCode, raw)
}

// observeClock compares the time a response was sent at with the local clock.
func (c *Client) observeClock(date string) {
	sent, err := http.ParseTime(date)
	if err != nil {
		return
	}

	ahead := time.Until(sent)
	c.ahead.Store(&ahead)
}

// ClockAhead returns how far the clock of the controller is ahead of the
// local clock, to the second, going by the latest response. It is negative
// if the clock of the controller is behind. The second value is false if no
// response has told the time yet.
func (c *Client) ClockAhead() (time.Duration, bool) {
	ahead := c.ahead.Load()
	if ahead == nil {
		return 0, false
	}
	return *ahead, true
}

// data returns the data field of a response envelope, or the error the
// response describes.
func data(status int, raw []byte) (json.RawMessage, error) {
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return nil, newError(status, raw)
	}

	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}

	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("cannot parse response: %w", err)
	}
	return envelope.Data, nil
}
