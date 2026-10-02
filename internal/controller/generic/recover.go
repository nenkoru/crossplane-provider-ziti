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

	// creationSkew is how far the clock of the Ziti controller may be behind
	// the clock of the provider.
	creationSkew = time.Minute
)

// A CreationRecoverer settles a creation whose result was never recorded: the
// provider stopped after it asked Ziti to create an entity and before it
// saved the ID Ziti assigned. The managed resource cannot find its entity
// without the ID, and cannot create it again because names are unique within
// a collection.
//
// For the same reason the entity, if it was created, is the one that has the
// name the managed resource asks for. It is taken over only if it is not
// older than the interrupted creation. An entity that was there before
// belongs to someone else: the managed resource is left as it is, and the
// reconciler keeps refusing to proceed until a person decides.
type CreationRecoverer struct {
	kube       kube.Client
	clients    Connecter
	collection string
}

// NewCreationRecoverer returns an initializer that settles interrupted
// creations of entities in the supplied collection of the Ziti API.
func NewCreationRecoverer(k kube.Client, clients Connecter, collection string) *CreationRecoverer {
	return &CreationRecoverer{kube: k, clients: clients, collection: collection}
}

// Initialize records the result of an interrupted creation, if there was one
// and its result can be determined.
func (r *CreationRecoverer) Initialize(ctx context.Context, mg resource.Managed) error {
	if !meta.ExternalCreateIncomplete(mg) {
		return nil
	}

	name, err := entityName(mg)
	if err != nil {
		return errors.Wrap(err, errEntityName)
	}
	if name == "" {
		return nil
	}

	entity, err := r.find(ctx, mg, name)
	if err != nil {
		return err
	}

	switch {
	case entity == nil:
		// Nothing was created, so it is safe to create the entity.
		meta.SetExternalCreateFailed(mg, time.Now())
	case entity.ID == "" || entity.CreatedAt.Before(meta.GetExternalCreatePending(mg).Add(-creationSkew)):
		return nil
	default:
		meta.SetExternalName(mg, entity.ID)
		meta.SetExternalCreateSucceeded(mg, time.Now())
	}
	return errors.Wrap(r.kube.Update(ctx, mg), errUpdateManaged)
}

// A namedEntity is what tells whether an entity was created by an
// interrupted creation.
type namedEntity struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
}

// find returns the entity with the supplied name in the Ziti controller of
// the managed resource, or nil if there is none.
func (r *CreationRecoverer) find(ctx context.Context, mg resource.Managed, name string) (*namedEntity, error) {
	modern, ok := mg.(resource.ModernManaged)
	if !ok {
		return nil, errors.New(errNotModern)
	}

	api, err := r.clients.Connect(ctx, modern)
	if err != nil {
		return nil, errors.Wrap(err, errConnect)
	}

	raw, err := api.FindByName(ctx, r.collection, name)
	if err != nil {
		return nil, errors.Wrap(err, errFindByName)
	}
	if raw == nil {
		return nil, nil
	}

	entity := &namedEntity{}
	if err := json.Unmarshal(raw, entity); err != nil {
		return nil, errors.Wrap(err, errParse)
	}
	return entity, nil
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
