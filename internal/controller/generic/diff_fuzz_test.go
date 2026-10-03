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
	"maps"
	"reflect"
	"slices"
	"testing"
)

// stored returns a desired body the way Ziti reports it back: without the
// fields that are empty, which Ziti leaves out, and with every top-level
// list of strings as a sorted set, which is how Ziti stores those lists.
func stored(desired map[string]any) map[string]any {
	out := map[string]any{}
	for field, v := range desired {
		if isEmpty(v) {
			continue
		}
		if l, ok := stringSet(v); ok {
			set := make([]any, 0, len(l))
			for _, s := range l {
				set = append(set, s)
			}
			v = set
		}
		out[field] = v
	}
	return out
}

// shuffled returns a body as a spec might give it: with every top-level list
// of strings in another order and with its first item twice.
func shuffled(body map[string]any) map[string]any {
	out := maps.Clone(body)
	for field, v := range out {
		l, ok := v.([]any)
		if !ok || len(l) == 0 {
			continue
		}
		if _, ok := stringSet(l); !ok {
			continue
		}
		s := slices.Clone(l)
		slices.Reverse(s)
		out[field] = append(s, l[0])
	}
	return out
}

// changed returns a body with the value of the supplied field changed in a
// way Ziti would not report it, and false if the field has no such value.
func changed(body map[string]any, field string) (map[string]any, bool) {
	out := maps.Clone(body)
	switch v := out[field].(type) {
	case string:
		out[field] = v + "-changed"
	case bool:
		out[field] = !v
	case float64:
		if v+1 == v {
			return nil, false
		}
		out[field] = v + 1
	default:
		return nil, false
	}
	return out, true
}

// FuzzFirstDifference checks the comparison of a desired body with the
// entity Ziti reports, for any JSON object as the desired body:
//
//   - normalizing a body twice gives what normalizing it once gives;
//   - a body does not differ from itself;
//   - a body does not differ from the form in which Ziti stores it, nor does
//     a body that lists the same strings in another order or twice;
//   - a body differs from an entity in which a value it sets has changed.
func FuzzFirstDifference(f *testing.F) {
	for _, seed := range []string{
		`{}`,
		`{"name": "web", "encryptionRequired": true, "roleAttributes": ["b", "a"], "tags": {}}`,
		`{"identityRoles": ["#all", "@id-1", "#all"], "semantic": "AnyOf", "postureCheckRoles": []}`,
		`{"data": {"addresses": ["b.ziti", "a.ziti"], "portRanges": [{"low": 80, "high": 80}]}, "tags": {"team": "web"}}`,
		`{"primary": {"cert": {"allowed": true, "allowExpiredCerts": false}}, "secondary": {"requireExtJwtSigner": null}}`,
		`{"cost": 0, "isAdmin": false, "externalId": "", "appData": null, "configs": [1, "a"]}`,
		`{"maxIdleTimeMillis": 9007199254740993, "operatingSystems": [{"type": "Linux", "versions": []}]}`,
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		body := map[string]any{}
		if err := json.Unmarshal([]byte(raw), &body); err != nil {
			return
		}

		desired, err := normalize(body)
		if err != nil {
			t.Fatalf("normalize(...): %v", err)
		}
		again, err := normalize(desired)
		if err != nil {
			t.Fatalf("normalize(normalize(...)): %v", err)
		}
		if !reflect.DeepEqual(desired, again) {
			t.Fatalf("normalize(normalize(x)) != normalize(x):\n%v\n%v", again, desired)
		}

		if field, ok := firstDifference(desired, desired); ok {
			t.Fatalf("a body differs from itself in %q: %v", field, desired)
		}
		reported := stored(desired)
		if field, ok := firstDifference(desired, reported); ok {
			t.Fatalf("a body differs from the form Ziti stores it in, in %q:\ndesired %v\nreported %v", field, desired, reported)
		}
		if field, ok := firstDifference(shuffled(desired), reported); ok {
			t.Fatalf("a body with its lists in another order differs from what Ziti stores, in %q:\ndesired %v\nreported %v", field, shuffled(desired), reported)
		}

		for _, field := range slices.Sorted(maps.Keys(desired)) {
			observed, ok := changed(reported, field)
			if !ok {
				continue
			}
			if _, differs := firstDifference(desired, observed); !differs {
				t.Fatalf("a change of %q goes unnoticed:\ndesired %v\nobserved %v", field, desired, observed)
			}
		}
	})
}
