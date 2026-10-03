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

// Package posturecheckprocess maps the PostureCheckProcess managed resource
// to Ziti posture checks of type PROCESS.
package posturecheckprocess

import (
	"context"
	"encoding/json"

	"github.com/crossplane/provider-ziti/apis/v1alpha1"
	"github.com/crossplane/provider-ziti/internal/client"
	"github.com/crossplane/provider-ziti/internal/controller/generic"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheck"
)

// Kind describes how a PostureCheckProcess maps to the Ziti API.
var Kind = generic.Kind[*v1alpha1.PostureCheckProcess]{
	GVK:        v1alpha1.PostureCheckProcessGroupVersionKind,
	List:       &v1alpha1.PostureCheckProcessList{},
	Collection: posturecheck.Collection,
	Desired:    desired,
	Observe: func(mg *v1alpha1.PostureCheckProcess, entity json.RawMessage) error {
		return generic.Unmarshal(entity, &mg.Status.AtProvider)
	},
}

func desired(_ context.Context, _ *client.Client, mg *v1alpha1.PostureCheckProcess) (map[string]any, error) {
	p := mg.Spec.ForProvider

	return map[string]any{
		"name": p.Name,
		// The type discriminates posture checks and is required on update too.
		"typeId":         "PROCESS",
		"roleAttributes": generic.Strings(p.RoleAttributes),
		// A PATCH only changes the settings of the process that it names, so
		// the hashes and the fingerprint are sent even when there are none.
		"process": map[string]any{
			"osType":            p.Process.OsType,
			"path":              p.Process.Path,
			"hashes":            posturecheck.HexSet(p.Process.Hashes),
			"signerFingerprint": posturecheck.Hex(v1alpha1.HexString(p.Process.SignerFingerprint)),
		},
		"tags": generic.Tags(p.Tags),
	}, nil
}
