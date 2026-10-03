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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
)

// A Fault is what the fake controller answers a request with in place of
// its result: an error, or no answer at all.
type Fault struct {
	// Method and Collection select the requests the fault applies to, such
	// as "PATCH" and "services". Empty ones select every method and every
	// collection.
	Method     string
	Collection string

	// Status is the HTTP status of the answer and Code the Ziti error code
	// in it. A status of zero closes the connection without an answer.
	Status int
	Code   string

	// CarriedOut makes the controller carry out the request before it
	// answers with the fault, as a proxy in between does that gives up
	// waiting for the controller.
	CarriedOut bool

	// Delay holds the answer back, so that a client with a shorter deadline
	// gives up waiting.
	Delay time.Duration
}

// Inject makes the fake controller answer requests with the supplied faults.
// Each fault applies to the first request it selects, in the order the
// faults were injected. Authentication is never affected.
func (s *Server) Inject(faults ...Fault) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.faults = append(s.faults, faults...)
}

// ClearFaults removes the faults that have not applied yet and returns them.
func (s *Server) ClearFaults() []Fault {
	s.mu.Lock()
	defer s.mu.Unlock()
	faults := s.faults
	s.faults = nil
	return faults
}

// takeFault returns the fault the supplied request is to be answered with,
// if any, and removes it.
func (s *Server) takeFault(r *http.Request) (Fault, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	path, ok := strings.CutPrefix(r.URL.Path, prefix)
	if !ok || path == "/authenticate" {
		return Fault{}, false
	}
	collection, _, _ := strings.Cut(strings.TrimPrefix(path, "/"), "/")
	for i, f := range s.faults {
		if (f.Method == "" || f.Method == r.Method) && (f.Collection == "" || f.Collection == collection) {
			s.faults = append(s.faults[:i:i], s.faults[i+1:]...)
			if !f.CarriedOut {
				s.requests = append(s.requests, r.Method+" "+path)
			}
			return f, true
		}
	}
	return Fault{}, false
}

// fail answers a request with a fault.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, f Fault) {
	if f.CarriedOut {
		s.serve(httptest.NewRecorder(), r)
	} else {
		// The server notices that the client gave up only once the body
		// has been read.
		_, _ = io.Copy(io.Discard, r.Body)
	}

	if f.Delay > 0 {
		select {
		case <-time.After(f.Delay):
		case <-r.Context().Done():
			return
		}
	}

	if f.Status != 0 {
		writeError(w, f.Status, f.Code, nil)
		return
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		panic("the fake controller cannot close the connection of a request")
	}
	conn, _, err := hijacker.Hijack()
	if err != nil {
		panic(err)
	}
	_ = conn.Close()
}
