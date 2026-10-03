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

package apis_test

import (
	"testing"

	"k8s.io/apiextensions-apiserver/pkg/apiserver/validation"

	"github.com/crossplane/provider-ziti/apis/v1alpha1"
)

// TestExpectedStatusOfHTTPChecks checks the bounds of the expected status of
// an HTTP check of host.v1 and host.v2 configs, which are those of the
// config schemas of Ziti.
func TestExpectedStatusOfHTTPChecks(t *testing.T) {
	validators := crdValidators(t)

	check := func(status int64) map[string]any {
		return map[string]any{
			"url": "http://localhost:8080/health", "method": "GET", "interval": "10s", "timeout": "5s", "expectStatus": status,
			"actions": []any{map[string]any{"trigger": "fail", "action": "mark unhealthy", "duration": "60s"}},
		}
	}
	manifests := map[string]func(status int64) map[string]any{
		"ConfigHostV1": func(status int64) map[string]any {
			return map[string]any{"name": "web-host", "address": "localhost", "port": int64(8080), "protocol": "tcp", "httpChecks": []any{check(status)}}
		},
		"ConfigHostV2": func(status int64) map[string]any {
			return map[string]any{"name": "web-hosts", "terminators": []any{map[string]any{
				"address": "localhost", "port": int64(8080), "protocol": "tcp", "httpChecks": []any{check(status)},
			}}}
		},
	}

	for kind, forProvider := range manifests {
		for status, valid := range map[int64]bool{99: false, 100: true, 200: true, 599: true, 600: false} {
			u := map[string]any{
				"apiVersion": v1alpha1.Group + "/" + v1alpha1.Version,
				"kind":       kind,
				"metadata":   map[string]any{"name": "web", "namespace": "default"},
				"spec":       map[string]any{"forProvider": forProvider(status)},
			}
			errs := validation.ValidateCustomResource(nil, u, validators[kind])
			if got := len(errs) == 0; got != valid {
				t.Errorf("%s with expectStatus %d: want valid %t, got %v", kind, status, valid, errs.ToAggregate())
			}
		}
	}
}
