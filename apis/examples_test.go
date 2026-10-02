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
	"bufio"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/validation"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"

	"github.com/crossplane/provider-ziti/apis"
	"github.com/crossplane/provider-ziti/apis/v1alpha1"
)

const (
	crdDir      = "../package/crds"
	examplesDir = "../examples"
)

// TestExamples checks that every example manifest of a provider kind is
// accepted by the CRD of its kind and has no fields the kind does not know.
func TestExamples(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := apis.AddToScheme(scheme); err != nil {
		t.Fatalf("cannot add APIs to scheme: %v", err)
	}
	validators := crdValidators(t)

	covered := map[string]bool{}
	for _, path := range yamlFiles(t, examplesDir) {
		for i, doc := range documents(t, path) {
			u := &unstructured.Unstructured{}
			if err := yaml.Unmarshal(doc, &u.Object); err != nil {
				t.Errorf("%s (document %d): cannot parse: %v", path, i, err)
				continue
			}
			if u.GroupVersionKind().Group != v1alpha1.Group {
				continue
			}

			kind := u.GetKind()
			covered[kind] = true

			validator, ok := validators[kind]
			if !ok {
				t.Errorf("%s (document %d): no CRD for kind %s", path, i, kind)
				continue
			}
			if errs := validation.ValidateCustomResource(nil, u.Object, validator); len(errs) > 0 {
				t.Errorf("%s (document %d): %s %q is not accepted by its CRD: %v", path, i, kind, u.GetName(), errs.ToAggregate())
			}

			typed, err := scheme.New(u.GroupVersionKind())
			if err != nil {
				t.Errorf("%s (document %d): %v", path, i, err)
				continue
			}
			if err := yaml.UnmarshalStrict(doc, typed); err != nil {
				t.Errorf("%s (document %d): %s %q has unknown fields: %v", path, i, kind, u.GetName(), err)
			}
		}
	}

	for kind := range validators {
		if !covered[kind] && kind != v1alpha1.ProviderConfigUsageKind && kind != v1alpha1.ClusterProviderConfigUsageKind {
			t.Errorf("kind %s has no example in %s", kind, examplesDir)
		}
	}
}

// crdValidators returns a schema validator per kind of the generated CRDs.
func crdValidators(t *testing.T) map[string]validation.SchemaValidator {
	t.Helper()

	validators := map[string]validation.SchemaValidator{}
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
			validator, _, err := validation.NewSchemaValidator(internal.OpenAPIV3Schema)
			if err != nil {
				t.Fatalf("cannot build a validator for %s: %v", path, err)
			}
			validators[crd.Spec.Names.Kind] = validator
		}
	}
	return validators
}

func yamlFiles(t *testing.T, dir string) []string {
	t.Helper()

	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && (filepath.Ext(path) == ".yaml" || filepath.Ext(path) == ".yml") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("cannot list %s: %v", dir, err)
	}
	return files
}

// documents splits a multi-document YAML file.
func documents(t *testing.T, path string) [][]byte {
	t.Helper()

	raw, err := os.ReadFile(path) //nolint:gosec // Reads example manifests of this repository.
	if err != nil {
		t.Fatalf("cannot read %s: %v", path, err)
	}

	var docs [][]byte
	reader := utilyaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(raw)))
	for {
		doc, err := reader.Read()
		if errors.Is(err, io.EOF) {
			return docs
		}
		if err != nil {
			t.Fatalf("cannot split %s: %v", path, err)
		}
		if len(bytes.TrimSpace(doc)) > 0 {
			docs = append(docs, doc)
		}
	}
}
