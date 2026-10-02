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

// Package confighostv1 implements the Ziti ConfigHostV1 managed resource.
package confighostv1

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

// Setup adds a controller that reconciles ConfigHostV1 managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(zitiv1alpha1.ConfigHostV1GroupKind)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		For(&zitiv1alpha1.ConfigHostV1{}).
		Complete(managed.NewReconciler(mgr,
			resource.ManagedKind(zitiv1alpha1.ConfigHostV1GroupVersionKind),
			managed.WithExternalConnecter(&confighostv1Connector{kube: mgr}),
			managed.WithLogger(o.Logger.WithValues("controller", name)),
			managed.WithPollInterval(o.PollInterval),
			managed.WithDeterministicExternalName(false),
		))
}

type confighostv1Connector struct {
	kube ctrl.Manager
}

func (c *confighostv1Connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	r := mg.(*zitiv1alpha1.ConfigHostV1)
	conn := connector.ZitiConnector{Kube: c.kube}
	zc, err := conn.Connect(ctx, r.Spec.ProviderConfigReference, r.GetNamespace())
	if err != nil {
		return nil, err
	}
	return &external{client: zc}, nil
}

func (c *confighostv1Connector) Disconnect(ctx context.Context, mg resource.Managed) error {
	return nil
}

type external struct{ client *client.ZitiClient }

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	r := mg.(*zitiv1alpha1.ConfigHostV1)
	log := ctrllog.FromContext(ctx)
	id := meta.GetExternalName(mg)
	if id == "" || id == r.Name {
		// Empty or set by NameAsExternalName initializer (K8s name, not Ziti ID)
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	resp, err := e.client.Read(client.MgmtPath("config", id))
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

	// Extract all fields from the config data
	data := cfg.Data.Data
	r.Status.AtProvider.ID = cfg.Data.ID
	r.Status.AtProvider.Name = cfg.Data.Name
	r.Status.AtProvider.CreatedAt = cfg.Data.CreatedAt
	r.Status.AtProvider.UpdatedAt = cfg.Data.UpdatedAt

	if v, ok := data["address"].(string); ok {
		r.Status.AtProvider.Address = v
	}
	if v, ok := data["port"].(float64); ok {
		r.Status.AtProvider.Port = int32(v)
	}
	if v, ok := data["protocol"].(string); ok {
		r.Status.AtProvider.Protocol = v
	}
	if v, ok := data["forwardProtocol"].(bool); ok {
		r.Status.AtProvider.ForwardProtocol = v
	}
	if v, ok := data["forwardPort"].(bool); ok {
		r.Status.AtProvider.ForwardPort = v
	}
	if v, ok := data["forwardAddress"].(bool); ok {
		r.Status.AtProvider.ForwardAddress = v
	}
	if v, ok := data["allowedProtocols"].([]interface{}); ok {
		r.Status.AtProvider.AllowedProtocols = toStringSlice(v)
	}
	if v, ok := data["allowedAddresses"].([]interface{}); ok {
		r.Status.AtProvider.AllowedAddresses = toStringSlice(v)
	}
	if v, ok := data["allowedSourceAddresses"].([]interface{}); ok {
		r.Status.AtProvider.AllowedSourceAddresses = toStringSlice(v)
	}
	if v, ok := data["forwardAddressTranslations"].([]interface{}); ok {
		r.Status.AtProvider.ForwardAddressTranslations = toForwardAddressTranslationSlice(v)
	}
	if v, ok := data["allowedPortRanges"].([]interface{}); ok {
		r.Status.AtProvider.AllowedPortRanges = toPortRangeSlice(v)
	}
	if v, ok := data["listenOptions"].(map[string]interface{}); ok {
		r.Status.AtProvider.ListenOptions = toListenOptions(v)
	}
	if v, ok := data["proxy"].(map[string]interface{}); ok {
		r.Status.AtProvider.Proxy = toProxyConfig(v)
	}
	if v, ok := data["httpChecks"].([]interface{}); ok {
		r.Status.AtProvider.HTTPChecks = toHTTPCheckSlice(v)
	}
	if v, ok := data["portChecks"].([]interface{}); ok {
		r.Status.AtProvider.PortChecks = toPortCheckSlice(v)
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
	r := mg.(*zitiv1alpha1.ConfigHostV1)
	log := ctrllog.FromContext(ctx)
	typesResp, err := e.client.Read(client.MgmtPathCustom("config-types"))
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
		if t.Name == "host.v1" {
			configTypeID = t.ID
			break
		}
	}
	if configTypeID == "" {
		return managed.ExternalCreation{}, fmt.Errorf("config type host.v1 not found")
	}

	data := buildConfigData(r.Spec.ForProvider)
	p := configCreatePayload{
		Name:         r.Spec.ForProvider.Name,
		ConfigTypeID: configTypeID,
		Data:         data,
	}
	if r.Spec.ForProvider.Tags != nil {
		p.Tags = r.Spec.ForProvider.Tags
	}
	resp, err := e.client.Create(client.MgmtPath("config", ""), p)
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
	r := mg.(*zitiv1alpha1.ConfigHostV1)
	id := meta.GetExternalName(mg)
	data := buildConfigData(r.Spec.ForProvider)
	p := configUpdatePayload{
		Name: r.Spec.ForProvider.Name,
		Data: data,
	}
	if r.Spec.ForProvider.Tags != nil {
		p.Tags = r.Spec.ForProvider.Tags
	}
	_, err := e.client.Update(client.MgmtPath("config", id), p)
	return managed.ExternalUpdate{}, err
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	id := meta.GetExternalName(mg)
	if id == "" {
		return managed.ExternalDelete{}, nil
	}
	log := ctrllog.FromContext(ctx)
	log.V(1).Info("Deleting config from Ziti", "id", id)
	_, err := e.client.Delete(client.MgmtPath("config", id))
	if resource.IgnoreNotFound(err) != nil {
		return managed.ExternalDelete{}, err
	}
	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(ctx context.Context) error { return nil }

// isUpToDate checks if the current spec matches the observed state.
func isUpToDate(r *zitiv1alpha1.ConfigHostV1, data map[string]interface{}) bool {
	spec := r.Spec.ForProvider

	if spec.Address != nil && *spec.Address != getString(data, "address") {
		return false
	}

	if spec.Port != nil && *spec.Port != getInt32(data, "port") {
		return false
	}

	if spec.Protocol != nil && *spec.Protocol != getString(data, "protocol") {
		return false
	}

	if spec.ForwardProtocol != nil && *spec.ForwardProtocol != getBool(data, "forwardProtocol") {
		return false
	}

	if spec.ForwardPort != nil && *spec.ForwardPort != getBool(data, "forwardPort") {
		return false
	}

	if spec.ForwardAddress != nil && *spec.ForwardAddress != getBool(data, "forwardAddress") {
		return false
	}

	if !stringSliceEqual(spec.AllowedProtocols, getStringSlice(data, "allowedProtocols")) {
		return false
	}

	if !stringSliceEqual(spec.AllowedAddresses, getStringSlice(data, "allowedAddresses")) {
		return false
	}

	if !stringSliceEqual(spec.AllowedSourceAddresses, getStringSlice(data, "allowedSourceAddresses")) {
		return false
	}

	if !forwardAddressTranslationsEqual(spec.ForwardAddressTranslations, getForwardAddressTranslations(data)) {
		return false
	}

	if !portRangeSliceEqual(spec.AllowedPortRanges, getPortRanges(data)) {
		return false
	}

	if !listenOptionsEqual(spec.ListenOptions, getListenOptions(data)) {
		return false
	}

	if !proxyConfigEqual(spec.Proxy, getProxyConfig(data)) {
		return false
	}

	if !httpChecksEqual(spec.HTTPChecks, getHTTPChecks(data)) {
		return false
	}

	if !portChecksEqual(spec.PortChecks, getPortChecks(data)) {
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

// buildConfigData builds the config data map from the spec.
func buildConfigData(p zitiv1alpha1.ConfigHostV1Parameters) map[string]interface{} {
	data := map[string]interface{}{
		"address":  p.Address,
		"port":     p.Port,
		"protocol": p.Protocol,
	}

	if p.ForwardProtocol != nil && *p.ForwardProtocol {
		data["forwardProtocol"] = true
	}
	if p.ForwardPort != nil && *p.ForwardPort {
		data["forwardPort"] = true
	}
	if p.ForwardAddress != nil && *p.ForwardAddress {
		data["forwardAddress"] = true
	}
	if len(p.AllowedProtocols) > 0 {
		data["allowedProtocols"] = p.AllowedProtocols
	}
	if len(p.AllowedAddresses) > 0 {
		data["allowedAddresses"] = p.AllowedAddresses
	}
	if len(p.AllowedSourceAddresses) > 0 {
		data["allowedSourceAddresses"] = p.AllowedSourceAddresses
	}
	if len(p.ForwardAddressTranslations) > 0 {
		data["forwardAddressTranslations"] = forwardAddressTranslationsToMap(p.ForwardAddressTranslations)
	}
	if len(p.AllowedPortRanges) > 0 {
		data["allowedPortRanges"] = portRangesToMap(p.AllowedPortRanges)
	}
	if p.ListenOptions != nil {
		data["listenOptions"] = listenOptionsToMap(p.ListenOptions)
	}
	if p.Proxy != nil {
		data["proxy"] = proxyConfigToMap(p.Proxy)
	}
	if len(p.HTTPChecks) > 0 {
		data["httpChecks"] = httpChecksToMap(p.HTTPChecks)
	}
	if len(p.PortChecks) > 0 {
		data["portChecks"] = portChecksToMap(p.PortChecks)
	}
	if p.ConfigTypeID != nil {
		data["configTypeId"] = *p.ConfigTypeID
	}
	if len(p.RoleAttributes) > 0 {
		data["roleAttributes"] = p.RoleAttributes
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

func toForwardAddressTranslationSlice(v []interface{}) []zitiv1alpha1.ForwardAddressTranslation {
	result := make([]zitiv1alpha1.ForwardAddressTranslation, 0, len(v))
	for _, item := range v {
		if m, ok := item.(map[string]interface{}); ok {
			result = append(result, zitiv1alpha1.ForwardAddressTranslation{
				From:         getString(m, "from"),
				To:           getString(m, "to"),
				PrefixLength: getInt32(m, "prefixLength"),
			})
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

func toListenOptions(m map[string]interface{}) *zitiv1alpha1.ListenOptions {
	return &zitiv1alpha1.ListenOptions{
		BindUsingEdgeIdentity: getBoolPtr(m, "bindUsingEdgeIdentity"),
		ConnectTimeout:        getStringPtr(m, "connectTimeout"),
		Cost:                  getInt32Ptr(m, "cost"),
		MaxConnections:        getInt32Ptr(m, "maxConnections"),
		Precedence:            getStringPtr(m, "precedence"),
	}
}

func toProxyConfig(m map[string]interface{}) *zitiv1alpha1.ProxyConfig {
	return &zitiv1alpha1.ProxyConfig{
		Address: getStringPtr(m, "address"),
		Type:    getStringPtr(m, "type"),
	}
}

func toHTTPCheckSlice(v []interface{}) []zitiv1alpha1.HTTPCheck {
	result := make([]zitiv1alpha1.HTTPCheck, 0, len(v))
	for _, item := range v {
		if m, ok := item.(map[string]interface{}); ok {
			result = append(result, zitiv1alpha1.HTTPCheck{
				URL:          getString(m, "url"),
				Method:       getString(m, "method"),
				Body:         getStringPtr(m, "body"),
				ExpectStatus: getInt32Ptr(m, "expectStatus"),
				ExpectInBody: getStringPtr(m, "expectInBody"),
				Interval:     getString(m, "interval"),
				Timeout:      getString(m, "timeout"),
				Actions:      toCheckActionSlice(getSlice(m, "actions")),
			})
		}
	}
	return result
}

func toPortCheckSlice(v []interface{}) []zitiv1alpha1.PortCheck {
	result := make([]zitiv1alpha1.PortCheck, 0, len(v))
	for _, item := range v {
		if m, ok := item.(map[string]interface{}); ok {
			result = append(result, zitiv1alpha1.PortCheck{
				Address:  getString(m, "address"),
				Interval: getString(m, "interval"),
				Timeout:  getString(m, "timeout"),
				Actions:  toCheckActionSlice(getSlice(m, "actions")),
			})
		}
	}
	return result
}

func toCheckActionSlice(v []interface{}) []zitiv1alpha1.CheckAction {
	result := make([]zitiv1alpha1.CheckAction, 0, len(v))
	for _, item := range v {
		if m, ok := item.(map[string]interface{}); ok {
			result = append(result, zitiv1alpha1.CheckAction{
				Trigger:           getString(m, "trigger"),
				Duration:          getString(m, "duration"),
				Action:            getString(m, "action"),
				ConsecutiveEvents: getInt32Ptr(m, "consecutiveEvents"),
			})
		}
	}
	return result
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

func getBool(m map[string]interface{}, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

func getSlice(m map[string]interface{}, key string) []interface{} {
	if v, ok := m[key].([]interface{}); ok {
		return v
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

func getStringPtr(m map[string]interface{}, key string) *string {
	if v, ok := m[key].(string); ok {
		return &v
	}
	return nil
}

func getInt32Ptr(m map[string]interface{}, key string) *int32 {
	if v, ok := m[key].(float64); ok {
		i := int32(v)
		return &i
	}
	return nil
}

func getBoolPtr(m map[string]interface{}, key string) *bool {
	if v, ok := m[key].(bool); ok {
		return &v
	}
	return nil
}

func getForwardAddressTranslations(m map[string]interface{}) []zitiv1alpha1.ForwardAddressTranslation {
	if v, ok := m["forwardAddressTranslations"].([]interface{}); ok {
		return toForwardAddressTranslationSlice(v)
	}
	return nil
}

func getPortRanges(m map[string]interface{}) []zitiv1alpha1.PortRange {
	if v, ok := m["allowedPortRanges"].([]interface{}); ok {
		return toPortRangeSlice(v)
	}
	return nil
}

func getListenOptions(m map[string]interface{}) *zitiv1alpha1.ListenOptions {
	if v, ok := m["listenOptions"].(map[string]interface{}); ok {
		return toListenOptions(v)
	}
	return nil
}

func getProxyConfig(m map[string]interface{}) *zitiv1alpha1.ProxyConfig {
	if v, ok := m["proxy"].(map[string]interface{}); ok {
		return toProxyConfig(v)
	}
	return nil
}

func getHTTPChecks(m map[string]interface{}) []zitiv1alpha1.HTTPCheck {
	if v, ok := m["httpChecks"].([]interface{}); ok {
		return toHTTPCheckSlice(v)
	}
	return nil
}

func getPortChecks(m map[string]interface{}) []zitiv1alpha1.PortCheck {
	if v, ok := m["portChecks"].([]interface{}); ok {
		return toPortCheckSlice(v)
	}
	return nil
}

// Conversion helpers for Create/Update

func forwardAddressTranslationsToMap(translations []zitiv1alpha1.ForwardAddressTranslation) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(translations))
	for _, t := range translations {
		result = append(result, map[string]interface{}{
			"from":         t.From,
			"to":           t.To,
			"prefixLength": t.PrefixLength,
		})
	}
	return result
}

func portRangesToMap(ranges []zitiv1alpha1.PortRange) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(ranges))
	for _, r := range ranges {
		result = append(result, map[string]interface{}{
			"low":  r.Low,
			"high": r.High,
		})
	}
	return result
}

func listenOptionsToMap(o *zitiv1alpha1.ListenOptions) map[string]interface{} {
	m := make(map[string]interface{})
	if o.BindUsingEdgeIdentity != nil {
		m["bindUsingEdgeIdentity"] = *o.BindUsingEdgeIdentity
	}
	if o.ConnectTimeout != nil {
		m["connectTimeout"] = *o.ConnectTimeout
	}
	if o.Cost != nil {
		m["cost"] = *o.Cost
	}
	if o.MaxConnections != nil {
		m["maxConnections"] = *o.MaxConnections
	}
	if o.Precedence != nil {
		m["precedence"] = *o.Precedence
	}
	return m
}

func proxyConfigToMap(p *zitiv1alpha1.ProxyConfig) map[string]interface{} {
	m := make(map[string]interface{})
	if p.Address != nil {
		m["address"] = *p.Address
	}
	if p.Type != nil {
		m["type"] = *p.Type
	}
	return m
}

func httpChecksToMap(checks []zitiv1alpha1.HTTPCheck) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(checks))
	for _, c := range checks {
		m := map[string]interface{}{
			"url":      c.URL,
			"method":   c.Method,
			"interval": c.Interval,
			"timeout":  c.Timeout,
		}
		if c.Body != nil {
			m["body"] = *c.Body
		}
		if c.ExpectStatus != nil {
			m["expectStatus"] = *c.ExpectStatus
		}
		if c.ExpectInBody != nil {
			m["expectInBody"] = *c.ExpectInBody
		}
		if len(c.Actions) > 0 {
			m["actions"] = checkActionsToMap(c.Actions)
		}
		result = append(result, m)
	}
	return result
}

func portChecksToMap(checks []zitiv1alpha1.PortCheck) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(checks))
	for _, c := range checks {
		m := map[string]interface{}{
			"address":  c.Address,
			"interval": c.Interval,
			"timeout":  c.Timeout,
		}
		if len(c.Actions) > 0 {
			m["actions"] = checkActionsToMap(c.Actions)
		}
		result = append(result, m)
	}
	return result
}

func checkActionsToMap(actions []zitiv1alpha1.CheckAction) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(actions))
	for _, a := range actions {
		m := map[string]interface{}{
			"trigger":  a.Trigger,
			"duration": a.Duration,
			"action":   a.Action,
		}
		if a.ConsecutiveEvents != nil {
			m["consecutiveEvents"] = *a.ConsecutiveEvents
		}
		result = append(result, m)
	}
	return result
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

func forwardAddressTranslationsEqual(a []zitiv1alpha1.ForwardAddressTranslation, b []zitiv1alpha1.ForwardAddressTranslation) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].From != b[i].From || a[i].To != b[i].To || a[i].PrefixLength != b[i].PrefixLength {
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

func listenOptionsEqual(a, b *zitiv1alpha1.ListenOptions) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return boolPtrEqual(a.BindUsingEdgeIdentity, b.BindUsingEdgeIdentity) &&
		stringPtrEqual(a.ConnectTimeout, b.ConnectTimeout) &&
		int32PtrEqual(a.Cost, b.Cost) &&
		int32PtrEqual(a.MaxConnections, b.MaxConnections) &&
		stringPtrEqual(a.Precedence, b.Precedence)
}

func proxyConfigEqual(a, b *zitiv1alpha1.ProxyConfig) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return stringPtrEqual(a.Address, b.Address) && stringPtrEqual(a.Type, b.Type)
}

func httpChecksEqual(a, b []zitiv1alpha1.HTTPCheck) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].URL != b[i].URL || a[i].Method != b[i].Method || a[i].Interval != b[i].Interval || a[i].Timeout != b[i].Timeout {
			return false
		}
		if !stringPtrEqual(a[i].Body, b[i].Body) ||
			!int32PtrEqual(a[i].ExpectStatus, b[i].ExpectStatus) ||
			!stringPtrEqual(a[i].ExpectInBody, b[i].ExpectInBody) ||
			!checkActionsEqual(a[i].Actions, b[i].Actions) {
			return false
		}
	}
	return true
}

func portChecksEqual(a, b []zitiv1alpha1.PortCheck) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Address != b[i].Address || a[i].Interval != b[i].Interval || a[i].Timeout != b[i].Timeout {
			return false
		}
		if !checkActionsEqual(a[i].Actions, b[i].Actions) {
			return false
		}
	}
	return true
}

func checkActionsEqual(a, b []zitiv1alpha1.CheckAction) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Trigger != b[i].Trigger || a[i].Duration != b[i].Duration || a[i].Action != b[i].Action {
			return false
		}
		if !int32PtrEqual(a[i].ConsecutiveEvents, b[i].ConsecutiveEvents) {
			return false
		}
	}
	return true
}

func boolPtrEqual(a, b *bool) bool {
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

func int32PtrEqual(a, b *int32) bool {
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
	Tags         map[string]string      `json:"tags,omitempty"`
}
type configUpdatePayload struct {
	Name string                 `json:"name,omitempty"`
	Data map[string]interface{} `json:"data,omitempty"`
	Tags map[string]string      `json:"tags,omitempty"`
}
