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

// Package generic implements the reconciliation logic shared by all Ziti
// managed resources. Every kind maps to one collection of the Ziti Edge
// Management API and only describes how its spec translates to an entity of
// that collection and how an entity translates back to its status.
package generic

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"

	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"

	"github.com/crossplane/provider-ziti/internal/client"
)

const (
	errConnect = "cannot connect to the Ziti controller"
	errDesired = "cannot determine the desired state"
	errGet     = "cannot get the external resource"
	errParse   = "cannot parse the external resource"
	errObserve = "cannot observe the external resource"
	errCreate  = "cannot create the external resource"
	errUpdate  = "cannot update the external resource"
	errDelete  = "cannot delete the external resource"
)

// A Kind describes how a managed resource kind maps to the Ziti API.
type Kind[T resource.ModernManaged] struct {
	// GVK of the managed resource.
	GVK schema.GroupVersionKind

	// List is an empty list of the managed resource kind.
	List resource.ManagedList

	// Collection of the Ziti Edge Management API the kind is stored in, e.g.
	// "services".
	Collection string

	// Desired returns the fields of the Ziti entity the managed resource asks
	// for. They are sent when the entity is created and updated, and the
	// entity is up to date when it matches every one of them. It may use the
	// API to resolve references to other entities.
	Desired func(ctx context.Context, api *client.Client, mg T) (map[string]any, error)

	// CreateOnly optionally returns fields that are only sent when the entity
	// is created, because they are immutable or write-only.
	CreateOnly func(ctx context.Context, api *client.Client, mg T) (map[string]any, error)

	// Observe copies the supplied Ziti entity into the status of the managed
	// resource.
	Observe func(mg T, entity json.RawMessage) error

	// ConnectionDetails optionally extracts connection details from the
	// supplied Ziti entity.
	ConnectionDetails func(entity json.RawMessage) (managed.ConnectionDetails, error)
}

// A Connecter produces a Ziti client for a managed resource.
type Connecter interface {
	Connect(ctx context.Context, mg resource.ModernManaged) (*client.Client, error)
}

type connector[T resource.ModernManaged] struct {
	kind    Kind[T]
	clients Connecter
}

func (c *connector[T]) Connect(ctx context.Context, mg T) (managed.TypedExternalClient[T], error) {
	api, err := c.clients.Connect(ctx, mg)
	if err != nil {
		return nil, errors.Wrap(err, errConnect)
	}
	return NewExternalClient(c.kind, api), nil
}

// NewExternalClient returns an external client that reconciles managed
// resources of the supplied kind using the supplied Ziti client.
func NewExternalClient[T resource.ModernManaged](kind Kind[T], api *client.Client) managed.TypedExternalClient[T] {
	return &external[T]{kind: kind, api: api}
}

// external reconciles a managed resource with the Ziti entity whose ID is the
// external name of the managed resource.
type external[T resource.ModernManaged] struct {
	kind Kind[T]
	api  *client.Client
}

func (e *external[T]) Observe(ctx context.Context, mg T) (managed.ExternalObservation, error) {
	id := meta.GetExternalName(mg)
	if id == "" {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	entity, raw, err := e.get(ctx, id)
	if client.IsNotFound(err) {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	if err != nil {
		return managed.ExternalObservation{}, err
	}

	if err := e.kind.Observe(mg, raw); err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errObserve)
	}

	// The entities a resource refers to may already be gone when it is being
	// deleted, and its desired state no longer matters.
	if meta.WasDeleted(mg) {
		return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: true}, nil
	}

	o := managed.ExternalObservation{ResourceExists: true}
	if o.Diff, err = e.diff(ctx, mg, entity); err != nil {
		return managed.ExternalObservation{}, err
	}
	o.ResourceUpToDate = o.Diff == ""

	if e.kind.ConnectionDetails != nil {
		if o.ConnectionDetails, err = e.kind.ConnectionDetails(raw); err != nil {
			return managed.ExternalObservation{}, errors.Wrap(err, errObserve)
		}
	}

	mg.SetConditions(xpv2.Available())
	return o, nil
}

// diff describes the first field in which the entity differs from the desired
// state of the managed resource. It is empty if the entity is up to date.
func (e *external[T]) diff(ctx context.Context, mg T, entity map[string]any) (string, error) {
	desired, err := e.kind.Desired(ctx, e.api, mg)
	if err != nil {
		return "", errors.Wrap(err, errDesired)
	}
	body, err := normalize(desired)
	if err != nil {
		return "", errors.Wrap(err, errDesired)
	}

	field, ok := firstDifference(body, entity)
	if !ok {
		return "", nil
	}
	return fmt.Sprintf("field %q differs: desired %v, observed %v", field, body[field], entity[field]), nil
}

// get returns the entity with the supplied ID, both parsed and as JSON, with
// its tags normalized to strings.
func (e *external[T]) get(ctx context.Context, id string) (map[string]any, json.RawMessage, error) {
	raw, err := e.api.Get(ctx, e.kind.Collection, id)
	if err != nil {
		return nil, nil, errors.Wrap(err, errGet)
	}

	entity := map[string]any{}
	if err := json.Unmarshal(raw, &entity); err != nil {
		return nil, nil, errors.Wrap(err, errParse)
	}
	stringifyTags(entity)

	if raw, err = json.Marshal(entity); err != nil {
		return nil, nil, errors.Wrap(err, errParse)
	}
	return entity, raw, nil
}

func (e *external[T]) Create(ctx context.Context, mg T) (managed.ExternalCreation, error) {
	desired, err := e.kind.Desired(ctx, e.api, mg)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errDesired)
	}

	if e.kind.CreateOnly != nil {
		createOnly, err := e.kind.CreateOnly(ctx, e.api, mg)
		if err != nil {
			return managed.ExternalCreation{}, errors.Wrap(err, errDesired)
		}
		maps.Copy(desired, createOnly)
	}

	id, err := e.api.Create(ctx, e.kind.Collection, desired)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreate)
	}

	meta.SetExternalName(mg, id)
	return managed.ExternalCreation{}, nil
}

func (e *external[T]) Update(ctx context.Context, mg T) (managed.ExternalUpdate, error) {
	desired, err := e.kind.Desired(ctx, e.api, mg)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errDesired)
	}

	if err := e.api.Patch(ctx, e.kind.Collection, meta.GetExternalName(mg), desired); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdate)
	}
	return managed.ExternalUpdate{}, nil
}

func (e *external[T]) Delete(ctx context.Context, mg T) (managed.ExternalDelete, error) {
	id := meta.GetExternalName(mg)
	if id == "" {
		return managed.ExternalDelete{}, nil
	}

	if err := e.api.Delete(ctx, e.kind.Collection, id); err != nil && !client.IsNotFound(err) {
		return managed.ExternalDelete{}, errors.Wrap(err, errDelete)
	}
	return managed.ExternalDelete{}, nil
}

func (e *external[T]) Disconnect(_ context.Context) error {
	return nil
}

// stringifyTags makes sure all tag values are strings. Ziti also accepts
// booleans, numbers and null as tag values.
func stringifyTags(entity map[string]any) {
	tags, ok := entity["tags"].(map[string]any)
	if !ok {
		return
	}
	for k, v := range tags {
		if _, ok := v.(string); !ok {
			tags[k] = fmt.Sprint(v)
		}
	}
}
