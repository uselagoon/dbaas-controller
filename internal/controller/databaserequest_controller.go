/*
Copyright 2024.

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

package controller

import (
	"context"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/prometheus/client_golang/prometheus"
	crdv1alpha1 "github.com/uselagoon/dbaas-controller/api/v1alpha1"
)

const databaseRequestFinalizer = "databaserequest.crd.lagoon.sh/finalizer"

var (
	// Prometheus metrics
	// promDatabaseRequestReconcileCounter is the counter for the reconciled database requests
	promDatabaseRequestReconcileCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "databaserequest_reconcile_total",
			Help: "The total number of reconciled database requests",
		},
		[]string{"name", "namespace"},
	)

	// promDatabaseRequestReconcileErrorCounter is the counter for the reconciled database requests errors
	promDatabaseRequestReconcileErrorCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "databaserequest_reconcile_error_total",
			Help: "The total number of reconciled database requests errors",
		},
		[]string{"name", "namespace", "scope", "type", "error"},
	)

	// promDatabaseRequestReconcileStatus is the status of the reconciled database requests
	promDatabaseRequestReconcileStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "databaserequest_reconcile_status",
			Help: "The status of the reconciled database requests",
		},
		[]string{"name", "namespace", "scope", "type"},
	)
)

// DatabaseRequestReconciler reconciles a DatabaseRequest object
type DatabaseRequestReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=crd.lagoon.sh,resources=databaserequests,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=crd.lagoon.sh,resources=databaserequests/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=crd.lagoon.sh,resources=databaserequests/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile is the main logic of the controller
func (r *DatabaseRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName("databaserequest_controller")
	logger.Info("Reconciling DatabaseRequest")
	promDatabaseRequestReconcileCounter.WithLabelValues(req.Name, req.Namespace).Inc()

	databaseRequest := &crdv1alpha1.DatabaseRequest{}
	if err := r.Get(ctx, req.NamespacedName, databaseRequest); err != nil {
		promDatabaseRequestReconcileErrorCounter.WithLabelValues(req.Name, req.Namespace, "", "", "get-dbreq").Inc()
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if databaseRequest.DeletionTimestamp != nil && !databaseRequest.DeletionTimestamp.IsZero() {
		// handle deletion logic
		if databaseRequest.Spec.DropDatabaseOnDelete {

		}
		if controllerutil.RemoveFinalizer(databaseRequest, databaseRequestFinalizer) {
			if err := r.Update(ctx, databaseRequest); err != nil {
				promDatabaseRequestReconcileErrorCounter.With(
					promLabels(databaseRequest, "update-dbreq")).Inc()
				promDatabaseRequestReconcileStatus.With(promLabels(databaseRequest, "")).Set(0)
				return ctrl.Result{}, err
			}
		}
		promDatabaseRequestReconcileStatus.DeletePartialMatch(prometheus.Labels{
			"name":      req.Name,
			"namespace": req.Namespace,
		})
		return ctrl.Result{}, nil
	}

	if controllerutil.AddFinalizer(databaseRequest, databaseRequestFinalizer) {
		if err := r.Update(ctx, databaseRequest); err != nil {
			promDatabaseRequestReconcileErrorCounter.With(
				promLabels(databaseRequest, "add-finalizer")).Inc()
			promDatabaseRequestReconcileStatus.With(promLabels(databaseRequest, "")).Set(0)
			return ctrl.Result{}, err
		}
	}

	if databaseRequest.Status.ObservedGeneration >= databaseRequest.Generation {
		logger.Info("No updates to reconcile")
		r.Recorder.Event(databaseRequest, v1.EventTypeNormal, "ReconcileSkipped", "No updates to reconcile")
		return ctrl.Result{}, nil
	}

	changed := false

	// check if the database request is already created and the secret and service exist
	secret := &v1.Secret{}
	if err := r.Get(ctx, req.NamespacedName, secret); err != nil {
		if apierrors.IsNotFound(err) {
			// create the secret
		} else {
			promDatabaseRequestReconcileErrorCounter.With(
				promLabels(databaseRequest, "get-secret")).Inc()
			promDatabaseRequestReconcileStatus.With(promLabels(databaseRequest, "")).Set(0)

			return ctrl.Result{}, err
		}
	}

	service := &v1.Service{}
	if err := r.Get(ctx, req.NamespacedName, service); err != nil {
		if apierrors.IsNotFound(err) {
			// create the service
		} else {
			promDatabaseRequestReconcileErrorCounter.With(
				promLabels(databaseRequest, "get-service")).Inc()
			promDatabaseRequestReconcileStatus.With(promLabels(databaseRequest, "")).Set(0)
			return ctrl.Result{}, err
		}
	}

	if changed {
		meta.SetStatusCondition(&databaseRequest.Status.Conditions, metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionTrue,
			Reason:  "DatabaseRequestCreated",
			Message: "The database request has been created",
		})
		r.Recorder.Event(databaseRequest, "Normal", "DatabaseRequestCreated", "The database request has been created")
	}
	promDatabaseRequestReconcileStatus.With(promLabels(databaseRequest, "")).Set(1)
	databaseRequest.Status.ObservedGeneration = databaseRequest.Generation
	// update the status
	if err := r.Status().Update(ctx, databaseRequest); err != nil {
		promDatabaseRequestReconcileErrorCounter.With(
			promLabels(databaseRequest, "update-status")).Inc()
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// promLabels returns the prometheus labels for the database request
func promLabels(databaseRequest *crdv1alpha1.DatabaseRequest, withError string) prometheus.Labels {
	labels := prometheus.Labels{
		"name":      databaseRequest.Name,
		"namespace": databaseRequest.Namespace,
		"scope":     databaseRequest.Spec.Scope,
		"type":      databaseRequest.Spec.Type,
	}
	if withError != "" {
		labels["error"] = withError
	}
	return labels
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	metrics.Registry.MustRegister(
		promDatabaseRequestReconcileCounter,
		promDatabaseRequestReconcileErrorCounter,
		promDatabaseRequestReconcileStatus,
	)
	r.Recorder = mgr.GetEventRecorderFor("DatabaseRequestReconciler")
	return ctrl.NewControllerManagedBy(mgr).
		For(&crdv1alpha1.DatabaseRequest{}).
		// do only reconcile on spec changes
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}
