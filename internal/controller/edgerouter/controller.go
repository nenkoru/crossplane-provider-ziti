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

// Package edgerouter implements the Ziti EdgeRouter managed resource.
package edgerouter

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

// Setup adds a controller that reconciles EdgeRouter managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(zitiv1alpha1.EdgeRouterGroupKind)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		For(&zitiv1alpha1.EdgeRouter{}).
		Complete(managed.NewReconciler(mgr,
			resource.ManagedKind(zitiv1alpha1.EdgeRouterGroupVersionKind),
			managed.WithExternalConnecter(&edgerouterConnector{kube: mgr}),
			managed.WithLogger(o.Logger.WithValues("controller", name)),
			managed.WithPollInterval(o.PollInterval),
			managed.WithDeterministicExternalName(false),
		))
}

type edgerouterConnector struct {
	kube ctrl.Manager
}

func (c *edgerouterConnector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	r := mg.(*zitiv1alpha1.EdgeRouter)
	conn := connector.ZitiConnector{Kube: c.kube}
	zc, err := conn.Connect(ctx, r.Spec.ProviderConfigReference, r.GetNamespace())
	if err != nil {
		return nil, err
	}
	return &external{client: zc}, nil
}

func (c *edgerouterConnector) Disconnect(ctx context.Context, mg resource.Managed) error { return nil }

type external struct{ client *client.ZitiClient }

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	r := mg.(*zitiv1alpha1.EdgeRouter)
	log := ctrllog.FromContext(ctx)
	id := meta.GetExternalName(mg)
	if id == "" || id == r.Name {
		// Empty or set by NameAsExternalName initializer (K8s name, not Ziti ID)
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	resp, err := e.client.Read(client.MgmtPath("edge-router", id))
	if err != nil {
		if resource.IgnoreNotFound(err) == nil {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		return managed.ExternalObservation{}, err
	}
	var er edgeResponse
	if err := json.Unmarshal(resp, &er); err != nil {
		return managed.ExternalObservation{}, fmt.Errorf("cannot unmarshal: %w", err)
	}
	r.Status.AtProvider.ID = er.Data.ID
	r.Status.AtProvider.Name = er.Data.Name
	r.Status.AtProvider.IsTunnelerEnabled = er.Data.IsTunnelerEnabled
	r.Status.AtProvider.NoTraversal = er.Data.NoTraversal
	r.Status.AtProvider.Cost = er.Data.Cost
	r.Status.AtProvider.CreatedAt = er.Data.CreatedAt
	r.Status.AtProvider.UpdatedAt = er.Data.UpdatedAt
	log.V(2).Info("Observed edge router", "id", er.Data.ID)
	return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: isUpToDate(r, &er.Data)}, nil
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	r := mg.(*zitiv1alpha1.EdgeRouter)
	p := edgeCreatePayload{Name: r.Spec.ForProvider.Name}
	if r.Spec.ForProvider.IsTunnelerEnabled != nil {
		p.IsTunnelerEnabled = *r.Spec.ForProvider.IsTunnelerEnabled
	}
	if r.Spec.ForProvider.NoTraversal != nil {
		p.NoTraversal = *r.Spec.ForProvider.NoTraversal
	}
	if r.Spec.ForProvider.Cost != nil {
		p.Cost = *r.Spec.ForProvider.Cost
	}
	resp, err := e.client.Create(client.MgmtPath("edge-router", ""), p)
	if err != nil {
		return managed.ExternalCreation{}, fmt.Errorf("cannot create: %w", err)
	}
	var cr edgeResponse
	json.Unmarshal(resp, &cr)
	meta.SetExternalName(mg, cr.Data.ID)
	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	r := mg.(*zitiv1alpha1.EdgeRouter)
	id := meta.GetExternalName(mg)
	p := edgeUpdatePayload{Name: r.Spec.ForProvider.Name}
	_, err := e.client.Update(client.MgmtPath("edge-router", id), p)
	return managed.ExternalUpdate{}, err
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	id := meta.GetExternalName(mg)
	if id == "" {
		return managed.ExternalDelete{}, nil
	}
	log := ctrllog.FromContext(ctx)
	log.V(1).Info("Deleting edge router from Ziti", "id", id)
	_, err := e.client.Delete(client.MgmtPath("edge-router", id))
	if resource.IgnoreNotFound(err) != nil {
		return managed.ExternalDelete{}, err
	}
	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(ctx context.Context) error { return nil }

func isUpToDate(r *zitiv1alpha1.EdgeRouter, er *edgeData) bool {
	return r.Spec.ForProvider.Name == er.Name
}

type edgeResponse struct {
	Data edgeData `json:"data"`
}
type edgeData struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	IsTunnelerEnabled bool   `json:"isTunnelerEnabled"`
	NoTraversal       bool   `json:"noTraversal"`
	Cost              int64  `json:"cost"`
	CreatedAt         string `json:"createdAt"`
	UpdatedAt         string `json:"updatedAt"`
}
type edgeCreatePayload struct {
	Name              string `json:"name"`
	IsTunnelerEnabled bool   `json:"isTunnelerEnabled,omitempty"`
	NoTraversal       bool   `json:"noTraversal,omitempty"`
	Cost              int64  `json:"cost,omitempty"`
}
type edgeUpdatePayload struct {
	Name string `json:"name,omitempty"`
}
