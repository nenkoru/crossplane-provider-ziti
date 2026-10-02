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
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/feature"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/statemetrics"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	kube "sigs.k8s.io/controller-runtime/pkg/client"
)

// SetupGated adds a controller that reconciles the supplied kind once its
// custom resource definition is established.
func SetupGated[T resource.ModernManaged](mgr ctrl.Manager, o controller.Options, clients Connecter, kind Kind[T]) error {
	o.Gate.Register(func() {
		if err := Setup(mgr, o, clients, kind); err != nil {
			panic(errors.Wrapf(err, "cannot setup %s controller", kind.GVK.Kind))
		}
	}, kind.GVK)
	return nil
}

// Setup adds a controller that reconciles the supplied kind.
func Setup[T resource.ModernManaged](mgr ctrl.Manager, o controller.Options, clients Connecter, kind Kind[T]) error {
	name := managed.ControllerName(schema.GroupKind{Group: kind.GVK.Group, Kind: kind.GVK.Kind}.String())

	opts := []managed.ReconcilerOption{
		managed.WithTypedExternalConnector[T](&connector[T]{kind: kind, clients: clients}),
		// Ziti assigns the ID of an entity when it is created, so the external
		// name must stay empty until then instead of defaulting to the name of
		// the managed resource. An ID that could not be saved is recovered.
		managed.WithInitializers(NewCreationRecoverer(mgr.GetClient(), clients, kind.Collection)),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithPollInterval(o.PollInterval),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))), //nolint:staticcheck // TODO(jbw976) Crossplane needs to update to the new events API, see https://github.com/crossplane/crossplane/issues/7152
	}

	if o.Features.Enabled(feature.EnableBetaManagementPolicies) {
		opts = append(opts, managed.WithManagementPolicies())
	}

	if o.Features.Enabled(feature.EnableAlphaChangeLogs) {
		opts = append(opts, managed.WithChangeLogger(o.ChangeLogOptions.ChangeLogger))
	}

	if o.MetricOptions != nil {
		opts = append(opts, managed.WithMetricRecorder(o.MetricOptions.MRMetrics))
	}

	if o.MetricOptions != nil && o.MetricOptions.MRStateMetrics != nil {
		stateMetricsRecorder := statemetrics.NewMRStateRecorder(
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, kind.List, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrapf(err, "cannot register MR state metrics recorder for kind %s", kind.GVK.Kind)
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(kind.GVK), opts...)

	//nolint:forcetypeassert // A kind that is not a Kubernetes object is a programming error.
	object := resource.MustCreateObject(kind.GVK, mgr.GetScheme()).(kube.Object)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(object).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}
