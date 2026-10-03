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
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"k8s.io/apiextensions-apiserver/pkg/apiserver/validation"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	"github.com/crossplane/provider-ziti/apis"
	"github.com/crossplane/provider-ziti/apis/v1alpha1"
)

const (
	readme = "../README.md"
	e2e    = "../test/e2e/e2e.sh"
)

var (
	// exampleRef matches a path of an example manifest.
	exampleRef = regexp.MustCompile(`examples/[A-Za-z0-9_./-]+\.ya?ml`)
	// yamlBlock matches a fenced YAML block of Markdown.
	yamlBlock = regexp.MustCompile("(?s)```yaml\n(.*?)```")
)

// TestReadme checks the examples of the README: every manifest it refers to
// exists, and is applied by the end-to-end test unless it only connects the
// provider to a controller, and every manifest of a kind of the provider it
// shows inline is accepted by the CRD of its kind and has no unknown fields.
// TestExamples validates the manifests in examples/.
func TestReadme(t *testing.T) {
	raw, err := os.ReadFile(readme)
	if err != nil {
		t.Fatalf("cannot read the README: %v", err)
	}
	script, err := os.ReadFile(e2e)
	if err != nil {
		t.Fatalf("cannot read the end-to-end test: %v", err)
	}

	refs := exampleRef.FindAllString(string(raw), -1)
	if len(refs) == 0 {
		t.Fatalf("the README refers to no example")
	}
	for _, ref := range refs {
		if _, err := os.Stat(filepath.Join("..", ref)); err != nil {
			t.Errorf("the README refers to %s: %v", ref, err)
			continue
		}
		if !strings.HasPrefix(ref, "examples/provider/") && !strings.Contains(string(script), ref) {
			t.Errorf("the README refers to %s, which the end-to-end test does not apply", ref)
		}
	}

	scheme := runtime.NewScheme()
	if err := apis.AddToScheme(scheme); err != nil {
		t.Fatalf("cannot add APIs to scheme: %v", err)
	}
	validators := crdValidators(t)
	for _, block := range yamlBlock.FindAllStringSubmatch(string(raw), -1) {
		for _, doc := range strings.Split(block[1], "\n---\n") {
			u := &unstructured.Unstructured{}
			if err := yaml.Unmarshal([]byte(doc), &u.Object); err != nil {
				t.Errorf("README: cannot parse a manifest: %v\n%s", err, doc)
				continue
			}
			if u.GroupVersionKind().Group != v1alpha1.Group {
				continue
			}
			if errs := validation.ValidateCustomResource(nil, u.Object, validators[u.GetKind()]); len(errs) > 0 {
				t.Errorf("README: %s %q is not accepted by its CRD: %v", u.GetKind(), u.GetName(), errs.ToAggregate())
			}
			typed, err := scheme.New(u.GroupVersionKind())
			if err != nil {
				t.Errorf("README: %v", err)
				continue
			}
			if err := yaml.UnmarshalStrict([]byte(doc), typed); err != nil {
				t.Errorf("README: %s %q has unknown fields: %v", u.GetKind(), u.GetName(), err)
			}
		}
	}
}
