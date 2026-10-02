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

// Package posturecheckos implements the Ziti PostureCheckOS managed resource.
package posturecheckos

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

// Setup adds a controller that reconciles PostureCheckOS managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(zitiv1alpha1.PostureCheckOSGroupKind)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		For(&zitiv1alpha1.PostureCheckOS{}).
		Complete(managed.NewReconciler(mgr,
			resource.ManagedKind(zitiv1alpha1.PostureCheckOSGroupVersionKind),
			managed.WithExternalConnecter(&posturecheckosConnector{kube: mgr}),
			managed.WithLogger(o.Logger.WithValues("controller", name)),
			managed.WithPollInterval(o.PollInterval),
			managed.WithDeterministicExternalName(false),
		))
}

type posturecheckosConnector struct {
	kube ctrl.Manager
}

func (c *posturecheckosConnector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	r := mg.(*zitiv1alpha1.PostureCheckOS)
	conn := connector.ZitiConnector{Kube: c.kube}
	zc, err := conn.Connect(ctx, r.Spec.ProviderConfigReference, r.GetNamespace())
	if err != nil {
		return nil, err
	}
	return &external{client: zc}, nil
}

func (c *posturecheckosConnector) Disconnect(ctx context.Context, mg resource.Managed) error {
	return nil
}

type external struct{ client *client.ZitiClient }

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	r := mg.(*zitiv1alpha1.PostureCheckOS)
	log := ctrllog.FromContext(ctx)
	id := meta.GetExternalName(mg)
	if id == "" || id == r.Name {
		// Empty or set by NameAsExternalName initializer (K8s name, not Ziti ID)
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	resp, err := e.client.Read(client.MgmtPath("posture-check", id))
	if err != nil {
		if resource.IgnoreNotFound(err) == nil {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		return managed.ExternalObservation{}, err
	}
	var pc postureResponse
	if err := json.Unmarshal(resp, &pc); err != nil {
		return managed.ExternalObservation{}, fmt.Errorf("cannot unmarshal: %w", err)
	}
	r.Status.AtProvider.ID = pc.Data.ID
	r.Status.AtProvider.Name = pc.Data.Name
	r.Status.AtProvider.Type = pc.Data.Type
	r.Status.AtProvider.CreatedAt = pc.Data.CreatedAt
	r.Status.AtProvider.UpdatedAt = pc.Data.UpdatedAt
	log.V(2).Info("Observed posture check", "id", pc.Data.ID)
	return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: isUpToDate(r, &pc.Data)}, nil
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	r := mg.(*zitiv1alpha1.PostureCheckOS)
	ops := make([]map[string]interface{}, len(r.Spec.ForProvider.OperatingSystems))
	for i, os := range r.Spec.ForProvider.OperatingSystems {
		ops[i] = map[string]interface{}{"type": os.Type, "versions": os.Versions}
	}
	p := postureCreatePayload{
		Name:             r.Spec.ForProvider.Name,
		TypeId:           "OS",
		OperatingSystems: ops,
		RoleAttributes:   r.Spec.ForProvider.RoleAttributes,
	}
	resp, err := e.client.Create(client.MgmtPathCustom("posture-checks"), p)
	if err != nil {
		return managed.ExternalCreation{}, fmt.Errorf("cannot create: %w", err)
	}
	var cr postureResponse
	json.Unmarshal(resp, &cr)
	meta.SetExternalName(mg, cr.Data.ID)
	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	r := mg.(*zitiv1alpha1.PostureCheckOS)
	id := meta.GetExternalName(mg)
	ops := make([]map[string]interface{}, len(r.Spec.ForProvider.OperatingSystems))
	for i, os := range r.Spec.ForProvider.OperatingSystems {
		ops[i] = map[string]interface{}{"type": os.Type, "versions": os.Versions}
	}
	p := postureUpdatePayload{
		Name:             r.Spec.ForProvider.Name,
		OperatingSystems: ops,
		RoleAttributes:   r.Spec.ForProvider.RoleAttributes,
	}
	_, err := e.client.Update(client.MgmtPathCustom("posture-checks/"+id), p)
	return managed.ExternalUpdate{}, err
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	id := meta.GetExternalName(mg)
	if id == "" {
		return managed.ExternalDelete{}, nil
	}
	_, err := e.client.Delete(client.MgmtPathCustom("posture-checks/" + id))
	if resource.IgnoreNotFound(err) != nil {
		return managed.ExternalDelete{}, err
	}
	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(ctx context.Context) error { return nil }

func isUpToDate(r *zitiv1alpha1.PostureCheckOS, pc *postureData) bool {
	return r.Spec.ForProvider.Name == pc.Name
}

type postureResponse struct {
	Data postureData `json:"data"`
}
type postureData struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Type           string          `json:"type"`
	RoleAttributes json.RawMessage `json:"roleAttributes"`
	CreatedAt      string          `json:"createdAt"`
	UpdatedAt      string          `json:"updatedAt"`
}
type postureCreatePayload struct {
	Name             string                   `json:"name"`
	TypeId           string                   `json:"typeId"`
	OperatingSystems []map[string]interface{} `json:"operatingSystems"`
	RoleAttributes   []string                 `json:"roleAttributes,omitempty"`
}
type postureUpdatePayload struct {
	Name             string                   `json:"name,omitempty"`
	OperatingSystems []map[string]interface{} `json:"operatingSystems,omitempty"`
	RoleAttributes   []string                 `json:"roleAttributes,omitempty"`
}
