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

// Package service implements the Ziti Service managed resource.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

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

// Setup adds a controller that reconciles Service managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(zitiv1alpha1.ServiceGroupKind)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		For(&zitiv1alpha1.Service{}).
		Complete(managed.NewReconciler(mgr,
			resource.ManagedKind(zitiv1alpha1.ServiceGroupVersionKind),
			managed.WithExternalConnecter(&serviceConnector{kube: mgr}),
			managed.WithLogger(o.Logger.WithValues("controller", name)),
			managed.WithPollInterval(o.PollInterval),
			managed.WithDeterministicExternalName(false),
		))
}

// serviceConnector connects to the Ziti API.
type serviceConnector struct {
	kube ctrl.Manager
}

func (c *serviceConnector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	r := mg.(*zitiv1alpha1.Service)
	conn := connector.ZitiConnector{Kube: c.kube}
	zc, err := conn.Connect(ctx, r.Spec.ProviderConfigReference, r.GetNamespace())
	if err != nil {
		return nil, err
	}
	return &external{client: zc}, nil
}

func (c *serviceConnector) Disconnect(ctx context.Context, mg resource.Managed) error {
	return nil
}

// external handles the actual CRUD operations.
type external struct {
	client      *client.ZitiClient
	configCache map[string]string
	cacheTime   time.Time
	cacheMu     sync.RWMutex
}

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	r := mg.(*zitiv1alpha1.Service)
	log := ctrllog.FromContext(ctx)

	// Use external name annotation (set by Create) as the Ziti ID
	id := meta.GetExternalName(mg)
	if id == "" || id == r.Name {
		// Empty or set by NameAsExternalName initializer (K8s name, not Ziti ID)
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	resp, err := e.client.Read(client.MgmtPath("service", id))
	if err != nil {
		if resource.IgnoreNotFound(err) == nil || isNotFoundError(err) {
			log.V(2).Info("Service not found", "id", id)
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		return managed.ExternalObservation{}, err
	}

	var svc serviceResponse
	if err := json.Unmarshal(resp, &svc); err != nil {
		return managed.ExternalObservation{}, fmt.Errorf("cannot unmarshal service response: %w", err)
	}

	// Unmarshal json.RawMessage fields
	var configs []string
	_ = json.Unmarshal(svc.Data.Configs, &configs)
	var postureQueries []string
	_ = json.Unmarshal(svc.Data.PostureQueries, &postureQueries)
	var roleAttributes []string
	_ = json.Unmarshal(svc.Data.RoleAttributes, &roleAttributes)

	r.Status.AtProvider.ID = svc.Data.ID
	r.Status.AtProvider.Name = svc.Data.Name
	r.Status.AtProvider.EncryptionRequired = svc.Data.EncryptionRequired
	r.Status.AtProvider.MaxIdleTimeMillis = svc.Data.MaxIdleTimeMillis
	r.Status.AtProvider.TerminatorStrategy = svc.Data.TerminatorStrategy
	r.Status.AtProvider.Configs = configs
	r.Status.AtProvider.PostureQueries = postureQueries
	r.Status.AtProvider.RoleAttributes = roleAttributes
	r.Status.AtProvider.CreatedAt = svc.Data.CreatedAt
	r.Status.AtProvider.UpdatedAt = svc.Data.UpdatedAt
	r.Status.AtProvider.Tags = svc.Data.Tags

	log.V(2).Info("Observed service", "id", svc.Data.ID, "name", svc.Data.Name)

	upToDate := e.isUpToDate(r, configs, postureQueries, roleAttributes, svc.Data)
	log.V(1).Info("isUpToDate check", "upToDate", upToDate, "spec.encryptionRequired", r.Spec.ForProvider.EncryptionRequired, "actual.encryptionRequired", svc.Data.EncryptionRequired, "spec.terminatorStrategy", r.Spec.ForProvider.TerminatorStrategy, "actual.terminatorStrategy", svc.Data.TerminatorStrategy, "spec.maxIdle", r.Spec.ForProvider.MaxIdleTimeMillis, "actual.maxIdle", svc.Data.MaxIdleTimeMillis)

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: upToDate,
	}, nil
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	r := mg.(*zitiv1alpha1.Service)
	log := ctrllog.FromContext(ctx)

	payload := serviceCreatePayload{
		Name: r.Spec.ForProvider.Name,
	}

	if r.Spec.ForProvider.EncryptionRequired != nil {
		payload.EncryptionRequired = *r.Spec.ForProvider.EncryptionRequired
	} else {
		payload.EncryptionRequired = true
	}

	if r.Spec.ForProvider.MaxIdleTimeMillis != nil {
		payload.MaxIdleTimeMillis = *r.Spec.ForProvider.MaxIdleTimeMillis
	}

	if r.Spec.ForProvider.TerminatorStrategy != nil {
		payload.TerminatorStrategy = string(*r.Spec.ForProvider.TerminatorStrategy)
	}

	// Resolve config names to IDs
	if len(r.Spec.ForProvider.Configs) > 0 {
		configIDs, err := e.resolveConfigNames(r.Spec.ForProvider.Configs)
		if err != nil {
			return managed.ExternalCreation{}, fmt.Errorf("cannot resolve config names: %w", err)
		}
		payload.Configs = configIDs
	}

	if r.Spec.ForProvider.PostureQueries != nil {
		payload.PostureQueries = r.Spec.ForProvider.PostureQueries
	}

	if r.Spec.ForProvider.RoleAttributes != nil {
		payload.RoleAttributes = r.Spec.ForProvider.RoleAttributes
	}

	if r.Spec.ForProvider.Tags != nil {
		payload.Tags = r.Spec.ForProvider.Tags
	}

	resp, err := e.client.CreateWithClient(client.MgmtPath("service", ""), payload)
	if err != nil {
		return managed.ExternalCreation{}, fmt.Errorf("cannot create service: %w", err)
	}

	var createResp serviceResponse
	if err := json.Unmarshal(resp, &createResp); err != nil {
		return managed.ExternalCreation{}, fmt.Errorf("cannot unmarshal create response: %w", err)
	}

	log.V(2).Info("Created service", "id", createResp.Data.ID, "name", createResp.Data.Name)

	// Set the external name to the Ziti ID so Observe can find it
	meta.SetExternalName(mg, createResp.Data.ID)

	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	r := mg.(*zitiv1alpha1.Service)
	log := ctrllog.FromContext(ctx)
	id := meta.GetExternalName(mg)

	payload := serviceUpdatePayload{
		Name: r.Spec.ForProvider.Name,
	}

	if r.Spec.ForProvider.EncryptionRequired != nil {
		payload.EncryptionRequired = *r.Spec.ForProvider.EncryptionRequired
	}

	if r.Spec.ForProvider.MaxIdleTimeMillis != nil {
		payload.MaxIdleTimeMillis = *r.Spec.ForProvider.MaxIdleTimeMillis
	}

	if r.Spec.ForProvider.TerminatorStrategy != nil {
		payload.TerminatorStrategy = string(*r.Spec.ForProvider.TerminatorStrategy)
	}

	// Resolve config names to IDs (same as Create)
	if len(r.Spec.ForProvider.Configs) > 0 {
		configIDs, err := e.resolveConfigNames(r.Spec.ForProvider.Configs)
		if err != nil {
			return managed.ExternalUpdate{}, fmt.Errorf("cannot resolve config names: %w", err)
		}
		payload.Configs = configIDs
	}

	if r.Spec.ForProvider.PostureQueries != nil {
		payload.PostureQueries = r.Spec.ForProvider.PostureQueries
	}

	if r.Spec.ForProvider.RoleAttributes != nil {
		payload.RoleAttributes = r.Spec.ForProvider.RoleAttributes
	}

	if r.Spec.ForProvider.Tags != nil {
		payload.Tags = r.Spec.ForProvider.Tags
	}

	log.V(1).Info("Updating service", "id", id, "payload", payload)
	_, err := e.client.UpdateWithClient(client.MgmtPath("service", id), payload)
	if err != nil {
		log.Error(err, "Failed to update service", "id", id)
		return managed.ExternalUpdate{}, fmt.Errorf("cannot update service: %w", err)
	}

	log.V(2).Info("Updated service", "id", id)

	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	r := mg.(*zitiv1alpha1.Service)
	log := ctrllog.FromContext(ctx)
	id := meta.GetExternalName(mg)

	if id == "" || id == r.Name {
		// Not created yet or external name is K8s name (not Ziti ID)
		return managed.ExternalDelete{}, nil
	}

	log.V(1).Info("Deleting service from Ziti", "id", id)
	_, err := e.client.Delete(client.MgmtPath("service", id))
	if err != nil {
		if resource.IgnoreNotFound(err) != nil {
			return managed.ExternalDelete{}, fmt.Errorf("cannot delete service: %w", err)
		}
		log.V(2).Info("Service already deleted", "id", id)
		return managed.ExternalDelete{}, nil
	}

	log.V(2).Info("Deleted service", "id", id)
	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(ctx context.Context) error {
	return nil
}

// isUpToDate checks if the current spec matches the observed state.
func (e *external) isUpToDate(r *zitiv1alpha1.Service, configs, postureQueries, roleAttributes []string, svc serviceData) bool {
	if r.Spec.ForProvider.Name != svc.Name {
		return false
	}

	// Note: encryptionRequired cannot be changed after creation in Ziti API
	// Skip drift detection for this field

	if r.Spec.ForProvider.MaxIdleTimeMillis != nil && *r.Spec.ForProvider.MaxIdleTimeMillis != svc.MaxIdleTimeMillis {
		return false
	}

	if r.Spec.ForProvider.TerminatorStrategy != nil && string(*r.Spec.ForProvider.TerminatorStrategy) != svc.TerminatorStrategy {
		return false
	}

	// Resolve config names to IDs for comparison
	if len(r.Spec.ForProvider.Configs) > 0 {
		configIDs, err := e.resolveConfigNames(r.Spec.ForProvider.Configs)
		if err != nil {
			return false
		}

		if !stringSliceEqual(configIDs, configs) {
			return false
		}
	} else if len(configs) > 0 {
		// Spec has no configs but Ziti has configs
		return false
	}

	if !stringSliceEqual(r.Spec.ForProvider.PostureQueries, postureQueries) {
		return false
	}

	if !stringSliceEqual(r.Spec.ForProvider.RoleAttributes, roleAttributes) {
		return false
	}

	if !stringMapEqual(r.Spec.ForProvider.Tags, svc.Tags) {
		return false
	}

	return true
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func stringMapEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// API response types

type serviceResponse struct {
	Data serviceData `json:"data"`
}

type serviceData struct {
	ID                 string            `json:"id"`
	Name               string            `json:"name"`
	EncryptionRequired bool              `json:"encryptionRequired"`
	MaxIdleTimeMillis  int64             `json:"maxIdleTimeMillis"`
	TerminatorStrategy string            `json:"terminatorStrategy"`
	Configs            json.RawMessage   `json:"configs"`
	PostureQueries     json.RawMessage   `json:"postureQueries"`
	RoleAttributes     json.RawMessage   `json:"roleAttributes"`
	Tags               map[string]string `json:"tags"`
	CreatedAt          string            `json:"createdAt"`
	UpdatedAt          string            `json:"updatedAt"`
}

type serviceCreatePayload struct {
	Name               string            `json:"name"`
	EncryptionRequired bool              `json:"encryptionRequired"`
	MaxIdleTimeMillis  int64             `json:"maxIdleTimeMillis,omitempty"`
	TerminatorStrategy string            `json:"terminatorStrategy,omitempty"`
	Configs            []string          `json:"configs,omitempty"`
	PostureQueries     []string          `json:"postureQueries,omitempty"`
	RoleAttributes     []string          `json:"roleAttributes,omitempty"`
	Tags               map[string]string `json:"tags,omitempty"`
}

type serviceUpdatePayload struct {
	Name               string            `json:"name,omitempty"`
	EncryptionRequired bool              `json:"encryptionRequired,omitempty"`
	MaxIdleTimeMillis  int64             `json:"maxIdleTimeMillis,omitempty"`
	TerminatorStrategy string            `json:"terminatorStrategy,omitempty"`
	Configs            []string          `json:"configs,omitempty"`
	PostureQueries     []string          `json:"postureQueries,omitempty"`
	RoleAttributes     []string          `json:"roleAttributes,omitempty"`
	Tags               map[string]string `json:"tags,omitempty"`
}

// resolveConfigNames resolves config names to IDs by looking up existing configs.
// Uses caching to avoid excessive API calls.
func (e *external) resolveConfigNames(configNames []string) ([]string, error) {
	e.cacheMu.RLock()
	if time.Since(e.cacheTime) < 5*time.Minute && len(e.configCache) > 0 {
		// Cache is valid
		configIDs := make([]string, 0, len(configNames))
		for _, name := range configNames {
			if id, ok := e.configCache[name]; ok {
				configIDs = append(configIDs, id)
			} else {
				return nil, fmt.Errorf("config '%s' not found", name)
			}
		}
		e.cacheMu.RUnlock()
		return configIDs, nil
	}
	e.cacheMu.RUnlock()

	// Cache miss or expired, fetch from API
	e.cacheMu.Lock()
	defer e.cacheMu.Unlock()

	// Double-check after acquiring lock
	if time.Since(e.cacheTime) < 5*time.Minute && len(e.configCache) > 0 {
		configIDs := make([]string, 0, len(configNames))
		for _, name := range configNames {
			if id, ok := e.configCache[name]; ok {
				configIDs = append(configIDs, id)
			} else {
				return nil, fmt.Errorf("config '%s' not found", name)
			}
		}
		return configIDs, nil
	}

	var allConfigs []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	limit := 100
	offset := 0

	for {
		resp, err := e.client.ReadWithClient(client.MgmtPath("config", "") + fmt.Sprintf("?limit=%d&offset=%d", limit, offset))
		if err != nil {
			return nil, fmt.Errorf("cannot read configs: %w", err)
		}

		var listResp struct {
			Data []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"data"`
			Meta struct {
				Pagination struct {
					TotalCount int `json:"totalCount"`
					Limit      int `json:"limit"`
					Offset     int `json:"offset"`
				} `json:"pagination"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(resp, &listResp); err != nil {
			return nil, fmt.Errorf("cannot unmarshal config list: %w", err)
		}

		allConfigs = append(allConfigs, listResp.Data...)

		if len(listResp.Data) < limit {
			break
		}
		offset += limit
	}

	// Update cache
	e.configCache = make(map[string]string, len(allConfigs))
	for _, cfg := range allConfigs {
		e.configCache[cfg.Name] = cfg.ID
	}
	e.cacheTime = time.Now()

	configIDs := make([]string, 0, len(configNames))
	for _, name := range configNames {
		if id, ok := e.configCache[name]; ok {
			configIDs = append(configIDs, id)
		} else {
			return nil, fmt.Errorf("config '%s' not found", name)
		}
	}

	return configIDs, nil
}

// isNotFoundError checks if the error is a Ziti API not found error.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "404 Not Found") || strings.Contains(errStr, "Not Found") || strings.Contains(errStr, "not found")
}
