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

// Package configinterceptv1 implements the Ziti ConfigInterceptV1 managed resource.
package configinterceptv1

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

// Setup adds a controller that reconciles ConfigInterceptV1 managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(zitiv1alpha1.ConfigInterceptV1GroupKind)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		For(&zitiv1alpha1.ConfigInterceptV1{}).
		Complete(managed.NewReconciler(mgr,
			resource.ManagedKind(zitiv1alpha1.ConfigInterceptV1GroupVersionKind),
			managed.WithExternalConnecter(&configinterceptv1Connector{kube: mgr}),
			managed.WithLogger(o.Logger.WithValues("controller", name)),
			managed.WithPollInterval(o.PollInterval),
			managed.WithDeterministicExternalName(false),
		))
}

type configinterceptv1Connector struct {
	kube ctrl.Manager
}

func (c *configinterceptv1Connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	r := mg.(*zitiv1alpha1.ConfigInterceptV1)
	conn := connector.ZitiConnector{Kube: c.kube}
	zc, err := conn.Connect(ctx, r.Spec.ProviderConfigReference, r.GetNamespace())
	if err != nil {
		return nil, err
	}
	return &external{client: zc}, nil
}

func (c *configinterceptv1Connector) Disconnect(ctx context.Context, mg resource.Managed) error {
	return nil
}

type external struct{ client *client.ZitiClient }

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	r := mg.(*zitiv1alpha1.ConfigInterceptV1)
	log := ctrllog.FromContext(ctx)
	id := meta.GetExternalName(mg)
	if id == "" || id == r.Name {
		// Empty or set by NameAsExternalName initializer (K8s name, not Ziti ID)
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	resp, err := e.client.ReadWithClient(client.MgmtPath("config", id))
	if err != nil {
		if resource.IgnoreNotFound(err) == nil {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		return managed.ExternalObservation{}, err
	}
	var cfg configResponse
	if err := json.Unmarshal(resp, &cfg); err != nil {
		return managed.ExternalObservation{}, fmt.Errorf("cannot unmarshal: %w", err)
	}

	data := cfg.Data.Data
	r.Status.AtProvider.ID = cfg.Data.ID
	r.Status.AtProvider.Name = cfg.Data.Name
	r.Status.AtProvider.CreatedAt = cfg.Data.CreatedAt
	r.Status.AtProvider.UpdatedAt = cfg.Data.UpdatedAt

	if v, ok := data["addresses"].([]interface{}); ok {
		r.Status.AtProvider.Addresses = toStringSlice(v)
	}
	if v, ok := data["protocols"].([]interface{}); ok {
		r.Status.AtProvider.Protocols = toStringSlice(v)
	}
	if v, ok := data["portRanges"].([]interface{}); ok {
		r.Status.AtProvider.PortRanges = toPortRangeSlice(v)
	}
	if v, ok := data["matchDomainStrategy"].(string); ok {
		r.Status.AtProvider.MatchDomainStrategy = v
	}
	if v, ok := data["dialOptions"].(map[string]interface{}); ok {
		r.Status.AtProvider.DialOptions = toDialOptions(v)
	}
	if v, ok := data["sourceIp"].(string); ok {
		r.Status.AtProvider.SourceIP = &v
	}
	if v, ok := data["roleAttributes"].([]interface{}); ok {
		r.Status.AtProvider.RoleAttributes = toStringSlice(v)
	}
	if v, ok := data["tags"].(map[string]interface{}); ok {
		r.Status.AtProvider.Tags = toStringMap(v)
	}

	log.V(2).Info("Observed config", "id", cfg.Data.ID)
	return managed.ExternalObservation{ResourceExists: true, ResourceUpToDate: isUpToDate(r, data)}, nil
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	r := mg.(*zitiv1alpha1.ConfigInterceptV1)
	log := ctrllog.FromContext(ctx)
	typesResp, err := e.client.ReadWithClient(client.MgmtPathCustom("config-types"))
	if err != nil {
		return managed.ExternalCreation{}, fmt.Errorf("cannot get config types: %w", err)
	}
	var typesRespData struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	json.Unmarshal(typesResp, &typesRespData)
	var configTypeID string
	for _, t := range typesRespData.Data {
		if t.Name == "intercept.v1" {
			configTypeID = t.ID
			break
		}
	}
	if configTypeID == "" {
		return managed.ExternalCreation{}, fmt.Errorf("config type intercept.v1 not found")
	}

	data := buildInterceptConfigData(r.Spec.ForProvider)
	p := configCreatePayload{
		Name:         r.Spec.ForProvider.Name,
		ConfigTypeID: configTypeID,
		Data:         data,
	}
	resp, err := e.client.CreateWithClient(client.MgmtPath("config", ""), p)
	if err != nil {
		return managed.ExternalCreation{}, fmt.Errorf("cannot create: %w", err)
	}
	var cr configResponse
	json.Unmarshal(resp, &cr)
	meta.SetExternalName(mg, cr.Data.ID)
	log.V(2).Info("Created config", "id", cr.Data.ID)
	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	r := mg.(*zitiv1alpha1.ConfigInterceptV1)
	id := meta.GetExternalName(mg)
	data := buildInterceptConfigData(r.Spec.ForProvider)
	p := configUpdatePayload{
		Name: r.Spec.ForProvider.Name,
		Data: data,
	}
	_, err := e.client.UpdateWithClient(client.MgmtPath("config", id), p)
	return managed.ExternalUpdate{}, err
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	id := meta.GetExternalName(mg)
	if id == "" {
		return managed.ExternalDelete{}, nil
	}
	log := ctrllog.FromContext(ctx)
	log.V(1).Info("Deleting intercept config from Ziti", "id", id)
	_, err := e.client.Delete(client.MgmtPath("config", id))
	if resource.IgnoreNotFound(err) != nil {
		return managed.ExternalDelete{}, err
	}
	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(ctx context.Context) error { return nil }

// isUpToDate checks if the current spec matches the observed state.
func isUpToDate(r *zitiv1alpha1.ConfigInterceptV1, data map[string]interface{}) bool {
	spec := r.Spec.ForProvider

	if !stringSliceEqual(spec.Addresses, getStringSlice(data, "addresses")) {
		return false
	}
	if !stringSliceEqual(spec.Protocols, getStringSlice(data, "protocols")) {
		return false
	}
	if !portRangeSliceEqual(spec.PortRanges, getPortRanges(data)) {
		return false
	}
	if spec.MatchDomainStrategy != nil && *spec.MatchDomainStrategy != getString(data, "matchDomainStrategy") {
		return false
	}
	if !dialOptionsEqual(spec.DialOptions, getDialOptions(data)) {
		return false
	}
	if spec.SourceIP != nil && *spec.SourceIP != getString(data, "sourceIp") {
		return false
	}
	if !stringSliceEqual(spec.RoleAttributes, getStringSlice(data, "roleAttributes")) {
		return false
	}
	if !stringMapEqual(spec.Tags, getStringMap(data, "tags")) {
		return false
	}
	return true
}

// buildInterceptConfigData builds the config data map from the spec.
func buildInterceptConfigData(p zitiv1alpha1.ConfigInterceptV1Parameters) map[string]interface{} {
	data := map[string]interface{}{
		"addresses": p.Addresses,
		"protocols": p.Protocols,
	}

	if len(p.PortRanges) > 0 {
		portRanges := make([]map[string]int32, len(p.PortRanges))
		for i, pr := range p.PortRanges {
			portRanges[i] = map[string]int32{"low": pr.Low, "high": pr.High}
		}
		data["portRanges"] = portRanges
	}
	if p.MatchDomainStrategy != nil {
		data["matchDomainStrategy"] = *p.MatchDomainStrategy
	}
	if p.DialOptions != nil {
		data["dialOptions"] = dialOptionsToMap(p.DialOptions)
	}
	if p.SourceIP != nil {
		data["sourceIp"] = *p.SourceIP
	}
	if len(p.RoleAttributes) > 0 {
		data["roleAttributes"] = p.RoleAttributes
	}
	if len(p.Tags) > 0 {
		data["tags"] = p.Tags
	}

	return data
}

// Helper functions for data conversion

func toStringSlice(v []interface{}) []string {
	result := make([]string, 0, len(v))
	for _, item := range v {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

func toStringMap(v map[string]interface{}) map[string]string {
	result := make(map[string]string, len(v))
	for k, val := range v {
		if s, ok := val.(string); ok {
			result[k] = s
		}
	}
	return result
}

func toPortRangeSlice(v []interface{}) []zitiv1alpha1.PortRange {
	result := make([]zitiv1alpha1.PortRange, 0, len(v))
	for _, item := range v {
		if m, ok := item.(map[string]interface{}); ok {
			result = append(result, zitiv1alpha1.PortRange{
				Low:  getInt32(m, "low"),
				High: getInt32(m, "high"),
			})
		}
	}
	return result
}

func toDialOptions(m map[string]interface{}) *zitiv1alpha1.DialOptions {
	return &zitiv1alpha1.DialOptions{
		ConnectTimeoutSeconds: getInt32Ptr(m, "connectTimeoutSeconds"),
		Identity:              getStringPtr(m, "identity"),
	}
}

// Getter helpers

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getInt32(m map[string]interface{}, key string) int32 {
	if v, ok := m[key].(float64); ok {
		return int32(v)
	}
	return 0
}

func getInt32Ptr(m map[string]interface{}, key string) *int32 {
	if v, ok := m[key].(float64); ok {
		i := int32(v)
		return &i
	}
	return nil
}

func getStringPtr(m map[string]interface{}, key string) *string {
	if v, ok := m[key].(string); ok {
		return &v
	}
	return nil
}

func getStringSlice(m map[string]interface{}, key string) []string {
	if v, ok := m[key].([]interface{}); ok {
		return toStringSlice(v)
	}
	return nil
}

func getStringMap(m map[string]interface{}, key string) map[string]string {
	if v, ok := m[key].(map[string]interface{}); ok {
		return toStringMap(v)
	}
	return nil
}

func getPortRanges(m map[string]interface{}) []zitiv1alpha1.PortRange {
	if v, ok := m["portRanges"].([]interface{}); ok {
		return toPortRangeSlice(v)
	}
	return nil
}

func getDialOptions(m map[string]interface{}) *zitiv1alpha1.DialOptions {
	if v, ok := m["dialOptions"].(map[string]interface{}); ok {
		return toDialOptions(v)
	}
	return nil
}

// Conversion helpers for Create/Update

func dialOptionsToMap(o *zitiv1alpha1.DialOptions) map[string]interface{} {
	m := make(map[string]interface{})
	if o.ConnectTimeoutSeconds != nil {
		m["connectTimeoutSeconds"] = *o.ConnectTimeoutSeconds
	}
	if o.Identity != nil {
		m["identity"] = *o.Identity
	}
	return m
}

// Equality helpers

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

func portRangeSliceEqual(a, b []zitiv1alpha1.PortRange) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Low != b[i].Low || a[i].High != b[i].High {
			return false
		}
	}
	return true
}

func dialOptionsEqual(a, b *zitiv1alpha1.DialOptions) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return int32PtrEqual(a.ConnectTimeoutSeconds, b.ConnectTimeoutSeconds) &&
		stringPtrEqual(a.Identity, b.Identity)
}

func int32PtrEqual(a, b *int32) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func stringPtrEqual(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// API payload types

type configResponse struct {
	Data configData `json:"data"`
}
type configData struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Data      map[string]interface{} `json:"data"`
	CreatedAt string                 `json:"createdAt"`
	UpdatedAt string                 `json:"updatedAt"`
}
type configCreatePayload struct {
	Name         string                 `json:"name"`
	ConfigTypeID string                 `json:"configTypeID"`
	Data         map[string]interface{} `json:"data"`
}
type configUpdatePayload struct {
	Name string                 `json:"name,omitempty"`
	Data map[string]interface{} `json:"data,omitempty"`
}
