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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/prometheus/client_golang/prometheus"
	crdv1alpha1 "github.com/uselagoon/dbaas-controller/api/v1alpha1"
	"github.com/uselagoon/dbaas-controller/internal/database"
)

const databaseProviderFinalizer = "relationaldatabaseprovider.crd.lagoon.sh/finalizer"

var (
	// Prometheus metrics
	// promRelationalDatabaseProviderReconcileErrorCounter counter for the reconciled relational database providers errors
	promRelationalDatabaseProviderReconcileErrorCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "relationaldatabaseprovider_reconcile_error_total",
			Help: "The total number of reconciled relational database providers errors",
		},
		[]string{"type", "name", "selector", "error"},
	)

	// promRelationalDatabaseProviderStatus is the gauge for the relational database provider status
	promRelationalDatabaseProviderStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "relationaldatabaseprovider_status",
			Help: "The status of the relational database provider",
		},
		[]string{"type", "name", "selector"},
	)

	// promRelationalDatabaseProviderConnectionVersion is the gauge for the relational database provider connection version
	promRelationalDatabaseProviderConnectionVersion = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "relationaldatabaseprovider_connection_version",
			Help: "The version of the relational database provider connection",
		},
		[]string{"type", "name", "selector", "hostname", "username", "version"},
	)
)

// RelationalDatabaseProviderReconciler reconciles a RelationalDatabaseProvider object
type RelationalDatabaseProviderReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	Recorder    events.EventRecorder
	RelDBClient database.RelationalDatabaseInterface
}

//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// nolint:lll
//+kubebuilder:rbac:groups=crd.lagoon.sh,resources=relationaldatabaseproviders,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=crd.lagoon.sh,resources=relationaldatabaseproviders/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=crd.lagoon.sh,resources=relationaldatabaseproviders/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *RelationalDatabaseProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName("relationaldatabaseprovider_controller")
	logger.Info("Reconciling RelationalDatabaseProvider")

	// Fetch the RelationalDatabaseProvider instance
	instance := &crdv1alpha1.RelationalDatabaseProvider{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.Info("RelationalDatabaseProvider not found")
			return ctrl.Result{}, nil
		}
		promRelationalDatabaseProviderReconcileErrorCounter.WithLabelValues(
			"", req.Name, "", "get-relationaldbprovider").Inc()
		return ctrl.Result{}, err
	}
	logger = logger.WithValues("type", instance.Spec.Type, "selector", instance.Spec.Selector)
	if instance.DeletionTimestamp != nil && !instance.DeletionTimestamp.IsZero() {
		// The object is being deleted
		// To be discussed whether we need to delete all the database requests using this provider...
		if controllerutil.RemoveFinalizer(instance, databaseProviderFinalizer) {
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
			logger.Info("No updates to reconcile")
			r.Recorder.Eventf(instance, instance, v1.EventTypeNormal, "ReconcileSkipped", "No updates to reconcile", "No updates to reconcile")
			return ctrl.Result{}, nil
		}
	}

	if controllerutil.AddFinalizer(instance, databaseProviderFinalizer) {
		if err := r.Update(ctx, instance); err != nil {
			return r.handleError(ctx, instance, "add-finalizer", err)
		}
	}

	// Reconcile the RelationalDatabaseProvider and check the unique name of the Connections
	uniqueNames := make(map[string]struct{}, len(instance.Spec.Connections))
	conns := make([]reldbConn, 0, len(instance.Spec.Connections))
	for _, conn := range instance.Spec.Connections {
		secret := &v1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{
			Name:      conn.PasswordSecretRef.Name,
			Namespace: conn.PasswordSecretRef.Namespace,
		}, secret); err != nil {
			return r.handleError(ctx, instance, "get-secret", err)
		}
		password := string(secret.Data["password"])
		if password == "" {
			return r.handleError(
				ctx,
				instance,
				fmt.Sprintf("%s-empty-password", instance.Spec.Type),
				fmt.Errorf(
					"%s connection secret %s in namespace %s has empty password",
					instance.Spec.Type, secret.Name, secret.Namespace,
				),
			)
		}
		conns = append(conns, reldbConn{
			dbType:           instance.Spec.Type,
			name:             conn.Name,
			hostname:         conn.Hostname,
			replicaHostnames: conn.ReplicaHostnames,
			password:         password,
			port:             conn.Port,
			username:         conn.Username,
			enabled:          conn.Enabled,
		})
		uniqueNames[conn.Name] = struct{}{}
	}

	if len(uniqueNames) != len(instance.Spec.Connections) {
		return r.handleError(
			ctx,
			instance,
			fmt.Sprintf("%s-unique-name-error", instance.Spec.Type),
			fmt.Errorf("%s database connections must have unique names", instance.Spec.Type),
		)
	}

	dbStatus := make([]crdv1alpha1.ConnectionStatus, 0, len(conns))
	errors := make([]error, 0, len(conns))
	foundEnabledDatabase := false
	for _, conn := range conns {
		// make a ping to the database to check if it's up and running and we can connect to it
		// if not, we should return an error and set the status to 0
		// Note we could periodically check the status of the database and update the status accordingly...
		if err := r.RelDBClient.Ping(ctx, conn.getDSN(false), instance.Spec.Type); err != nil {
			errors = append(errors, err)
			dbStatus = append(dbStatus, crdv1alpha1.ConnectionStatus{
				Name:     conn.name,
				Hostname: conn.hostname,
				Status:   "unavailable",
				Enabled:  conn.enabled,
			})
			promRelationalDatabaseProviderConnectionVersion.WithLabelValues(
				instance.Spec.Type, req.Name, instance.Spec.Selector, conn.hostname, conn.username, "").Set(0)
			logger.Error(err, "Failed to ping the database", "hostname", conn.hostname)
			continue
		}
		version, err := r.RelDBClient.Version(ctx, conn.getDSN(false), instance.Spec.Type)
		if err != nil {
			errors = append(errors, err)
			dbStatus = append(dbStatus, crdv1alpha1.ConnectionStatus{
				Name:     conn.name,
				Hostname: conn.hostname,
				Status:   "unavailable",
				Enabled:  conn.enabled,
			})
			logger.Error(err, "Failed to get the database version", "hostname", conn.hostname)
			promRelationalDatabaseProviderConnectionVersion.WithLabelValues(
				instance.Spec.Type, req.Name, instance.Spec.Selector, conn.hostname, conn.username, version).Set(0)
			continue
		}

		// check if the database is initialized
		err = r.RelDBClient.Initialize(ctx, conn.getDSN(false), instance.Spec.Type)
		if err != nil {
			errors = append(errors, err)
			dbStatus = append(dbStatus, crdv1alpha1.ConnectionStatus{
				Name:     conn.name,
				Hostname: conn.hostname,
				Status:   "unavailable",
				Enabled:  conn.enabled,
			})
			promRelationalDatabaseProviderConnectionVersion.WithLabelValues(
				instance.Spec.Type, req.Name, instance.Spec.Selector, conn.hostname, conn.username, version).Set(0)
			continue
		}

		dbStatus = append(dbStatus, crdv1alpha1.ConnectionStatus{
			Name:            conn.name,
			Hostname:        conn.hostname,
			DatabaseVersion: version,
			Status:          "available",
			Enabled:         conn.enabled,
		})
		if conn.enabled {
			foundEnabledDatabase = true
			promRelationalDatabaseProviderConnectionVersion.WithLabelValues(
				instance.Spec.Type, req.Name, instance.Spec.Selector, conn.hostname, conn.username, version).Set(1)
		} else {
			promRelationalDatabaseProviderConnectionVersion.WithLabelValues(
				instance.Spec.Type, req.Name, instance.Spec.Selector, conn.hostname, conn.username, version).Set(0)
		}
	}

	instance.Status.ConnectionStatus = dbStatus
	instance.Status.ObservedGeneration = instance.Generation

	if len(errors) == len(conns) {
		return r.handleError(
			ctx,
			instance,
			fmt.Sprintf("%s-connection-error", instance.Spec.Type),
			fmt.Errorf("failed to connect to any of the %s databases: %v", instance.Spec.Type, errors),
		)
	}
	if !foundEnabledDatabase {
		return r.handleError(
			ctx,
			instance,
			fmt.Sprintf("%s-connection-not-any-enabled", instance.Spec.Type),
			fmt.Errorf("no enabled working %s database found", instance.Spec.Type),
		)
	}

	// update the status condition to ready
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "Reconciled",
		Message: "RelationalDatabaseProvider reconciled",
	})
	// update the status
	if err := r.Status().Update(ctx, instance); err != nil {
		promRelationalDatabaseProviderReconcileErrorCounter.WithLabelValues(
			instance.Spec.Type, req.Name, instance.Spec.Selector, "update-status").Inc()
		promRelationalDatabaseProviderStatus.WithLabelValues(instance.Spec.Type, req.Name, instance.Spec.Selector).Set(0)
		return ctrl.Result{}, err
	}

	r.Recorder.Eventf(instance, instance, "Normal", "Reconciled", "RelationalDatabaseProvider reconciled", "RelationalDatabaseProvider reconciled")
	promRelationalDatabaseProviderStatus.WithLabelValues(instance.Spec.Type, req.Name, instance.Spec.Selector).Set(1)
	return ctrl.Result{}, nil
}

// handleError handles the error and returns the result and the error
func (r *RelationalDatabaseProviderReconciler) handleError(
	ctx context.Context,
	instance *crdv1alpha1.RelationalDatabaseProvider,
	promErr string,
	err error,
) (ctrl.Result, error) {
	promRelationalDatabaseProviderReconcileErrorCounter.WithLabelValues(
		instance.Spec.Type, instance.Name, instance.Spec.Selector, promErr).Inc()
	promRelationalDatabaseProviderStatus.WithLabelValues(instance.Spec.Type, instance.Name, instance.Spec.Selector).Set(0)
	r.Recorder.Eventf(instance, instance, v1.EventTypeWarning, errTypeToEventReason(promErr), err.Error(), err.Error())

	// set the status condition to false
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  errTypeToEventReason(promErr),
		Message: err.Error(),
	})

	// update the status
	if err := r.Status().Update(ctx, instance); err != nil {
		promRelationalDatabaseProviderReconcileErrorCounter.WithLabelValues(
			instance.Spec.Type, instance.Name, instance.Spec.Selector, "update-status").Inc()
		log.FromContext(ctx).Error(err, "Failed to update status")
	}

	return ctrl.Result{}, err
}

// reldbConn is the connection to a MySQL or PostgreSQL database
type reldbConn struct {
	dbType           string
	name             string
	hostname         string
	replicaHostnames []string
	password         string
	port             int
	username         string
	enabled          bool
}

// getDSN constructs the DSN string for the MySQL or PostgreSQL connection.
func (rc *reldbConn) getDSN(useDatabase bool) string {
	dsn := ""
	switch rc.dbType {
	case "mysql":
		dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/", rc.username, rc.password, rc.hostname, rc.port)
		if useDatabase {
			dsn += rc.name
		}
	case "postgres":
		dsn = fmt.Sprintf(
			"host=%s port=%d user=%s password=%s sslmode=disable",
			rc.hostname, rc.port, rc.username, rc.password,
		)
		if useDatabase {
			dsn += fmt.Sprintf(" dbname=%s", rc.name)
		}
	}
	return dsn
}

// SetupWithManager sets up the controller with the Manager.
func (r *RelationalDatabaseProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// register metrics
	metrics.Registry.MustRegister(
		promRelationalDatabaseProviderReconcileErrorCounter,
		promRelationalDatabaseProviderStatus,
		promRelationalDatabaseProviderConnectionVersion,
	)
	r.Recorder = mgr.GetEventRecorder("relationaldatabaseprovider_controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&crdv1alpha1.RelationalDatabaseProvider{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		// let's set the max concurrent reconciles to 1 as we don't want to run multiple reconciles at the same time
		// although we could also change this and guard it by the name of the database provider
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}
