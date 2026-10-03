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
	"context"
	"os"
	"strings"
	"testing"

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	structuralschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/cel"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	celconfig "k8s.io/apiserver/pkg/apis/cel"
	"sigs.k8s.io/yaml"

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

// TestForwardAddressTranslations checks the rules of an address translation
// of host.v1 and host.v2 configs. The schemas of Ziti take two IPv4 addresses
// with a prefix of at most 32 bits, or two IPv6 addresses with one of at most
// 128.
func TestForwardAddressTranslations(t *testing.T) {
	validators := crdValidators(t)
	rules := crdRules(t)

	manifests := map[string]func(translation map[string]any) map[string]any{
		"ConfigHostV1": func(translation map[string]any) map[string]any {
			return map[string]any{
				"name": "web-host", "forwardAddress": true, "allowedAddresses": []any{"10.0.0.0/8"}, "port": int64(8080), "protocol": "tcp",
				"forwardAddressTranslations": []any{translation},
			}
		},
		"ConfigHostV2": func(translation map[string]any) map[string]any {
			return map[string]any{"name": "web-hosts", "terminators": []any{map[string]any{
				"forwardAddress": true, "allowedAddresses": []any{"10.0.0.0/8"}, "port": int64(8080), "protocol": "tcp",
				"forwardAddressTranslations": []any{translation},
			}}}
		},
	}

	cases := map[string]struct {
		from, to     string
		prefixLength int64
		want         string
	}{
		"IPv4":                    {from: "10.0.0.0", to: "192.168.0.0", prefixLength: 16},
		"IPv4WholeAddress":        {from: "10.0.0.1", to: "192.168.0.1", prefixLength: 32},
		"IPv4PrefixTooLong":       {from: "10.0.0.0", to: "192.168.0.0", prefixLength: 40, want: "prefixLength must be at most 32 for IPv4"},
		"IPv6":                    {from: "fd00::", to: "fd01::", prefixLength: 64},
		"IPv6WholeAddress":        {from: "fd00::1", to: "fd01::1", prefixLength: 128},
		"IPv6PrefixTooLong":       {from: "fd00::", to: "fd01::", prefixLength: 129, want: "should be less than or equal to 128"},
		"IPv4ToIPv6":              {from: "10.0.0.0", to: "fd01::", prefixLength: 16, want: "from and to must be IP addresses of the same family"},
		"IPv6ToIPv4":              {from: "fd00::", to: "192.168.0.0", prefixLength: 16, want: "from and to must be IP addresses of the same family"},
		"FromInCIDRNotation":      {from: "10.0.0.0/16", to: "192.168.0.0", prefixLength: 16, want: "from and to must be IP addresses of the same family"},
		"ToIsAHostName":           {from: "10.0.0.0", to: "internal.example", prefixLength: 16, want: "from and to must be IP addresses of the same family"},
		"NegativePrefixLength":    {from: "10.0.0.0", to: "192.168.0.0", prefixLength: -1, want: "should be greater than or equal to 0"},
		"FromLongerThanAnAddress": {from: strings.Repeat("1", 46), to: "192.168.0.0", prefixLength: 16, want: "may not be more than 45 bytes"},
	}

	for kind, forProvider := range manifests {
		for name, tc := range cases {
			t.Run(kind+"/"+name, func(t *testing.T) {
				u := map[string]any{
					"apiVersion": v1alpha1.Group + "/" + v1alpha1.Version,
					"kind":       kind,
					"metadata":   map[string]any{"name": "web", "namespace": "default"},
					"spec": map[string]any{"forProvider": forProvider(map[string]any{
						"from": tc.from, "to": tc.to, "prefixLength": tc.prefixLength,
					})},
				}
				errs := validation.ValidateCustomResource(nil, u, validators[kind])
				errs = append(errs, rules[kind](u)...)

				if tc.want == "" {
					if len(errs) > 0 {
						t.Fatalf("want valid, got %v", errs.ToAggregate())
					}
					return
				}
				if len(errs) == 0 {
					t.Fatalf("want an error with %q, got none", tc.want)
				}
				if got := errs.ToAggregate().Error(); !strings.Contains(got, tc.want) {
					t.Errorf("want an error with %q, got %s", tc.want, got)
				}
			})
		}
	}
}

// crdRules returns, per kind, a function that evaluates the CEL rules of the
// generated CRD on an object, as the API server does on create.
func crdRules(t *testing.T) map[string]func(obj map[string]any) field.ErrorList {
	t.Helper()

	rules := map[string]func(obj map[string]any) field.ErrorList{}
	for _, path := range yamlFiles(t, crdDir) {
		raw, err := os.ReadFile(path) //nolint:gosec // Reads generated manifests of this repository.
		if err != nil {
			t.Fatalf("cannot read %s: %v", path, err)
		}

		crd := &apiextensionsv1.CustomResourceDefinition{}
		if err := yaml.Unmarshal(raw, crd); err != nil {
			t.Fatalf("cannot parse %s: %v", path, err)
		}

		for _, version := range crd.Spec.Versions {
			internal := &apiextensions.CustomResourceValidation{}
			if err := apiextensionsv1.Convert_v1_CustomResourceValidation_To_apiextensions_CustomResourceValidation(version.Schema, internal, nil); err != nil {
				t.Fatalf("cannot convert the schema of %s: %v", path, err)
			}
			structural, err := structuralschema.NewStructural(internal.OpenAPIV3Schema)
			if err != nil {
				t.Fatalf("the schema of %s is not structural: %v", path, err)
			}
			validator := cel.NewValidator(structural, true, celconfig.PerCallLimit)
			rules[crd.Spec.Names.Kind] = func(obj map[string]any) field.ErrorList {
				if validator == nil {
					return nil
				}
				errs, _ := validator.Validate(context.Background(), nil, structural, obj, nil, celconfig.RuntimeCELCostBudget)
				return errs
			}
		}
	}
	return rules
}
