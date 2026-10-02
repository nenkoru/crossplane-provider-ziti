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

package generic

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/crossplane/provider-ziti/internal/client"
)

// Unmarshal replaces the supplied observation with the Ziti entity.
func Unmarshal[O any](entity json.RawMessage, observation *O) error {
	var o O
	if err := json.Unmarshal(entity, &o); err != nil {
		return err
	}
	*observation = o
	return nil
}

// UnmarshalConfig replaces the supplied observation with a Ziti config, whose
// type specific settings are nested in its data field.
func UnmarshalConfig[O any](entity json.RawMessage, observation *O) error {
	var config struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(entity, &config); err != nil {
		return err
	}

	var o O
	if len(config.Data) > 0 {
		if err := json.Unmarshal(config.Data, &o); err != nil {
			return err
		}
	}
	// The common fields win over type specific settings of the same name.
	if err := json.Unmarshal(entity, &o); err != nil {
		return err
	}
	*observation = o
	return nil
}

// Strings returns the supplied list, or an empty list if it is nil, so that
// it is sent as [] and clears the field.
func Strings(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
}

// Tags returns the supplied tags, or empty tags if they are nil, so that they
// are sent as {} and clear the field.
func Tags(t map[string]string) map[string]string {
	if t == nil {
		return map[string]string{}
	}
	return t
}

// ResolveIDs resolves a list of names or IDs of entities to their IDs.
func ResolveIDs(ctx context.Context, api *client.Client, collection string, namesOrIDs []string) ([]string, error) {
	ids := make([]string, 0, len(namesOrIDs))
	for _, n := range namesOrIDs {
		id, err := api.ResolveID(ctx, collection, n)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// ResolveRoles resolves the "@name" role selectors of a policy to "@id".
// Attribute selectors such as "#all" are returned as is.
func ResolveRoles(ctx context.Context, api *client.Client, collection string, roles []string) ([]string, error) {
	out := make([]string, 0, len(roles))
	for _, role := range roles {
		ref, ok := strings.CutPrefix(role, "@")
		if !ok {
			out = append(out, role)
			continue
		}

		id, err := api.ResolveID(ctx, collection, ref)
		if err != nil {
			return nil, err
		}
		out = append(out, "@"+id)
	}
	return out, nil
}

// ConfigType returns the create-only field that sets the type of a config.
func ConfigType(ctx context.Context, api *client.Client, name string) (map[string]any, error) {
	id, err := api.ResolveID(ctx, "config-types", name)
	if err != nil {
		return nil, err
	}
	return map[string]any{"configTypeId": id}, nil
}

// ConfigData returns the type specific settings of a config: the parameters
// of its managed resource without the fields common to all configs.
func ConfigData(parameters any) (map[string]any, error) {
	raw, err := json.Marshal(parameters)
	if err != nil {
		return nil, err
	}

	data := map[string]any{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	delete(data, "name")
	delete(data, "tags")
	return data, nil
}
