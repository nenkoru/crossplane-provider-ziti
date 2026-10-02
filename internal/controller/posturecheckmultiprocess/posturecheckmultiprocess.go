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

// Package posturecheckmultiprocess maps the PostureCheckMultiProcess managed
// resource to Ziti posture checks of type PROCESS_MULTI.
package posturecheckmultiprocess

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"

	"github.com/crossplane/provider-ziti/apis/v1alpha1"
	"github.com/crossplane/provider-ziti/internal/client"
	"github.com/crossplane/provider-ziti/internal/controller/generic"
	"github.com/crossplane/provider-ziti/internal/controller/posturecheck"
)

// Kind describes how a PostureCheckMultiProcess maps to the Ziti API.
var Kind = generic.Kind[*v1alpha1.PostureCheckMultiProcess]{
	GVK:        v1alpha1.PostureCheckMultiProcessGroupVersionKind,
	List:       &v1alpha1.PostureCheckMultiProcessList{},
	Collection: posturecheck.Collection,
	Desired:    desired,
	Observe: func(mg *v1alpha1.PostureCheckMultiProcess, entity json.RawMessage) error {
		return generic.Unmarshal(entity, &mg.Status.AtProvider)
	},
}

func desired(_ context.Context, _ *client.Client, mg *v1alpha1.PostureCheckMultiProcess) (map[string]any, error) {
	p := mg.Spec.ForProvider

	// Ziti stores the processes under the key below and returns them in its
	// order, whatever order they were sent in.
	key := func(process v1alpha1.MultiProcess) string { return process.OsType + "-" + process.Path }
	sorted := slices.SortedFunc(slices.Values(p.Processes), func(a, b v1alpha1.MultiProcess) int {
		return cmp.Compare(key(a), key(b))
	})

	processes := make([]map[string]any, 0, len(sorted))
	for _, process := range sorted {
		processes = append(processes, map[string]any{
			"osType":             process.OsType,
			"path":               process.Path,
			"hashes":             posturecheck.HexSet(process.Hashes),
			"signerFingerprints": posturecheck.HexSet(process.SignerFingerprints),
		})
	}

	return map[string]any{
		"name": p.Name,
		// The type discriminates posture checks and is required on update too.
		"typeId":         "PROCESS_MULTI",
		"roleAttributes": generic.Strings(p.RoleAttributes),
		"semantic":       cmp.Or(p.Semantic, "AllOf"),
		"processes":      processes,
		"tags":           generic.Tags(p.Tags),
	}, nil
}
