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
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/prometheus/client_golang/prometheus"
	crdv1alpha1 "github.com/uselagoon/dbaas-controller/api/v1alpha1"
	"github.com/uselagoon/dbaas-controller/internal/database/mongodb"
)

const mongoDBProviderFinalizer = "mongodbprovider.crd.lagoon.sh/finalizer"

var (
	// Prometheus metrics
	// promMongoDBProviderReconcileErrorCounter counter for the reconciled mongodb providers errors
	promMongoDBProviderReconcileErrorCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mongodbprovider_reconcile_error_total",
			Help: "The total number of reconciled mongodb providers errors",
		},
		[]string{"name", "selector", "error"},
	)

	// promMongoDBProviderStatus is the gauge for the mongodb provider status
	promMongoDBProviderStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mongodbprovider_status",
			Help: "The status of the mongodb provider",
		},
		[]string{"name", "selector"},
	)

	// promMongoDBProviderConnectionVersion is the gauge for the mongodb provider connection version
	promMongoDBProviderConnectionVersion = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mongodbprovider_connection_version",
			Help: "The version of the mongodb provider connection",
		},
		[]string{"name", "selector", "hostname", "username", "version"},
	)
)

// MongoDBProviderReconciler reconciles a MongoDBProvider object
type MongoDBProviderReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Recorder      record.EventRecorder
	MongoDBClient mongodb.MongoDBInterface
}

// nolint:lll
//+kubebuilder:rbac:groups=crd.lagoon.sh,resources=mongodbproviders,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=crd.lagoon.sh,resources=mongodbproviders/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=crd.lagoon.sh,resources=mongodbproviders/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the MongoDBProvider object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.3/pkg/reconcile
func (r *MongoDBProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName("mongodbprovider_controller")
	logger.Info("Reconciling MongoDBProvider")

	instance := &crdv1alpha1.MongoDBProvider{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.Info("MongoDBProvider resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		promMongoDBProviderReconcileErrorCounter.WithLabelValues(req.Name, "", "get-mongodbprovider").Inc()
		return ctrl.Result{}, err
	}
	logger = logger.WithValues("selector", instance.Spec.Selector)
	log.IntoContext(ctx, logger)
	if instance.DeletionTimestamp != nil && !instance.DeletionTimestamp.IsZero() {
		// The object is being deleted
		// To be discussed whether we should delete the database requests using this provider...
		if controllerutil.RemoveFinalizer(instance, mongoDBProviderFinalizer) {
			if err := r.Update(ctx, instance); err != nil {
				return r.handleError(ctx, instance, "remove-finalizer", err)
			}
		}
		return ctrl.Result{}, nil
	}

	// Check if we need to reconcile based on Generation and ObservedGeneration but only if
	// the status condition is not false. This makes sure that in case of an error the controller
	// will try to reconcile again.
	if instance.Status.Conditions != nil && meta.IsStatusConditionTrue(instance.Status.Conditions, "Ready") {
		if instance.Status.ObservedGeneration >= instance.Generation {
			logger.Info("Skipping reconcile: generation has not changed")
			r.Recorder.Event(instance, v1.EventTypeNormal, "ReconcileSkipped", "No updates to reconcile")
			return ctrl.Result{}, nil
		}
	}

	// Add finalizer if not already added
	if !controllerutil.ContainsFinalizer(instance, mongoDBProviderFinalizer) {
		controllerutil.AddFinalizer(instance, mongoDBProviderFinalizer)
		if err := r.Update(ctx, instance); err != nil {
			return r.handleError(ctx, instance, "add-finalizer", err)
		}
	}

	// Check the unique names of connections
	uniqueNames := make(map[string]struct{}, len(instance.Spec.Connections))
	conns := make([]mongoDBConn, 0, len(instance.Spec.Connections))
	for _, conn := range instance.Spec.Connections {
		uniqueNames[conn.Name] = struct{}{}
		secret := &v1.Secret{}
		if err := r.Get(ctx, client.ObjectKey{
			Name: conn.PasswordSecretRef.Name, Namespace: conn.PasswordSecretRef.Namespace,
		}, secret); err != nil {
			return r.handleError(ctx, instance, "get-secret", err)
		}
		password := string(secret.Data["password"])
		if password == "" {
			return r.handleError(ctx, instance, "empty-password", fmt.Errorf(
				"password is empty for secret %s/%s", conn.PasswordSecretRef.Namespace, conn.PasswordSecretRef.Name,
			))
		}
		conns = append(conns, mongoDBConn{
			options: mongodb.MongoDBClientOptions{
				Name:      conn.Name,
				Hostname:  conn.Hostname,
				Port:      conn.Port,
				Username:  conn.Username,
				Password:  password,
				Mechanism: conn.Auth.Mechanism,
				Source:    conn.Auth.Source,
				TLS:       conn.Auth.TLS,
			},
			enabled: conn.Enabled,
		})
	}
	if len(uniqueNames) != len(instance.Spec.Connections) {
		return r.handleError(ctx, instance, "dunique-name-error", fmt.Errorf(
			"connection names are not unique for MongoDBProvider %s", instance.Name,
		))
	}

	dbStatus := make([]crdv1alpha1.ConnectionStatus, 0, len(conns))
	errors := make([]error, 0, len(conns))
	foundEnabledDatabase := false
	for _, conn := range conns {
		// make a ping to the database to check if it's up and running and we can connect to it
		// if not, we should return an error and set the status to 0
		// Note we could periodically check the status of the database and update the status accordingly...
		if err := r.MongoDBClient.Ping(ctx, conn.options); err != nil {
			errors = append(errors, err)
			dbStatus = append(dbStatus, crdv1alpha1.ConnectionStatus{
				Name:            conn.options.Name,
				Hostname:        conn.options.Hostname,
				DatabaseVersion: "unknown",
				Enabled:         conn.enabled,
				Status:          "unavailable",
			})
			promMongoDBProviderConnectionVersion.WithLabelValues(
				conn.options.Name, instance.Spec.Selector, conn.options.Hostname, conn.options.Username, "",
			).Set(0)
			logger.Error(err, "Failed to ping MongoDB", "hostname", conn.options.Hostname)
			continue
		}
		version, err := r.MongoDBClient.Version(ctx, conn.options)
		if err != nil {
			errors = append(errors, err)
			dbStatus = append(dbStatus, crdv1alpha1.ConnectionStatus{
				Name:            conn.options.Name,
				Hostname:        conn.options.Hostname,
				DatabaseVersion: "unknown",
				Enabled:         conn.enabled,
				Status:          "unavailable",
			})
			promMongoDBProviderConnectionVersion.WithLabelValues(
				conn.options.Name, instance.Spec.Selector, conn.options.Hostname, conn.options.Username, version,
			).Set(0)
			logger.Error(err, "Failed to get MongoDB version", "hostname", conn.options.Hostname)
			continue
		}

		dbStatus = append(dbStatus, crdv1alpha1.ConnectionStatus{
			Name:            conn.options.Name,
			Hostname:        conn.options.Hostname,
			DatabaseVersion: version,
			Status:          "available",
			Enabled:         conn.enabled,
		})
		if conn.enabled {
			foundEnabledDatabase = true
			promMongoDBProviderConnectionVersion.WithLabelValues(
				conn.options.Name, instance.Spec.Selector, conn.options.Hostname, conn.options.Username, version,
			).Set(1)
		} else {
			promMongoDBProviderConnectionVersion.WithLabelValues(
				conn.options.Name, instance.Spec.Selector, conn.options.Hostname, conn.options.Username, version,
			).Set(0)
		}
	}

	instance.Status.ConnectionStatus = dbStatus
	instance.Status.ObservedGeneration = instance.Generation

	if len(errors) == len(conns) {
		return r.handleError(ctx, instance, "all-connections-error", fmt.Errorf(
			"all connections failed for MongoDBProvider %s", instance.Name,
		))
	}

	if !foundEnabledDatabase {
		return r.handleError(ctx, instance, "no-enabled-database", fmt.Errorf(
			"no enabled database found for MongoDBProvider %s", instance.Name,
		))
	}

	// update the status condition to ready
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "Reconciled",
		Message: "MongoDBProvider reconciled",
	})
	// update the status
	if err := r.Status().Update(ctx, instance); err != nil {
		promMongoDBProviderReconcileErrorCounter.WithLabelValues(
			req.Name, instance.Spec.Selector, "update-status").Inc()
		promMongoDBProviderStatus.WithLabelValues(req.Name, instance.Spec.Selector).Set(0)
		return ctrl.Result{}, err
	}

	r.Recorder.Event(instance, v1.EventTypeNormal, "Reconciled", "MongoDBProvider reconciled")
	promMongoDBProviderStatus.WithLabelValues(req.Name, instance.Spec.Selector).Set(1)
	return ctrl.Result{}, nil
}

func (r *MongoDBProviderReconciler) handleError(
	ctx context.Context,
	instance *crdv1alpha1.MongoDBProvider,
	promErr string,
	err error,
) (ctrl.Result, error) {
	promMongoDBProviderReconcileErrorCounter.WithLabelValues(instance.Name, instance.Spec.Selector, promErr).Inc()
	promMongoDBProviderStatus.WithLabelValues(instance.Name, instance.Spec.Selector).Set(0)
	r.Recorder.Event(instance, v1.EventTypeWarning, errTypeToEventReason(promErr), err.Error())

	// set the status condition
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  errTypeToEventReason(promErr),
		Message: err.Error(),
	})

	// update the status
	if err := r.Status().Update(ctx, instance); err != nil {
		promMongoDBProviderReconcileErrorCounter.WithLabelValues(instance.Name, instance.Spec.Selector, "update-status").Inc()
		log.FromContext(ctx).Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, err
}

// mongoDBConn is a struct to hold the connection details to a MongoDB database
type mongoDBConn struct {
	options mongodb.MongoDBClientOptions
	enabled bool
}

// SetupWithManager sets up the controller with the Manager.
func (r *MongoDBProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// register metrics
	metrics.Registry.MustRegister(
		promMongoDBProviderReconcileErrorCounter,
		promMongoDBProviderStatus,
		promMongoDBProviderConnectionVersion,
	)
	r.Recorder = mgr.GetEventRecorderFor("mongodbprovider-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&crdv1alpha1.MongoDBProvider{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		// Only allow one reconcile at a time
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
		}).
		Complete(r)
}
