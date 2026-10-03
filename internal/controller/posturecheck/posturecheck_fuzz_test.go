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

package posturecheck

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/crossplane/provider-ziti/apis/v1alpha1"
)

// nonHex is what the Ziti controller removes from a MAC address after it
// has put it in lower case (cleanMacAddress in its posture check store).
var nonHex = regexp.MustCompile("[^a-f0-9]")

// FuzzHex checks that Hex rewrites a value exactly as the Ziti controller
// rewrites a MAC address, that its result only has hexadecimal digits in
// lower case, and that rewriting it again changes nothing.
func FuzzHex(f *testing.F) {
	for _, seed := range []string{"", "00:1A:2B:3C:4D:5E", "0a-1b-2c-3d-4e-5f", "001a.2b3c.4d5e", "A9 09 50 2D", "sha256:FFEE", "ÄÖ\xff", "K"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, v string) {
		got := Hex(v1alpha1.HexString(v))
		if want := nonHex.ReplaceAllString(strings.ToLower(v), ""); got != want {
			t.Fatalf("Hex(%q) = %q, Ziti stores %q", v, got, want)
		}
		if again := Hex(v1alpha1.HexString(got)); again != got {
			t.Fatalf("Hex(Hex(%q)) = %q, want %q", v, again, got)
		}
	})
}

// FuzzSet checks that Set returns a list the way Ziti stores it: sorted,
// without duplicates, with every item of the original, whatever their
// order, and that it is a fixed point.
func FuzzSet(f *testing.F) {
	f.Add(">=14.0.0,>=13.0.0,>=14.0.0")
	f.Add("")
	f.Add("b,a,,a")

	f.Fuzz(func(t *testing.T, joined string) {
		l := strings.Split(joined, ",")
		set := Set(l)

		if !slices.IsSorted(set) || len(slices.Compact(slices.Clone(set))) != len(set) {
			t.Fatalf("Set(%q) = %q: want a sorted list without duplicates", l, set)
		}
		for _, item := range l {
			if !slices.Contains(set, item) {
				t.Fatalf("Set(%q) = %q: %q is missing", l, set, item)
			}
		}
		reversed := slices.Clone(l)
		slices.Reverse(reversed)
		if got := Set(reversed); !slices.Equal(got, set) {
			t.Fatalf("Set(%q) = %q, but Set(%q) = %q", reversed, got, l, set)
		}
		if got := Set(set); !slices.Equal(got, set) {
			t.Fatalf("Set(Set(%q)) = %q, want %q", l, got, set)
		}

		hex := make([]v1alpha1.HexString, 0, len(l))
		for _, item := range l {
			hex = append(hex, v1alpha1.HexString(item))
		}
		hexSet := HexSet(hex)
		again := make([]v1alpha1.HexString, 0, len(hexSet))
		for _, item := range hexSet {
			again = append(again, v1alpha1.HexString(item))
		}
		if got := HexSet(again); !slices.Equal(got, hexSet) {
			t.Fatalf("HexSet(HexSet(%q)) = %q, want %q", l, got, hexSet)
		}
	})
}
