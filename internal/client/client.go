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

// Package client provides a HTTP client for the Ziti Edge Controller API.
package client

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Config holds the configuration for connecting to a Ziti Controller.
type Config struct {
	// Host is the Ziti Controller URL (e.g. https://controller.example.com:441).
	Host string

	// Username for password-based authentication.
	Username string

	// Password for password-based authentication.
	Password string

	// Cert is the PEM-encoded client certificate for mTLS authentication.
	Cert string

	// Key is the PEM-encoded private key for mTLS authentication.
	Key string

	// CA is the PEM-encoded CA certificate for server verification.
	CA string
}

// ZitiClient is a client for the Ziti Edge Controller REST API.
type ZitiClient struct {
	host         string
	httpClient   *http.Client
	sessionToken string
	mu           sync.RWMutex
	tokenTime    time.Time
}

// New creates a new ZitiClient from the given config.
func New(cfg Config) (*ZitiClient, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, // Ziti controllers self-sign by default
	}

	if cfg.CA != "" {
		tlsConfig.InsecureSkipVerify = false
		// CA pool setup handled in auth if needed
	}

	if cfg.Cert != "" && cfg.Key != "" {
		// Will be set per-request in auth flow
	}

	return &ZitiClient{
		host: cfg.Host,
		httpClient: &http.Client{
			Transport: &http.Transport{TLSClientConfig: tlsConfig},
			Timeout:   30 * time.Second,
		},
	}, nil
}

// Authenticate logs in to the Ziti Controller and stores the session token.
func (c *ZitiClient) Authenticate(cfg Config) error {
	var token string
	var err error

	if cfg.Cert != "" && cfg.Key != "" {
		token, err = authenticateCert(cfg.Host, cfg.Cert, cfg.Key, cfg.CA)
	} else {
		token, err = authenticatePassword(cfg.Host, cfg.Username, cfg.Password)
	}

	if err != nil {
		return err
	}

	c.mu.Lock()
	c.sessionToken = token
	c.tokenTime = time.Now()
	c.mu.Unlock()

	return nil
}

// getToken returns the current session token.
func (c *ZitiClient) getToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sessionToken
}

// doRequest performs an HTTP request to the Ziti Controller API.
func (c *ZitiClient) doRequest(method, path string, body interface{}) ([]byte, error) {
	url := c.host + path

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("zt-session", c.getToken())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusNotFound {
		// Ziti API may return 404 for validation errors too, check error body first
		var zitiErr struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
				Cause   struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"cause"`
			} `json:"error"`
		}
		if err := json.Unmarshal(respBody, &zitiErr); err == nil && zitiErr.Error.Cause.Message != "" {
			return nil, fmt.Errorf("Ziti API error (%s): %s - %s", resp.Status, zitiErr.Error.Cause.Code, zitiErr.Error.Cause.Message)
		}
		return nil, apierrors.NewNotFound(schema.GroupResource{Group: "ziti.crossplane.io", Resource: path}, "not found")
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		// Try to parse Ziti error response for better error messages
		var zitiErr struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
				Cause   struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"cause"`
			} `json:"error"`
		}
		if err := json.Unmarshal(respBody, &zitiErr); err == nil && zitiErr.Error.Cause.Message != "" {
			return nil, fmt.Errorf("Ziti API error (%s): %s - %s", resp.Status, zitiErr.Error.Cause.Code, zitiErr.Error.Cause.Message)
		}
		return nil, fmt.Errorf("Ziti API error (%s): %s", resp.Status, string(respBody))
	}

	return respBody, nil
}

// Create sends a POST request to create a Ziti resource.
func (c *ZitiClient) Create(path string, body interface{}) ([]byte, error) {
	return c.doRequest(http.MethodPost, path, body)
}

// Read sends a GET request to read a Ziti resource.
func (c *ZitiClient) Read(path string) ([]byte, error) {
	return c.doRequest(http.MethodGet, path, nil)
}

// Update sends a PATCH request to update a Ziti resource.
// Note: Ziti management API uses PATCH for updates, not PUT.
// PATCH returns empty data {"data":{}}, use Read() after to verify.
func (c *ZitiClient) Update(path string, body interface{}) ([]byte, error) {
	return c.doRequest(http.MethodPatch, path, body)
}

// Patch sends a PATCH request to update a Ziti resource.
func (c *ZitiClient) Patch(path string, body interface{}) ([]byte, error) {
	return c.doRequest(http.MethodPatch, path, body)
}

// Delete sends a DELETE request to delete a Ziti resource.
func (c *ZitiClient) Delete(path string) ([]byte, error) {
	return c.doRequest(http.MethodDelete, path, nil)
}

// CreateWithClient creates a resource using resty for retry support.
func (c *ZitiClient) CreateWithClient(path string, body interface{}) ([]byte, error) {
	client := c.newRestyClient()
	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("zt-session", c.getToken()).
		SetBody(body).
		Post(c.host + path)

	if err != nil {
		return nil, err
	}

	if resp.StatusCode() == http.StatusNotFound {
		// Ziti API may return 404 for validation errors too, check error body first
		var zitiErr struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
				Cause   struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"cause"`
			} `json:"error"`
		}
		if err := json.Unmarshal(resp.Body(), &zitiErr); err == nil && zitiErr.Error.Cause.Message != "" {
			return nil, fmt.Errorf("Ziti API error (%d): %s - %s", resp.StatusCode(), zitiErr.Error.Cause.Code, zitiErr.Error.Cause.Message)
		}
		return nil, apierrors.NewNotFound(schema.GroupResource{Group: "ziti.crossplane.io", Resource: "resources"}, "not found")
	}

	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusCreated {
		return nil, fmt.Errorf("Ziti API error (%d): %s", resp.StatusCode(), string(resp.Body()))
	}

	return resp.Body(), nil
}

// UpdateWithClient updates a resource using resty for retry support.
// Note: Ziti management API uses PATCH for updates.
func (c *ZitiClient) UpdateWithClient(path string, body interface{}) ([]byte, error) {
	client := c.newRestyClient()
	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("zt-session", c.getToken()).
		SetBody(body).
		Patch(c.host + path)

	if err != nil {
		return nil, err
	}

	if resp.StatusCode() == http.StatusNotFound {
		var zitiErr struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
				Cause   struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"cause"`
			} `json:"error"`
		}
		if err := json.Unmarshal(resp.Body(), &zitiErr); err == nil && zitiErr.Error.Cause.Message != "" {
			return nil, fmt.Errorf("Ziti API error (%d): %s - %s", resp.StatusCode(), zitiErr.Error.Cause.Code, zitiErr.Error.Cause.Message)
		}
		return nil, apierrors.NewNotFound(schema.GroupResource{Group: "ziti.crossplane.io", Resource: "resources"}, "not found")
	}

	if resp.StatusCode() != http.StatusOK && resp.StatusCode() != http.StatusCreated {
		return nil, fmt.Errorf("Ziti API error (%d): %s", resp.StatusCode(), string(resp.Body()))
	}

	return resp.Body(), nil
}

// newRestyClient creates a resty client with retry support.
func (c *ZitiClient) newRestyClient() *resty.Client {
	return resty.New().
		SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true}).
		SetTimeout(30 * time.Second).
		SetRetryCount(3).
		SetRetryWaitTime(2 * time.Second).
		SetRetryMaxWaitTime(10 * time.Second)
}

// ReadWithClient reads a resource using resty for retry support.
func (c *ZitiClient) ReadWithClient(path string) ([]byte, error) {
	client := c.newRestyClient()
	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("zt-session", c.getToken()).
		Get(c.host + path)

	if err != nil {
		return nil, err
	}

	if resp.StatusCode() == http.StatusNotFound {
		return nil, apierrors.NewNotFound(schema.GroupResource{Group: "ziti.crossplane.io", Resource: path}, "not found")
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("Ziti API error (%d): %s", resp.StatusCode(), string(resp.Body()))
	}

	return resp.Body(), nil
}

// Response wraps a Ziti API JSON response.
type Response struct {
	Data       json.RawMessage `json:"data"`
	Success    bool            `json:"success"`
	StatusCode int             `json:"statusCode"`
	Msg        string          `json:"msg"`
}

// ParseResponse parses a Ziti API JSON response and extracts the data field.
func ParseResponse(body []byte) (json.RawMessage, error) {
	var resp Response
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}
