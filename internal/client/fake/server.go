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

// Package fake provides an in-memory fake of the Ziti Edge Management API
// for use in tests.
package fake

import (
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/crossplane/provider-ziti/internal/client"
)

const (
	// Username accepted by the fake controller.
	Username = "admin"
	// Password accepted by the fake controller.
	Password = "admin"

	prefix = "/edge/management/v1"

	// routerEnrollmentDuration is how long the enrollment token of an edge
	// router that is enrolled anew is valid.
	routerEnrollmentDuration = 180 * time.Minute
)

var nameFilter = regexp.MustCompile(`^name="((?:[^"\\]|\\.)*)"$`)

// defaults are the values the real controller stores in place of an empty
// one.
var defaults = map[string]map[string]any{
	"cas": {"identityNameFormat": "[caName]-[commonName]"},
	"external-jwt-signers": {
		"claimsProperty":           "/sub",
		"targetToken":              "ACCESS",
		"enrollNameClaimsSelector": "/sub",
		"enrollAuthPolicyId":       "default",
	},
}

// unchangeable are the fields the real controller keeps when an entity is
// replaced: they cannot be changed, or only the controller sets them.
var unchangeable = map[string][]string{
	"cas": {"certPem", "fingerprint", "isVerified", "verificationToken"},
}

// Server is a fake Ziti controller. Entities are stored as the JSON documents
// that were posted to it, plus an id and the time they were created.
type Server struct {
	*httptest.Server

	mu          sync.Mutex
	collections map[string]map[string]map[string]any
	sessions    map[string]bool
	nextID      int
	logins      int
	requests    []string
	ahead       time.Duration
	loseCreate  bool
	failRenewal bool
}

// NewServer starts a fake Ziti controller serving TLS.
func NewServer() *Server {
	s := &Server{
		collections: map[string]map[string]map[string]any{},
		sessions:    map[string]bool{},
	}
	s.Server = httptest.NewUnstartedServer(http.HandlerFunc(s.handle))
	s.TLS = &tls.Config{ClientAuth: tls.RequestClientCert, MinVersion: tls.VersionTLS12}
	s.StartTLS()

	s.Put("config-types", map[string]any{"id": "host-v1-id", "name": "host.v1"})
	s.Put("config-types", map[string]any{"id": "host-v2-id", "name": "host.v2"})
	s.Put("config-types", map[string]any{"id": "intercept-v1-id", "name": "intercept.v1"})
	return s
}

// CA returns the PEM-encoded certificate of the fake controller.
func (s *Server) CA() string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.Certificate().Raw}))
}

// Client returns a client that is connected to the fake controller.
func (s *Server) Client() (*client.Client, error) {
	return client.New(client.Config{Host: s.URL, Username: Username, Password: Password, CA: s.CA()})
}

// Put stores an entity, replacing any entity with the same id.
func (s *Server) Put(collection string, entity map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.put(collection, entity)
}

func (s *Server) put(collection string, entity map[string]any) {
	if s.collections[collection] == nil {
		s.collections[collection] = map[string]map[string]any{}
	}
	s.collections[collection][fmt.Sprint(entity["id"])] = entity
}

// Delete removes an entity.
func (s *Server) Delete(collection, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.collections[collection], id)
}

// Entity returns a copy of the stored entity, or nil if it does not exist.
func (s *Server) Entity(collection, id string) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return maps.Clone(s.collections[collection][id])
}

// Len returns the number of entities in a collection.
func (s *Server) Len(collection string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.collections[collection])
}

// SetClockAhead makes the clock of the fake controller run ahead of the
// local one, or behind it if the duration is negative.
func (s *Server) SetClockAhead(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ahead = d
}

// LoseNextCreateResponse makes the fake controller carry out the next
// creation and answer it with an error, as a proxy does that gives up
// waiting for the controller.
func (s *Server) LoseNextCreateResponse() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseCreate = true
}

// FailNextRenewal makes the fake controller answer the next request for a
// new enrollment token with an error, without carrying it out.
func (s *Server) FailNextRenewal() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failRenewal = true
}

// now returns the time on the clock of the fake controller.
func (s *Server) now() time.Time {
	return time.Now().Add(s.ahead)
}

// ExpireSessions invalidates all API sessions.
func (s *Server) ExpireSessions() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions = map[string]bool{}
}

// Logins returns how many times a client authenticated successfully.
func (s *Server) Logins() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.logins
}

// Requests returns the method and path of every authenticated request.
func (s *Server) Requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.requests...)
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	w.Header().Set("Date", s.now().UTC().Format(http.TimeFormat))

	path, ok := strings.CutPrefix(r.URL.Path, prefix)
	if !ok {
		writeError(w, http.StatusNotFound, "NOT_FOUND", nil)
		return
	}

	if path == "/authenticate" {
		s.authenticate(w, r)
		return
	}

	if !s.sessions[r.Header.Get("zt-session")] {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", nil)
		return
	}
	s.requests = append(s.requests, r.Method+" "+path)

	collection, id, _ := strings.Cut(strings.TrimPrefix(path, "/"), "/")
	if entity, action, ok := strings.Cut(id, "/"); r.Method == http.MethodPost && (ok || collection == "enrollments") {
		s.renew(w, r, collection, entity, action)
		return
	}
	s.route(w, r, collection, id)
}

func (s *Server) route(w http.ResponseWriter, r *http.Request, collection, id string) {
	switch {
	case r.Method == http.MethodGet && id == "":
		s.list(w, r, collection)
	case r.Method == http.MethodPost && id == "":
		s.create(w, r, collection)
	case r.Method == http.MethodGet:
		s.get(w, collection, id)
	case r.Method == http.MethodPatch:
		s.patch(w, r, collection, id)
	case r.Method == http.MethodPut:
		s.replace(w, r, collection, id)
	case r.Method == http.MethodDelete:
		s.remove(w, collection, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", nil)
	}
}

func (s *Server) authenticate(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Query().Get("method") {
	case "password":
		var creds struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&creds); err != nil || creds.Username != Username || creds.Password != Password {
			writeError(w, http.StatusUnauthorized, "INVALID_AUTH", nil)
			return
		}
	case "cert":
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			writeError(w, http.StatusUnauthorized, "INVALID_AUTH", nil)
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "INVALID_AUTH_METHOD", nil)
		return
	}

	s.logins++
	token := fmt.Sprintf("session-%d", s.logins)
	s.sessions[token] = true
	writeData(w, http.StatusOK, map[string]any{"token": token})
}

func (s *Server) list(w http.ResponseWriter, r *http.Request, collection string) {
	items := []map[string]any{}
	match := nameFilter.FindStringSubmatch(r.URL.Query().Get("filter"))
	for _, e := range s.collections[collection] {
		if match == nil || e["name"] == strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(match[1]) {
			items = append(items, e)
		}
	}
	writeData(w, http.StatusOK, items)
}

func (s *Server) create(w http.ResponseWriter, r *http.Request, collection string) {
	entity := map[string]any{}
	if err := json.NewDecoder(r.Body).Decode(&entity); err != nil {
		writeError(w, http.StatusBadRequest, "COULD_NOT_PARSE_BODY", nil)
		return
	}

	for _, e := range s.collections[collection] {
		if e["name"] == entity["name"] {
			writeError(w, http.StatusBadRequest, "COULD_NOT_VALIDATE", map[string]any{"field": "name", "reason": "duplicate value"})
			return
		}
	}

	s.nextID++
	entity["id"] = fmt.Sprintf("id-%d", s.nextID)
	entity["createdAt"] = s.now().UTC().Format(time.RFC3339Nano)

	// The controller turns an enrollment request, whatever its method, into
	// an enrollment token.
	if enrollment, ok := entity["enrollment"].(map[string]any); ok {
		for method := range enrollment {
			enrollment[method] = map[string]any{
				"jwt":       "jwt-for-" + fmt.Sprint(entity["name"]),
				"expiresAt": "2030-01-01T00:00:00.000Z",
			}
		}
	}
	switch collection {
	case "edge-routers":
		entity["enrollmentJwt"] = "jwt-for-" + fmt.Sprint(entity["name"])
	case "cas":
		// A certificate authority starts out unverified, with the token its
		// owner verifies it with.
		entity["fingerprint"] = "fingerprint-of-" + fmt.Sprint(entity["name"])
		entity["isVerified"] = false
		entity["verificationToken"] = "token-for-" + fmt.Sprint(entity["name"])
	case "external-jwt-signers":
		// Like the real controller, the tags of a new signer are ignored.
		delete(entity, "tags")
	}
	setDefaults(collection, entity)

	s.put(collection, entity)
	if s.loseCreate {
		s.loseCreate = false
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	writeData(w, http.StatusCreated, map[string]any{"id": entity["id"]})
}

// renew handles a request for a new enrollment token: the creation or the
// refresh of the enrollment of an identity, or enrolling an edge router anew.
func (s *Server) renew(w http.ResponseWriter, r *http.Request, collection, id, action string) {
	if s.failRenewal {
		s.failRenewal = false
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", nil)
		return
	}

	switch {
	case collection == "enrollments" && id == "":
		s.createEnrollment(w, r)
	case collection == "enrollments" && action == "refresh":
		s.refreshEnrollment(w, r, id)
	case collection == "edge-routers" && action == "re-enroll":
		s.reEnroll(w, id)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", nil)
	}
}

// token returns an enrollment token that differs from every token issued
// before.
func (s *Server) token(entity map[string]any) string {
	s.nextID++
	return fmt.Sprintf("jwt-%d-for-%v", s.nextID, entity["name"])
}

// expiry returns the time a request wants an enrollment token to expire at,
// in the format of the API. Like the real controller, the fake one refuses a
// time that does not lie ahead on its clock.
func (s *Server) expiry(w http.ResponseWriter, settings map[string]any) (string, bool) {
	expiresAt, err := time.Parse(time.RFC3339, fmt.Sprint(settings["expiresAt"]))
	if err != nil || !expiresAt.After(s.now()) {
		writeError(w, http.StatusBadRequest, "COULD_NOT_VALIDATE", map[string]any{"field": "expiresAt", "reason": "must be in the future"})
		return "", false
	}
	return expiresAt.UTC().Format(time.RFC3339Nano), true
}

// createEnrollment gives an identity an enrollment of a method it has none
// of. The enrollment keeps the settings it was created with.
func (s *Server) createEnrollment(w http.ResponseWriter, r *http.Request) {
	enrollment := map[string]any{}
	if err := json.NewDecoder(r.Body).Decode(&enrollment); err != nil {
		writeError(w, http.StatusBadRequest, "COULD_NOT_PARSE_BODY", nil)
		return
	}
	identity, ok := s.collections["identities"][fmt.Sprint(enrollment["identityId"])]
	if !ok {
		writeError(w, http.StatusBadRequest, "COULD_NOT_VALIDATE", map[string]any{"field": "identityId", "reason": "identity not found"})
		return
	}
	expiresAt, ok := s.expiry(w, enrollment)
	if !ok {
		return
	}

	enrollments, _ := identity["enrollment"].(map[string]any)
	if enrollments == nil {
		enrollments = map[string]any{}
		identity["enrollment"] = enrollments
	}
	method := fmt.Sprint(enrollment["method"])
	if _, ok := enrollments[method]; ok {
		writeError(w, http.StatusConflict, "ENROLLMENT_EXISTS", nil)
		return
	}

	s.nextID++
	enrollment["id"] = fmt.Sprintf("id-%d", s.nextID)
	enrollment["jwt"] = s.token(identity)
	enrollment["expiresAt"] = expiresAt
	delete(enrollment, "identityId")
	delete(enrollment, "method")
	enrollments[method] = enrollment
	writeData(w, http.StatusCreated, map[string]any{"id": enrollment["id"]})
}

// refreshEnrollment gives the enrollment of an identity a new token.
func (s *Server) refreshEnrollment(w http.ResponseWriter, r *http.Request, id string) {
	settings := map[string]any{}
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		writeError(w, http.StatusBadRequest, "COULD_NOT_PARSE_BODY", nil)
		return
	}

	for _, identity := range s.collections["identities"] {
		enrollments, _ := identity["enrollment"].(map[string]any)
		for _, e := range enrollments {
			enrollment, _ := e.(map[string]any)
			if enrollment["id"] != id {
				continue
			}
			expiresAt, ok := s.expiry(w, settings)
			if !ok {
				return
			}
			enrollment["jwt"] = s.token(identity)
			enrollment["expiresAt"] = expiresAt
			writeData(w, http.StatusOK, map[string]any{})
			return
		}
	}
	writeError(w, http.StatusNotFound, "NOT_FOUND", nil)
}

// reEnroll gives an edge router a new enrollment token. Like the real
// controller, it does so for a router that has enrolled as well, which
// thereby loses its certificate.
func (s *Server) reEnroll(w http.ResponseWriter, id string) {
	router, ok := s.collections["edge-routers"][id]
	if !ok {
		writeError(w, http.StatusNotFound, "NOT_FOUND", nil)
		return
	}
	router["enrollmentJwt"] = s.token(router)
	router["enrollmentExpiresAt"] = s.now().Add(routerEnrollmentDuration).UTC().Format(time.RFC3339Nano)
	router["isVerified"] = false
	delete(router, "fingerprint")
	writeData(w, http.StatusOK, map[string]any{})
}

func (s *Server) get(w http.ResponseWriter, collection, id string) {
	entity, ok := s.collections[collection][id]
	if !ok {
		writeError(w, http.StatusNotFound, "NOT_FOUND", nil)
		return
	}
	writeData(w, http.StatusOK, entity)
}

func (s *Server) patch(w http.ResponseWriter, r *http.Request, collection, id string) {
	entity, ok := s.collections[collection][id]
	if !ok {
		writeError(w, http.StatusNotFound, "NOT_FOUND", nil)
		return
	}

	fields := map[string]any{}
	if err := json.NewDecoder(r.Body).Decode(&fields); err != nil {
		writeError(w, http.StatusBadRequest, "COULD_NOT_PARSE_BODY", nil)
		return
	}
	// Like the real controller, a PATCH silently ignores encryptionRequired
	// of a service and does not change an auth policy reliably.
	switch collection {
	case "services":
		delete(fields, "encryptionRequired")
	case "auth-policies":
		delete(fields, "primary")
	case "external-jwt-signers":
		// Nor does a PATCH remove a setting: it ignores fields that are null.
		maps.DeleteFunc(fields, func(_ string, v any) bool { return v == nil })
	case "cas":
		// Likewise, except for the external ID claim of a certificate
		// authority, which a PATCH removes unless it sets one.
		maps.DeleteFunc(fields, func(_ string, v any) bool { return v == nil })
		if _, ok := fields["externalIdClaim"]; !ok {
			delete(entity, "externalIdClaim")
		}
	}
	maps.Copy(entity, fields)
	writeData(w, http.StatusOK, map[string]any{})
}

func (s *Server) replace(w http.ResponseWriter, r *http.Request, collection, id string) {
	old, ok := s.collections[collection][id]
	if !ok {
		writeError(w, http.StatusNotFound, "NOT_FOUND", nil)
		return
	}

	entity := map[string]any{}
	if err := json.NewDecoder(r.Body).Decode(&entity); err != nil {
		writeError(w, http.StatusBadRequest, "COULD_NOT_PARSE_BODY", nil)
		return
	}
	entity["id"] = id
	if created, ok := old["createdAt"]; ok {
		entity["createdAt"] = created
	}
	for _, field := range unchangeable[collection] {
		if v, ok := old[field]; ok {
			entity[field] = v
		}
	}
	setDefaults(collection, entity)
	s.put(collection, entity)
	writeData(w, http.StatusOK, map[string]any{})
}

func (s *Server) remove(w http.ResponseWriter, collection, id string) {
	entity, ok := s.collections[collection][id]
	if !ok {
		writeError(w, http.StatusNotFound, "NOT_FOUND", nil)
		return
	}
	// Like the real controller, an identity that has hosting settings for a
	// service that is gone cannot be deleted: the service is "not found".
	if missing := s.missingService(entity); collection == "identities" && missing != "" {
		writeError(w, http.StatusNotFound, "NOT_FOUND", map[string]any{"field": "id", "type": "service", "value": missing})
		return
	}
	delete(s.collections[collection], id)
	writeData(w, http.StatusOK, map[string]any{})
}

// missingService returns the ID of a service the hosting settings of an
// identity refer to that does not exist, if any.
func (s *Server) missingService(identity map[string]any) string {
	for _, field := range []string{"serviceHostingCosts", "serviceHostingPrecedences"} {
		perService, _ := identity[field].(map[string]any)
		for id := range perService {
			if _, ok := s.collections["services"][id]; !ok {
				return id
			}
		}
	}
	return ""
}

// setDefaults fills in the defaults of the controller for the fields of an
// entity that are empty.
func setDefaults(collection string, entity map[string]any) {
	for field, value := range defaults[collection] {
		if v := entity[field]; v == nil || v == "" {
			entity[field] = value
		}
	}
}

func writeData(w http.ResponseWriter, status int, data any) {
	write(w, status, map[string]any{"data": data, "meta": map[string]any{}})
}

func writeError(w http.ResponseWriter, status int, code string, cause any) {
	write(w, status, map[string]any{"error": map[string]any{"code": code, "message": "fake " + code, "cause": cause}})
}

func write(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		panic(err)
	}
}
