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

// Package posturecheck holds what the posture check kinds share: the form in
// which Ziti stores their values.
//
// Ziti does not store the values of a posture check as they are sent. It
// keeps every list of strings as a set and returns it sorted and without
// duplicates, and it rewrites MAC addresses, hashes and fingerprints to lower
// case without separators. A kind that sent the values of its spec as they
// are would see a difference on every poll and update the entity forever, so
// the kinds send the stored form.
package posturecheck

import (
	"slices"
	"strings"

	"github.com/crossplane/provider-ziti/apis/v1alpha1"
)

// Collection of the Ziti Edge Management API all posture checks are stored in.
const Collection = "posture-checks"

// Hex returns a hexadecimal value the way Ziti stores it: in lower case and
// without anything that is not a digit.
func Hex(v v1alpha1.HexString) string {
	return strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			return r
		}
		return -1
	}, strings.ToLower(string(v)))
}

// Set returns a list of strings the way Ziti stores it: sorted and without
// duplicates. It is never nil, so that it is sent as [] and clears the field.
func Set(l []string) []string {
	out := append([]string{}, l...)
	slices.Sort(out)
	return slices.Compact(out)
}

// HexSet returns a list of hexadecimal values the way Ziti stores it.
func HexSet(l []v1alpha1.HexString) []string {
	out := make([]string, 0, len(l))
	for _, v := range l {
		out = append(out, Hex(v))
	}
	return Set(out)
}
