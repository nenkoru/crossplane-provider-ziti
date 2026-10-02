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
	"encoding/json"
	"fmt"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// ZitiObject represents a standard Ziti resource with common fields.
type ZitiObject struct {
	ID             string                 `json:"id"`
	Name           string                 `json:"name"`
	LastUpdatedBy  string                 `json:"lastUpdatedBy"`
	LastUpdated    string                 `json:"lastUpdated"`
	Tags           map[string]interface{} `json:"tags,omitempty"`
	RoleAttributes []string               `json:"roleAttributes,omitempty"`
}

// ParseID extracts the resource ID from a Ziti API response.
func ParseID(body []byte) (string, error) {
	var resp Response
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return "", fmt.Errorf("failed to unmarshal data: %w", err)
	}

	id, ok := data["id"].(string)
	if !ok || id == "" {
		return "", fmt.Errorf("no id found in response data")
	}

	return id, nil
}

// ParseLastUpdated extracts the lastUpdated timestamp from a Ziti API response.
func ParseLastUpdated(body []byte) (string, error) {
	var resp Response
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return "", fmt.Errorf("failed to unmarshal data: %w", err)
	}

	lastUpdated, _ := data["lastUpdated"].(string)
	return lastUpdated, nil
}

// ParseZitiObject parses a Ziti API response into a ZitiObject.
func ParseZitiObject(body []byte) (*ZitiObject, error) {
	var resp Response
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	var obj ZitiObject
	if err := json.Unmarshal(resp.Data, &obj); err != nil {
		return nil, fmt.Errorf("failed to unmarshal object: %w", err)
	}

	return &obj, nil
}

// ParseList extracts a list of IDs from a Ziti API list response.
func ParseList(body []byte) ([]string, error) {
	var resp Response
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	var items []map[string]interface{}
	if err := json.Unmarshal(resp.Data, &items); err != nil {
		return nil, fmt.Errorf("failed to unmarshal list: %w", err)
	}

	ids := make([]string, 0, len(items))
	for _, item := range items {
		if id, ok := item["id"].(string); ok {
			ids = append(ids, id)
		}
	}

	return ids, nil
}

// ParseListObjects parses a Ziti API list response into a slice of objects.
func ParseListObjects[T any](body []byte) ([]T, error) {
	var resp Response
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	var items []T
	if err := json.Unmarshal(resp.Data, &items); err != nil {
		return nil, fmt.Errorf("failed to unmarshal list items: %w", err)
	}

	return items, nil
}

// MgmtPath builds a URL path for a Ziti management API resource.
// Example: MgmtPath("service", "abc123") → "/edge/management/v1/services/abc123"
func MgmtPath(resource, id string) string {
	if id != "" {
		return fmt.Sprintf("/edge/management/v1/%ss/%s", resource, id)
	}
	return fmt.Sprintf("/edge/management/v1/%ss", resource)
}

// MgmtPathCustom builds a custom management API path.
// Example: MgmtPathCustom("configs/host.v1", "") → "/edge/management/v1/configs/host.v1"
func MgmtPathCustom(path string) string {
	return "/edge/management/v1/" + path
}

// ResourcePath builds a URL path for a Ziti resource (deprecated, use MgmtPath).
func ResourcePath(resource, id string) string {
	return MgmtPath(resource, id)
}

// IsNotFound returns true if the error is a "not found" error.
func IsNotFound(err error) bool {
	return apierrors.IsNotFound(err)
}
