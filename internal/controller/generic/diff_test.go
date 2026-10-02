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
	"encoding/json"
	"testing"
)

func TestFirstDifference(t *testing.T) {
	cases := map[string]struct {
		desired  map[string]any
		observed string
		want     string
	}{
		"Equal": {
			desired:  map[string]any{"name": "web", "encryptionRequired": true, "cost": int64(10)},
			observed: `{"name": "web", "encryptionRequired": true, "cost": 10}`,
		},
		"UnmanagedFieldsAreIgnored": {
			desired:  map[string]any{"name": "web"},
			observed: `{"name": "web", "id": "abc", "createdAt": "2026-01-01T00:00:00Z"}`,
		},
		"ScalarDiffers": {
			desired:  map[string]any{"name": "web", "encryptionRequired": false},
			observed: `{"name": "web", "encryptionRequired": true}`,
			want:     "encryptionRequired",
		},
		"FirstFieldInAlphabeticalOrderIsReported": {
			desired:  map[string]any{"name": "web", "cost": 1},
			observed: `{"name": "db", "cost": 2}`,
			want:     "cost",
		},
		"MissingFieldDiffers": {
			desired:  map[string]any{"terminatorStrategy": "weighted"},
			observed: `{}`,
			want:     "terminatorStrategy",
		},
		"EmptyListEqualsNull": {
			desired:  map[string]any{"roleAttributes": []string{}},
			observed: `{"roleAttributes": null}`,
		},
		"EmptyListEqualsAbsent": {
			desired:  map[string]any{"roleAttributes": []string{}},
			observed: `{}`,
		},
		"EmptyObjectEqualsNull": {
			desired:  map[string]any{"tags": map[string]string{}},
			observed: `{"tags": null}`,
		},
		"FalseEqualsAbsent": {
			desired:  map[string]any{"promptOnWake": false, "cost": 0, "externalId": ""},
			observed: `{}`,
		},
		"FalseDiffersFromTrue": {
			desired:  map[string]any{"promptOnWake": false},
			observed: `{"promptOnWake": true}`,
			want:     "promptOnWake",
		},
		"TrueDiffersFromAbsent": {
			desired:  map[string]any{"promptOnWake": true},
			observed: `{}`,
			want:     "promptOnWake",
		},
		"NestedFalseEqualsAbsent": {
			desired:  map[string]any{"primary": map[string]any{"cert": map[string]any{"allowed": true, "allowExpiredCerts": false}}},
			observed: `{"primary": {"cert": {"allowed": true}}}`,
		},
		"TopLevelStringListIsASet": {
			desired:  map[string]any{"identityRoles": []string{"#b", "@id-1", "#a"}},
			observed: `{"identityRoles": ["#a", "#b", "@id-1"]}`,
		},
		"TopLevelStringListDiffers": {
			desired:  map[string]any{"identityRoles": []string{"#a"}},
			observed: `{"identityRoles": ["#a", "#b"]}`,
			want:     "identityRoles",
		},
		"TopLevelStringListIsCleared": {
			desired:  map[string]any{"identityRoles": []string{}},
			observed: `{"identityRoles": ["#a"]}`,
			want:     "identityRoles",
		},
		"NestedObjectEqual": {
			desired:  map[string]any{"data": map[string]any{"address": "localhost", "port": int32(8080), "allowedAddresses": nil}},
			observed: `{"data": {"address": "localhost", "port": 8080}}`,
		},
		"NestedListIsOrdered": {
			desired:  map[string]any{"data": map[string]any{"addresses": []string{"a.ziti", "b.ziti"}}},
			observed: `{"data": {"addresses": ["b.ziti", "a.ziti"]}}`,
			want:     "data",
		},
		"NestedObjectMustMatchExactly": {
			desired:  map[string]any{"data": map[string]any{"address": "localhost"}},
			observed: `{"data": {"address": "localhost", "port": 8080}}`,
			want:     "data",
		},
		"TagRemoved": {
			desired:  map[string]any{"tags": map[string]string{"team": "a"}},
			observed: `{"tags": {"team": "a", "env": "dev"}}`,
			want:     "tags",
		},
		"ListOfObjectsDiffers": {
			desired:  map[string]any{"operatingSystems": []map[string]any{{"type": "Linux", "versions": []string{}}}},
			observed: `{"operatingSystems": [{"type": "Windows", "versions": null}]}`,
			want:     "operatingSystems",
		},
		"ListOfObjectsEqual": {
			desired:  map[string]any{"operatingSystems": []map[string]any{{"type": "Linux", "versions": []string{}}}},
			observed: `{"operatingSystems": [{"type": "Linux", "versions": null}]}`,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			desired, err := normalize(tc.desired)
			if err != nil {
				t.Fatalf("normalize(...): %v", err)
			}
			observed := map[string]any{}
			if err := json.Unmarshal([]byte(tc.observed), &observed); err != nil {
				t.Fatalf("cannot parse observed entity: %v", err)
			}

			got, differs := firstDifference(desired, observed)
			if got != tc.want || differs != (tc.want != "") {
				t.Errorf("firstDifference(...): want %q, got %q (differs: %t)", tc.want, got, differs)
			}
		})
	}
}
