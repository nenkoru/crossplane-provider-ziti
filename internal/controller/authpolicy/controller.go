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

// Package authpolicy implements the Ziti AuthPolicy managed resource.
package authpolicy

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

// Setup adds a controller that reconciles AuthPolicy managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(zitiv1alpha1.AuthPolicyGroupKind)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		For(&zitiv1alpha1.AuthPolicy{}).
		Complete(managed.NewReconciler(mgr,
			resource.ManagedKind(zitiv1alpha1.AuthPolicyGroupVersionKind),
			managed.WithExternalConnecter(&authpolicyConnector{kube: mgr}),
			managed.WithLogger(o.Logger.WithValues("controller", name)),
			managed.WithPollInterval(o.PollInterval),
			managed.WithDeterministicExternalName(false),
		))
}

type authpolicyConnector struct {
	kube ctrl.Manager
}

func (c *authpolicyConnector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	r := mg.(*zitiv1alpha1.AuthPolicy)
	conn := connector.ZitiConnector{Kube: c.kube}
	zc, err := conn.Connect(ctx, r.Spec.ProviderConfigReference, r.GetNamespace())
	if err != nil {
		return nil, err
	}
	return &external{client: zc}, nil
}

func (c *authpolicyConnector) Disconnect(ctx context.Context, mg resource.Managed) error { return nil }

type external struct{ client *client.ZitiClient }

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	r := mg.(*zitiv1alpha1.AuthPolicy)
	log := ctrllog.FromContext(ctx)
	id := meta.GetExternalName(mg)
	if id == "" || id == r.Name {
		// Empty or set by NameAsExternalName initializer (K8s name, not Ziti ID)
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	resp, err := e.client.Read(client.MgmtPath("auth-policy", id))
	if err != nil {
		if resource.IgnoreNotFound(err) == nil {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		return managed.ExternalObservation{}, err
	}
	var ap authResponse
	if err := json.Unmarshal(resp, &ap); err != nil {
		return managed.ExternalObservation{}, fmt.Errorf("cannot unmarshal: %w", err)
	}
	r.Status.AtProvider.ID = ap.Data.ID
	r.Status.AtProvider.Name = ap.Data.Name
	r.Status.AtProvider.CreatedAt = ap.Data.CreatedAt
	r.Status.AtProvider.UpdatedAt = ap.Data.UpdatedAt
	log.V(2).Info("Observed auth policy", "id", ap.Data.ID)
	return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: isUpToDate(r, &ap.Data)}, nil
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	r := mg.(*zitiv1alpha1.AuthPolicy)
	p := authCreatePayload{
		Name:           r.Spec.ForProvider.Name,
		Primary:        buildAuthMethods(r.Spec.ForProvider.Primary),
		Secondary:      buildAuthMethods(r.Spec.ForProvider.Secondary),
		RoleAttributes: r.Spec.ForProvider.RoleAttributes,
	}
	resp, err := e.client.Create(client.MgmtPath("auth-policies", ""), p)
	if err != nil {
		return managed.ExternalCreation{}, fmt.Errorf("cannot create: %w", err)
	}
	var cr authResponse
	json.Unmarshal(resp, &cr)
	meta.SetExternalName(mg, cr.Data.ID)
	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	r := mg.(*zitiv1alpha1.AuthPolicy)
	id := meta.GetExternalName(mg)
	p := authUpdatePayload{
		Name:      r.Spec.ForProvider.Name,
		Primary:   buildAuthMethods(r.Spec.ForProvider.Primary),
		Secondary: buildAuthMethods(r.Spec.ForProvider.Secondary),
	}
	_, err := e.client.Update(client.MgmtPath("auth-policy", id), p)
	return managed.ExternalUpdate{}, err
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	id := meta.GetExternalName(mg)
	if id == "" {
		return managed.ExternalDelete{}, nil
	}
	_, err := e.client.Delete(client.MgmtPath("auth-policy", id))
	if resource.IgnoreNotFound(err) != nil {
		return managed.ExternalDelete{}, err
	}
	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(ctx context.Context) error { return nil }

func isUpToDate(r *zitiv1alpha1.AuthPolicy, ap *authData) bool {
	return r.Spec.ForProvider.Name == ap.Name
}

func buildAuthMethods(m *zitiv1alpha1.AuthMethods) map[string]interface{} {
	if m == nil {
		return nil
	}
	p := map[string]interface{}{}
	if m.Cert != nil {
		p["cert"] = map[string]bool{"allowed": m.Cert.Allowed, "allowExpiredCerts": m.Cert.AllowExpiredCerts}
	}
	if m.UPDB != nil {
		p["updb"] = map[string]interface{}{
			"allowed":                m.UPDB.Allowed,
			"minPasswordLength":      m.UPDB.MinPasswordLength,
			"maxAttempts":            m.UPDB.MaxAttempts,
			"lockoutDurationMinutes": m.UPDB.LockoutDurationMinutes,
		}
	}
	if m.ExtJWT != nil {
		p["extJwt"] = map[string]interface{}{"allowed": m.ExtJWT.Allowed, "allowedSigners": m.ExtJWT.AllowedSigners}
	}
	return p
}

type authResponse struct{ Data authData `json:"data"` }
type authData struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Primary        json.RawMessage `json:"primary"`
	Secondary      json.RawMessage `json:"secondary"`
	RoleAttributes json.RawMessage `json:"roleAttributes"`
	CreatedAt      string          `json:"createdAt"`
	UpdatedAt      string          `json:"updatedAt"`
}
type authCreatePayload struct {
	Name           string                  `json:"name"`
	Primary        map[string]interface{}  `json:"primary,omitempty"`
	Secondary      map[string]interface{}  `json:"secondary,omitempty"`
	RoleAttributes []string                `json:"roleAttributes,omitempty"`
}
type authUpdatePayload struct {
	Name      string                  `json:"name,omitempty"`
	Primary   map[string]interface{}  `json:"primary,omitempty"`
	Secondary map[string]interface{}  `json:"secondary,omitempty"`
}
