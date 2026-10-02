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

// Package servicepolicy implements the Ziti ServicePolicy managed resource.
package servicepolicy

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

// Setup adds a controller that reconciles ServicePolicy managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(zitiv1alpha1.ServicePolicyGroupKind)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		For(&zitiv1alpha1.ServicePolicy{}).
		Complete(managed.NewReconciler(mgr,
			resource.ManagedKind(zitiv1alpha1.ServicePolicyGroupVersionKind),
			managed.WithExternalConnecter(&servicepolicyConnector{kube: mgr}),
			managed.WithLogger(o.Logger.WithValues("controller", name)),
			managed.WithPollInterval(o.PollInterval),
			managed.WithDeterministicExternalName(false),
		))
}

type servicepolicyConnector struct {
	kube ctrl.Manager
}

func (c *servicepolicyConnector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	r := mg.(*zitiv1alpha1.ServicePolicy)
	conn := connector.ZitiConnector{Kube: c.kube}
	zc, err := conn.Connect(ctx, r.Spec.ProviderConfigReference, r.GetNamespace())
	if err != nil {
		return nil, err
	}
	return &external{client: zc}, nil
}

func (c *servicepolicyConnector) Disconnect(ctx context.Context, mg resource.Managed) error {
	return nil
}

type external struct{ client *client.ZitiClient }

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	r := mg.(*zitiv1alpha1.ServicePolicy)
	log := ctrllog.FromContext(ctx)
	id := meta.GetExternalName(mg)
	if id == "" || id == r.Name {
		// Empty or set by NameAsExternalName initializer (K8s name, not Ziti ID)
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	resp, err := e.client.Read(client.MgmtPath("service-policy", id))
	if err != nil {
		if resource.IgnoreNotFound(err) == nil {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		return managed.ExternalObservation{}, err
	}
	var sp policyResponse
	if err := json.Unmarshal(resp, &sp); err != nil {
		return managed.ExternalObservation{}, fmt.Errorf("cannot unmarshal: %w", err)
	}
	r.Status.AtProvider.ID = sp.Data.ID
	r.Status.AtProvider.Name = sp.Data.Name
	r.Status.AtProvider.Type = sp.Data.Type
	r.Status.AtProvider.Semantic = sp.Data.Semantic
	r.Status.AtProvider.CreatedAt = sp.Data.CreatedAt
	r.Status.AtProvider.UpdatedAt = sp.Data.UpdatedAt
	log.V(2).Info("Observed service policy", "id", sp.Data.ID)
	return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: isUpToDate(r, &sp.Data)}, nil
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	r := mg.(*zitiv1alpha1.ServicePolicy)
	p := policyCreatePayload{
		Name:          r.Spec.ForProvider.Name,
		Type:          string(r.Spec.ForProvider.Type),
		Semantic:      string(r.Spec.ForProvider.Semantic),
		ServiceRoles:  r.Spec.ForProvider.ServiceRoles,
		IdentityRoles: r.Spec.ForProvider.IdentityRoles,
	}
	resp, err := e.client.Create(client.MgmtPathCustom("service-policies"), p)
	if err != nil {
		return managed.ExternalCreation{}, fmt.Errorf("cannot create: %w", err)
	}
	var cr policyResponse
	json.Unmarshal(resp, &cr)
	meta.SetExternalName(mg, cr.Data.ID)
	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	r := mg.(*zitiv1alpha1.ServicePolicy)
	id := meta.GetExternalName(mg)
	p := policyUpdatePayload{
		Name:          r.Spec.ForProvider.Name,
		ServiceRoles:  r.Spec.ForProvider.ServiceRoles,
		IdentityRoles: r.Spec.ForProvider.IdentityRoles,
	}
	_, err := e.client.Update(client.MgmtPath("service-policy", id), p)
	return managed.ExternalUpdate{}, err
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	id := meta.GetExternalName(mg)
	if id == "" {
		return managed.ExternalDelete{}, nil
	}
	_, err := e.client.Delete(client.MgmtPath("service-policy", id))
	if resource.IgnoreNotFound(err) != nil {
		return managed.ExternalDelete{}, err
	}
	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(ctx context.Context) error { return nil }

func isUpToDate(r *zitiv1alpha1.ServicePolicy, sp *policyData) bool {
	return r.Spec.ForProvider.Name == sp.Name &&
		string(r.Spec.ForProvider.Type) == sp.Type &&
		string(r.Spec.ForProvider.Semantic) == sp.Semantic
}

type policyResponse struct {
	Data policyData `json:"data"`
}
type policyData struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Type          string `json:"type"`
	Semantic      string `json:"semantic"`
	ServiceRoles  string `json:"serviceRoles"`
	IdentityRoles string `json:"identityRoles"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}
type policyCreatePayload struct {
	Name          string   `json:"name"`
	Type          string   `json:"type"`
	Semantic      string   `json:"semantic"`
	ServiceRoles  []string `json:"serviceRoles"`
	IdentityRoles []string `json:"identityRoles"`
}
type policyUpdatePayload struct {
	Name          string   `json:"name,omitempty"`
	ServiceRoles  []string `json:"serviceRoles,omitempty"`
	IdentityRoles []string `json:"identityRoles,omitempty"`
}
