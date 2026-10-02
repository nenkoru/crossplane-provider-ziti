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

package client

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// authResponse represents the JSON response from Ziti authentication.
type authResponse struct {
	Data struct {
		Token string `json:"token"`
	} `json:"data"`
}

// authenticatePassword authenticates using username/password and returns the session token.
func authenticatePassword(host, username, password string) (string, error) {
	payload := map[string]string{
		"username": username,
		"password": password,
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal auth payload: %w", err)
	}

	authURL := fmt.Sprintf("%s/authenticate?method=password", host)
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
	}

	req, err := http.NewRequest("POST", authURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to build auth request: %w", err)
	}
	req.Header.Add("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("auth request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read auth response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("auth failed with status %d: %s", resp.StatusCode, string(body))
	}

	var authResp authResponse
	if err := json.Unmarshal(body, &authResp); err != nil {
		return "", fmt.Errorf("failed to parse auth response: %w", err)
	}

	if authResp.Data.Token == "" {
		return "", errors.New("no token returned in auth response")
	}

	return authResp.Data.Token, nil
}

// authenticateCert authenticates using mTLS client certificate and returns the session token.
func authenticateCert(host, certPEM, keyPEM, caPEM string) (string, error) {
	clientCert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		return "", fmt.Errorf("failed to parse client certificate/key: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates:       []tls.Certificate{clientCert},
		InsecureSkipVerify: true,
	}

	if caPEM != "" {
		pool := x509.NewCertPool()
		if ok := pool.AppendCertsFromPEM([]byte(caPEM)); ok {
			tlsConfig.RootCAs = pool
			tlsConfig.InsecureSkipVerify = false
		}
	}

	transport := &http.Transport{TLSClientConfig: tlsConfig}
	httpClient := &http.Client{Transport: transport, Timeout: 15 * time.Second}

	authURL := fmt.Sprintf("%s/authenticate?method=cert", host)
	req, err := http.NewRequest("POST", authURL, bytes.NewBufferString("{}"))
	if err != nil {
		return "", fmt.Errorf("failed to build cert auth request: %w", err)
	}
	req.Header.Add("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("cert auth request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read cert auth response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("cert auth failed with status %d: %s", resp.StatusCode, string(body))
	}

	var authResp authResponse
	if err := json.Unmarshal(body, &authResp); err != nil {
		return "", fmt.Errorf("failed to parse cert auth response: %w", err)
	}

	if authResp.Data.Token == "" {
		return "", errors.New("no token returned in cert auth response")
	}

	return authResp.Data.Token, nil
}

// ResolveHost parses and validates the Ziti controller host URL.
func ResolveHost(host string) (*url.URL, error) {
	u, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("invalid host URL %q: %w", host, err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, fmt.Errorf("host URL must use http or https scheme: %q", host)
	}
	return u, nil
}
