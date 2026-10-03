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
	"slices"
)

// normalize converts a body to the representation json.Unmarshal produces, so
// that it can be compared with an entity returned by the API.
func normalize(body map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// firstDifference returns the first field of the desired body, in
// alphabetical order, that the observed entity does not match.
//
// Fields of the entity that are not part of the body are ignored: they are
// either read-only or not managed. Values of the body must match exactly,
// except that:
//   - an absent value, null, false, zero, an empty string, an empty list and
//     an empty object are all considered equal, as Ziti leaves empty values
//     out of its responses;
//   - a top-level list of strings is compared as a set: Ziti stores roles,
//     attributes and the other lists of strings of an entity as sets, which
//     keep neither the order of their items nor duplicates.
func firstDifference(desired, observed map[string]any) (string, bool) {
	for _, field := range slices.Sorted(maps.Keys(desired)) {
		d, o := desired[field], observed[field]

		if ds, ok := stringSet(d); ok && d != nil {
			if os, ok := stringSet(o); ok && slices.Equal(ds, os) {
				continue
			}
			return field, true
		}

		if !equal(d, o) {
			return field, true
		}
	}
	return "", false
}

// stringSet returns the elements of a list of strings sorted and without
// duplicates. Null is an empty list.
func stringSet(v any) ([]string, bool) {
	if v == nil {
		return []string{}, true
	}

	list, ok := v.([]any)
	if !ok {
		return nil, false
	}

	out := make([]string, 0, len(list))
	for _, item := range list {
		s, ok := item.(string)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	slices.Sort(out)
	return slices.Compact(out), true
}

func equal(d, o any) bool {
	if isEmpty(d) && isEmpty(o) {
		return true
	}

	switch dv := d.(type) {
	case []any:
		ov, ok := o.([]any)
		return ok && slices.EqualFunc(dv, ov, equal)
	case map[string]any:
		ov, ok := o.(map[string]any)
		return ok && equalObjects(dv, ov)
	default:
		return d == o
	}
}

func equalObjects(d, o map[string]any) bool {
	for k := range d {
		if !equal(d[k], o[k]) {
			return false
		}
	}
	for k := range o {
		if _, ok := d[k]; !ok && !isEmpty(o[k]) {
			return false
		}
	}
	return true
}

// isEmpty returns true for null and the zero value of every JSON type.
func isEmpty(v any) bool {
	switch tv := v.(type) {
	case nil:
		return true
	case bool:
		return !tv
	case float64:
		return tv == 0
	case string:
		return tv == ""
	case []any:
		return len(tv) == 0
	case map[string]any:
		return len(tv) == 0
	default:
		return false
	}
}
