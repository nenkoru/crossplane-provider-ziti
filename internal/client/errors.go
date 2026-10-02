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

package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// maxCauseLength bounds how much of an error cause ends up in conditions and events.
const maxCauseLength = 512

// Error is an error response of the Ziti Edge Management API.
type Error struct {
	// StatusCode is the HTTP status code of the response.
	StatusCode int

	// Code is the Ziti error code, e.g. NOT_FOUND or COULD_NOT_VALIDATE.
	Code string

	// Message is the human readable error message.
	Message string

	// Cause is the compact JSON cause of the error, if any.
	Cause string
}

func (e *Error) Error() string {
	msg := fmt.Sprintf("ziti API error %d", e.StatusCode)
	if e.Code != "" {
		msg += " " + e.Code
	}
	if e.Message != "" {
		msg += ": " + e.Message
	}
	if e.Cause != "" {
		msg += " (cause: " + e.Cause + ")"
	}
	return msg
}

// newError builds an Error from an error response body.
func newError(status int, body []byte) *Error {
	e := &Error{StatusCode: status}

	var envelope struct {
		Error struct {
			Code    string          `json:"code"`
			Message string          `json:"message"`
			Cause   json.RawMessage `json:"cause"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Error.Code == "" {
		e.Message = truncate(string(bytes.TrimSpace(body)))
		return e
	}

	e.Code = envelope.Error.Code
	e.Message = envelope.Error.Message

	if len(envelope.Error.Cause) > 0 && string(envelope.Error.Cause) != "null" {
		compact := &bytes.Buffer{}
		if err := json.Compact(compact, envelope.Error.Cause); err == nil {
			e.Cause = truncate(compact.String())
		}
	}
	return e
}

func truncate(s string) string {
	if len(s) <= maxCauseLength {
		return s
	}
	return s[:maxCauseLength] + "..."
}

// IsNotFound returns true if the error says the requested entity does not exist.
func IsNotFound(err error) bool {
	var apiErr *Error
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

// IsRejected returns true if the controller answered that it did not carry
// out the request. Any other error, such as a timeout or a failure of a proxy
// in between, leaves open whether the request took effect.
func IsRejected(err error) bool {
	var apiErr *Error
	return errors.As(err, &apiErr) && apiErr.StatusCode >= http.StatusBadRequest && apiErr.StatusCode < http.StatusInternalServerError
}

func isUnauthorized(err error) bool {
	var apiErr *Error
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusUnauthorized
}
