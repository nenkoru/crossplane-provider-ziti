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
	"context"
	"encoding/json"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	kube "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	errNotModern     = "managed resource does not reference a provider config"
	errEntityName    = "cannot determine the name of the external resource"
	errFindByName    = "cannot look for the external resource by name"
	errUpdateManaged = "cannot update the managed resource"

	// AnnotationKeyCreateUnconfirmed holds the time a request to create the
	// entity was sent that the controller never answered: the request may or
	// may not have taken effect.
	AnnotationKeyCreateUnconfirmed = "ziti.crossplane.io/create-unconfirmed"

	// clockTolerance is how inexact the comparison of the clock of the Ziti
	// controller with the local one is: the times involved are cut to the
	// second, and a response takes a moment to arrive.
	clockTolerance = 5 * time.Second
)

// A CreationRecoverer settles a creation whose result is unknown. Either the
// provider stopped after it asked Ziti to create an entity and before it
// saved the ID Ziti assigned, or the answer of Ziti never arrived. The
// managed resource cannot find its entity without the ID, and cannot create
// it again because names are unique within a collection.
//
// For the same reason the entity, if it was created, is the one that has the
// name the managed resource asks for. It is taken over only if it was created
// after the creation in question started, going by the clock of the
// controller. An entity that was there before belongs to someone else and is
// left alone: its name was taken, so the creation cannot have succeeded, and
// the next attempt reports the conflict.
type CreationRecoverer struct {
	kube       kube.Client
	clients    Connecter
	collection string
}

// NewCreationRecoverer returns an initializer that settles creations of
// entities in the supplied collection of the Ziti API whose result is
// unknown.
func NewCreationRecoverer(k kube.Client, clients Connecter, collection string) *CreationRecoverer {
	return &CreationRecoverer{kube: k, clients: clients, collection: collection}
}

// Initialize records the result of a creation whose result is unknown, if
// there was one.
func (r *CreationRecoverer) Initialize(ctx context.Context, mg resource.Managed) error {
	started, incomplete := unsettled(mg)
	if started.IsZero() {
		return nil
	}

	name, err := entityName(mg)
	if err != nil {
		return errors.Wrap(err, errEntityName)
	}
	if name == "" {
		return nil
	}

	entity, ahead, err := r.find(ctx, mg, name)
	if err != nil {
		return err
	}

	switch {
	case entity == nil && !incomplete:
		// The request may still take effect, so the annotation stays until
		// an entity with the name exists.
		return nil
	case entity == nil:
		// Nothing was created, so it is safe to create the entity.
		meta.SetExternalCreateFailed(mg, time.Now())
	case entity.ID == "" || entity.CreatedAt.Before(started.Add(ahead-clockTolerance)):
		// The name was taken before the creation started.
		meta.SetExternalCreateFailed(mg, time.Now())
		meta.RemoveAnnotations(mg, AnnotationKeyCreateUnconfirmed)
	default:
		meta.SetExternalName(mg, entity.ID)
		meta.SetExternalCreateSucceeded(mg, time.Now())
		meta.RemoveAnnotations(mg, AnnotationKeyCreateUnconfirmed)
	}
	return errors.Wrap(r.kube.Update(ctx, mg), errUpdateManaged)
}

// unsettled returns when a creation whose result is unknown started, on the
// local clock, and whether the reconciler refuses to proceed because of it.
// The time is zero if there is no such creation.
func unsettled(mg resource.Managed) (started time.Time, incomplete bool) {
	if meta.ExternalCreateIncomplete(mg) {
		started, incomplete = meta.GetExternalCreatePending(mg), true
	}

	// A request that was not answered is the earlier of the two if both are
	// recorded: the entity may stem from either.
	unconfirmed, err := time.Parse(time.RFC3339, mg.GetAnnotations()[AnnotationKeyCreateUnconfirmed])
	if err == nil && (started.IsZero() || unconfirmed.Before(started)) {
		started = unconfirmed
	}
	return started, incomplete
}

// A namedEntity is what tells whether an entity stems from a creation whose
// result is unknown.
type namedEntity struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
}

// find returns the entity with the supplied name in the Ziti controller of
// the managed resource, or nil if there is none, and how far the clock of
// that controller is ahead of the local one.
func (r *CreationRecoverer) find(ctx context.Context, mg resource.Managed, name string) (*namedEntity, time.Duration, error) {
	modern, ok := mg.(resource.ModernManaged)
	if !ok {
		return nil, 0, errors.New(errNotModern)
	}

	api, err := r.clients.Connect(ctx, modern)
	if err != nil {
		return nil, 0, errors.Wrap(err, errConnect)
	}

	raw, err := api.FindByName(ctx, r.collection, name)
	if err != nil {
		return nil, 0, errors.Wrap(err, errFindByName)
	}
	// A controller that does not tell the time is taken to agree with the
	// local clock.
	ahead, _ := api.ClockAhead()
	if raw == nil {
		return nil, ahead, nil
	}

	entity := &namedEntity{}
	if err := json.Unmarshal(raw, entity); err != nil {
		return nil, 0, errors.Wrap(err, errParse)
	}
	return entity, ahead, nil
}

// entityName returns the name a managed resource gives its Ziti entity. Every
// kind keeps it in spec.forProvider.name.
func entityName(mg resource.Managed) (string, error) {
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(mg)
	if err != nil {
		return "", err
	}
	name, _, err := unstructured.NestedString(u, "spec", "forProvider", "name")
	return name, err
}
