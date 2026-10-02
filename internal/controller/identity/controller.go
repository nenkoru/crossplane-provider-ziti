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

// Package identity implements the Ziti Identity managed resource.
package identity

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	zitiv1alpha1 "github.com/crossplane/provider-ziti/apis/v1alpha1"
	"github.com/crossplane/provider-ziti/internal/client"
	"github.com/crossplane/provider-ziti/internal/connector"
)

// Setup adds a controller that reconciles Identity managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(zitiv1alpha1.IdentityGroupKind)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		For(&zitiv1alpha1.Identity{}).
		Complete(managed.NewReconciler(mgr,
			resource.ManagedKind(zitiv1alpha1.IdentityGroupVersionKind),
			managed.WithExternalConnecter(&identityConnector{kube: mgr}),
			managed.WithLogger(o.Logger.WithValues("controller", name)),
			managed.WithPollInterval(o.PollInterval),
			managed.WithDeterministicExternalName(false),
		))
}

type identityConnector struct {
	kube ctrl.Manager
}

func (c *identityConnector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	r := mg.(*zitiv1alpha1.Identity)
	conn := connector.ZitiConnector{Kube: c.kube}
	zc, err := conn.Connect(ctx, r.Spec.ProviderConfigReference, r.GetNamespace())
	if err != nil {
		return nil, err
	}
	return &external{client: zc}, nil
}

func (c *identityConnector) Disconnect(ctx context.Context, mg resource.Managed) error { return nil }

type external struct{ client *client.ZitiClient }

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	r := mg.(*zitiv1alpha1.Identity)
	log := ctrllog.FromContext(ctx)
	id := meta.GetExternalName(mg)
	if id == "" || id == r.Name {
		// Empty or set by NameAsExternalName initializer (K8s name, not Ziti ID)
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	resp, err := e.client.Read(client.MgmtPath("identity", id))
	if err != nil {
		if resource.IgnoreNotFound(err) == nil {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		return managed.ExternalObservation{}, err
	}
	var ir identityResponse
	if err := json.Unmarshal(resp, &ir); err != nil {
		return managed.ExternalObservation{}, fmt.Errorf("cannot unmarshal: %w", err)
	}
	r.Status.AtProvider.ID = ir.Data.ID
	r.Status.AtProvider.Name = ir.Data.Name
	r.Status.AtProvider.IsAdmin = ir.Data.IsAdmin
	r.Status.AtProvider.RoleAttributes = ir.Data.RoleAttributes
	r.Status.AtProvider.CreatedAt = ir.Data.CreatedAt
	r.Status.AtProvider.UpdatedAt = ir.Data.UpdatedAt
	log.V(2).Info("Observed identity", "id", ir.Data.ID)
	return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: isUpToDate(r, &ir.Data)}, nil
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	r := mg.(*zitiv1alpha1.Identity)
	typeId := r.Spec.ForProvider.Type
	if typeId == "" {
		typeId = "Default"
	}
	p := identityCreatePayload{
		Name:           r.Spec.ForProvider.Name,
		Type:           typeId,
		IsAdmin:        r.Spec.ForProvider.IsAdmin,
		RoleAttributes: r.Spec.ForProvider.RoleAttributes,
	}
	resp, err := e.client.Create(client.MgmtPathCustom("identities"), p)
	if err != nil {
		return managed.ExternalCreation{}, fmt.Errorf("cannot create: %w", err)
	}
	var cr identityResponse
	json.Unmarshal(resp, &cr)
	meta.SetExternalName(mg, cr.Data.ID)
	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	r := mg.(*zitiv1alpha1.Identity)
	id := meta.GetExternalName(mg)
	p := identityUpdatePayload{
		Name:           r.Spec.ForProvider.Name,
		IsAdmin:        r.Spec.ForProvider.IsAdmin,
		RoleAttributes: r.Spec.ForProvider.RoleAttributes,
	}
	_, err := e.client.Update(client.MgmtPath("identity", id), p)
	return managed.ExternalUpdate{}, err
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	id := meta.GetExternalName(mg)
	if id == "" {
		return managed.ExternalDelete{}, nil
	}
	log := ctrllog.FromContext(ctx)
	log.V(1).Info("Deleting identity from Ziti", "id", id)
	_, err := e.client.Delete(client.MgmtPath("identity", id))
	if resource.IgnoreNotFound(err) != nil {
		return managed.ExternalDelete{}, err
	}
	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(ctx context.Context) error { return nil }

func isUpToDate(r *zitiv1alpha1.Identity, ir *identityData) bool {
	return r.Spec.ForProvider.Name == ir.Name
}

type identityResponse struct {
	Data identityData `json:"data"`
}
type identityData struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	IsAdmin        bool     `json:"isAdmin"`
	RoleAttributes []string `json:"roleAttributes"`
	CreatedAt      string   `json:"createdAt"`
	UpdatedAt      string   `json:"updatedAt"`
}
type identityCreatePayload struct {
	Name           string   `json:"name"`
	Type           string   `json:"type,omitempty"`
	IsAdmin        *bool    `json:"isAdmin,omitempty"`
	RoleAttributes []string `json:"roleAttributes,omitempty"`
}
type identityUpdatePayload struct {
	Name           string   `json:"name,omitempty"`
	IsAdmin        *bool    `json:"isAdmin,omitempty"`
	RoleAttributes []string `json:"roleAttributes,omitempty"`
}
