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

	"github.com/crossplane/provider-ziti/internal/client"
)

const (
	// Username accepted by the fake controller.
	Username = "admin"
	// Password accepted by the fake controller.
	Password = "admin"

	prefix = "/edge/management/v1"
)

var nameFilter = regexp.MustCompile(`^name="((?:[^"\\]|\\.)*)"$`)

// Server is a fake Ziti controller. Entities are stored as the JSON documents
// that were posted to it, plus an id.
type Server struct {
	*httptest.Server

	mu          sync.Mutex
	collections map[string]map[string]map[string]any
	sessions    map[string]bool
	nextID      int
	logins      int
	requests    []string
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

	// The controller turns an enrollment request into an enrollment token.
	if enrollment, ok := entity["enrollment"].(map[string]any); ok && enrollment["ott"] == true {
		entity["enrollment"] = map[string]any{"ott": map[string]any{
			"jwt":       "jwt-for-" + fmt.Sprint(entity["name"]),
			"expiresAt": "2030-01-01T00:00:00.000Z",
		}}
	}
	if collection == "edge-routers" {
		entity["enrollmentJwt"] = "jwt-for-" + fmt.Sprint(entity["name"])
	}

	s.put(collection, entity)
	writeData(w, http.StatusCreated, map[string]any{"id": entity["id"]})
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
	maps.Copy(entity, fields)
	writeData(w, http.StatusOK, map[string]any{})
}

func (s *Server) remove(w http.ResponseWriter, collection, id string) {
	if _, ok := s.collections[collection][id]; !ok {
		writeError(w, http.StatusNotFound, "NOT_FOUND", nil)
		return
	}
	delete(s.collections[collection], id)
	writeData(w, http.StatusOK, map[string]any{})
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
