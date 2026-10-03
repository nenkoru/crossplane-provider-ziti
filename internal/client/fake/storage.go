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

package fake

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// stored rewrites an entity that was sent to the fake controller into the
// form in which the real controller stores and reports it.
//
// The real controller keeps every list of strings of an entity as the keys
// of a bolt bucket: as a set, reported in byte order without duplicates.
// That holds for role attributes, policy roles, the configs of a service and
// the scopes of a signer at the top level of an entity, and for the lists
// nested in posture checks and auth policies below. It also rewrites MAC
// addresses, hashes and fingerprints of posture checks to lower case, MAC
// addresses without separators, and keeps one operating system of an OS
// check per type and one process of a multi process check per operating
// system and path, in the order of those keys. The data of a config is kept
// as it was sent.
func stored(collection string, entity map[string]any) {
	for field, v := range entity {
		if l, ok := stringSet(v, asIs); ok {
			entity[field] = l
		}
	}

	switch collection {
	case "posture-checks":
		storedPostureCheck(entity)
	case "auth-policies":
		primary, _ := entity["primary"].(map[string]any)
		extJWT, _ := primary["extJwt"].(map[string]any)
		if signers, ok := stringSet(extJWT["allowedSigners"], asIs); ok {
			extJWT["allowedSigners"] = signers
		}
	}
}

func storedPostureCheck(entity map[string]any) {
	if macs, ok := stringSet(entity["macAddresses"], cleanHex); ok {
		entity["macAddresses"] = macs
	}

	if process, ok := entity["process"].(map[string]any); ok {
		if hashes, ok := stringSet(process["hashes"], cleanHex); ok {
			process["hashes"] = hashes
		}
		if fingerprint, ok := process["signerFingerprint"].(string); ok {
			process["signerFingerprint"] = cleanHex(fingerprint)
		}
	}

	if oses, ok := entity["operatingSystems"].([]any); ok {
		entity["operatingSystems"] = keyed(oses, func(os map[string]any) string {
			if versions, ok := stringSet(os["versions"], asIs); ok {
				os["versions"] = versions
			}
			return fmt.Sprint(os["type"])
		})
	}

	if processes, ok := entity["processes"].([]any); ok {
		entity["processes"] = keyed(processes, func(p map[string]any) string {
			for _, field := range []string{"hashes", "signerFingerprints"} {
				if l, ok := stringSet(p[field], cleanHex); ok {
					p[field] = l
				}
			}
			return fmt.Sprint(p["osType"]) + "-" + fmt.Sprint(p["path"])
		})
	}
}

// keyed keeps the last of the objects of a list with the same key, in the
// order of the keys, as entries of a bolt bucket are kept.
func keyed(list []any, key func(map[string]any) string) []any {
	byKey := map[string]any{}
	for _, item := range list {
		if o, ok := item.(map[string]any); ok {
			byKey[key(o)] = o
		}
	}
	out := make([]any, 0, len(byKey))
	for _, k := range slices.Sorted(maps.Keys(byKey)) {
		out = append(out, byKey[k])
	}
	return out
}

// stringSet returns a list of strings as a set: rewritten by the supplied
// function, sorted and without duplicates. The second value is false for
// anything but a list of strings.
func stringSet(v any, rewrite func(string) string) ([]any, bool) {
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
		out = append(out, rewrite(s))
	}
	slices.Sort(out)
	out = slices.Compact(out)

	set := make([]any, 0, len(out))
	for _, s := range out {
		set = append(set, s)
	}
	return set, true
}

func asIs(s string) string { return s }

// cleanHex returns a MAC address, a hash or a fingerprint the way the real
// controller stores it: in lower case and without anything that is not a
// hexadecimal digit.
func cleanHex(s string) string {
	return strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			return r
		}
		return -1
	}, strings.ToLower(s))
}

// references are the fields of policies and services that the real
// controller removes a reference to an entity of the collection from when
// the entity is deleted: the "@id" roles of the policies, and the IDs of
// configs in services.
var references = map[string][]struct{ collection, field string }{
	"services": {
		{"service-policies", "serviceRoles"},
		{"service-edge-router-policies", "serviceRoles"},
	},
	"identities": {
		{"service-policies", "identityRoles"},
		{"edge-router-policies", "identityRoles"},
	},
	"edge-routers": {
		{"edge-router-policies", "edgeRouterRoles"},
		{"service-edge-router-policies", "edgeRouterRoles"},
	},
	"posture-checks": {
		{"service-policies", "postureCheckRoles"},
	},
	"configs": {
		{"services", "configs"},
	},
}

// dropReferences removes the references to a deleted entity that the real
// controller removes. It keeps the hosting settings of identities for a
// deleted service, as the real controller does.
func (s *Server) dropReferences(collection, id string) {
	ref := "@" + id
	if collection == "configs" {
		ref = id
	}
	for _, r := range references[collection] {
		for _, e := range s.collections[r.collection] {
			if l, ok := e[r.field].([]any); ok {
				e[r.field] = slices.DeleteFunc(slices.Clone(l), func(v any) bool { return v == ref })
			}
		}
	}
}
