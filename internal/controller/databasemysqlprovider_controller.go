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
	"github.com/uselagoon/dbaas-controller/internal/database/mysql"
)

const databaseMySQLProviderFinalizer = "databasemysqlprovider.crd.lagoon.sh/finalizer"

var (
	// Prometheus metrics
	// promDatabaseMySQLProviderReconcileCounter is the counter for the reconciled database mysql providers
	promDatabaseMySQLProviderReconcileCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "databasemysqlprovider_reconcile_total",
			Help: "The total number of reconciled database mysql providers",
		},
		[]string{"name"},
	)

	//promDatabaseMySQLProviderReconcileErrorCounter is the counter for the reconciled database mysql providers errors
	promDatabaseMySQLProviderReconcileErrorCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "databasemysqlprovider_reconcile_error_total",
			Help: "The total number of reconciled database mysql providers errors",
		},
		[]string{"name", "scope", "error"},
	)

	// promDatabaseMySQLProviderStatus is the gauge for the database mysql provider status
	promDatabaseMySQLProviderStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "databasemysqlprovider_status",
			Help: "The status of the database mysql provider",
		},
		[]string{"name", "scope"},
	)

	// promDatabaseMySQLProviderConnectionVersion is the gauge for the database mysql provider connection version
	promDatabaseMySQLProviderConnectionVersion = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "databasemysqlprovider_connection_version",
			Help: "The version of the database mysql provider connection",
		},
		[]string{"name", "scope", "hostname", "username", "version"},
	)
)

// DatabaseMySQLProviderReconciler reconciles a DatabaseMySQLProvider object
type DatabaseMySQLProviderReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	Recorder    record.EventRecorder
	MySQLClient mysql.MySQLInterface
}

//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=crd.lagoon.sh,resources=databasemysqlproviders,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=crd.lagoon.sh,resources=databasemysqlproviders/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=crd.lagoon.sh,resources=databasemysqlproviders/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DatabaseMySQLProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName("databasemysqlprovider_controller")
	logger.Info("Reconciling DatabaseMySQLProvider")
	promDatabaseMySQLProviderReconcileCounter.WithLabelValues(req.Name).Inc()

	// Fetch the DatabaseMySQLProvider instance
	instance := &crdv1alpha1.DatabaseMySQLProvider{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.Info("DatabaseMySQLProvider not found")
			return ctrl.Result{}, nil
		}
		promDatabaseMySQLProviderReconcileErrorCounter.WithLabelValues(req.Name, "", "get-dbmysqlprovider").Inc()
		return ctrl.Result{}, err
	}

	if instance.DeletionTimestamp != nil && !instance.DeletionTimestamp.IsZero() {
		// The object is being deleted
		// To be discussed whether we need to delete all the database requests using this provider...
		if controllerutil.RemoveFinalizer(instance, databaseMySQLProviderFinalizer) {
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
			r.Recorder.Event(instance, v1.EventTypeNormal, "ReconcileSkipped", "No updates to reconcile")
			return ctrl.Result{}, nil
		}
	}

	if controllerutil.AddFinalizer(instance, databaseMySQLProviderFinalizer) {
		if err := r.Update(ctx, instance); err != nil {
			return r.handleError(ctx, instance, "add-finalizer", err)
		}
	}

	// Reconcile the DatabaseMySQLProvider and check the unique name of the MySQLConnections
	uniqueNames := make(map[string]struct{}, len(instance.Spec.MySQLConnections))
	mySQLConns := make([]mySQLConn, 0, len(instance.Spec.MySQLConnections))
	for _, conn := range instance.Spec.MySQLConnections {
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
				"empty-password",
				fmt.Errorf("mysql connection secret %s in namespace %s has empty password", secret.Name, secret.Namespace),
			)
		}
		mySQLConns = append(mySQLConns, mySQLConn{
			name:             conn.Name,
			hostname:         conn.Hostname,
			replicaHostnames: conn.ReplicaHostnames,
			password:         password,
			port:             conn.Port,
			username:         conn.Username,
			enabled:          conn.Enabled,
		})
		uniqueNames[conn.Hostname] = struct{}{}
	}

	if len(uniqueNames) != len(instance.Spec.MySQLConnections) {
		return r.handleError(
			ctx,
			instance,
			"unique-name",
			fmt.Errorf("mysql connections must have unique names"),
		)
	}

	mySQLStatus := make([]crdv1alpha1.MySQLConnectionStatus, 0, len(mySQLConns))
	errors := make([]error, 0, len(mySQLConns))
	foundEnabledMySQL := false
	for _, conn := range mySQLConns {
		// make a ping to the database to check if it's up and running and we can connect to it
		// if not, we should return an error and set the status to 0
		// Note we could periodically check the status of the database and update the status accordingly...
		if err := r.MySQLClient.Ping(ctx, conn.getDSN()); err != nil {
			errors = append(errors, err)
			mySQLStatus = append(mySQLStatus, crdv1alpha1.MySQLConnectionStatus{
				Name:     conn.name,
				Hostname: conn.hostname,
				Status:   "unavailable",
				Enabled:  conn.enabled,
			})
			continue
		}
		version, err := r.MySQLClient.Version(ctx, conn.getDSN())
		if err != nil {
			errors = append(errors, err)
			mySQLStatus = append(mySQLStatus, crdv1alpha1.MySQLConnectionStatus{
				Name:     conn.name,
				Hostname: conn.hostname,
				Status:   "unavailable",
				Enabled:  conn.enabled,
			})
			continue
		}

		// check if the database is initialized
		err = r.MySQLClient.Initialize(ctx, conn.getDSN())
		if err != nil {
			errors = append(errors, err)
			mySQLStatus = append(mySQLStatus, crdv1alpha1.MySQLConnectionStatus{
				Name:     conn.name,
				Hostname: conn.hostname,
				Status:   "unavailable",
				Enabled:  conn.enabled,
			})
			continue
		}

		promDatabaseMySQLProviderConnectionVersion.WithLabelValues(
			req.Name, instance.Spec.Scope, conn.hostname, conn.username, version).Set(1)
		mySQLStatus = append(mySQLStatus, crdv1alpha1.MySQLConnectionStatus{
			Name:         conn.name,
			Hostname:     conn.hostname,
			MySQLVersion: version,
			Status:       "available",
			Enabled:      conn.enabled,
		})

		if conn.enabled {
			foundEnabledMySQL = true
		}
	}

	instance.Status.MySQLConnectionStatus = mySQLStatus
	instance.Status.ObservedGeneration = instance.Generation

	if len(errors) == len(mySQLConns) {
		return r.handleError(
			ctx,
			instance,
			"mysql-connection",
			fmt.Errorf("failed to connect to any of the MySQL databases: %v", errors),
		)
	}
	if !foundEnabledMySQL {
		return r.handleError(
			ctx,
			instance,
			"mysql-connection",
			fmt.Errorf("no enabled working MySQL database found"),
		)
	}

	// update the status condition to ready
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "Reconciled",
		Message: "DatabaseMySQLProvider reconciled",
	})
	// update the status
	if err := r.Status().Update(ctx, instance); err != nil {
		promDatabaseMySQLProviderReconcileErrorCounter.WithLabelValues(
			req.Name, instance.Spec.Scope, "update-status").Inc()
		promDatabaseMySQLProviderStatus.WithLabelValues(req.Name, instance.Spec.Scope).Set(0)
		return ctrl.Result{}, err
	}

	r.Recorder.Event(instance, "Normal", "Reconciled", "DatabaseMySQLProvider reconciled")
	promDatabaseMySQLProviderStatus.WithLabelValues(req.Name, instance.Spec.Scope).Set(1)
	return ctrl.Result{}, nil
}

// handleError handles the error and returns the result and the error
func (r *DatabaseMySQLProviderReconciler) handleError(
	ctx context.Context,
	instance *crdv1alpha1.DatabaseMySQLProvider,
	promErr string,
	err error,
) (ctrl.Result, error) {
	promDatabaseMySQLProviderReconcileErrorCounter.WithLabelValues(
		instance.Name, instance.Spec.Scope, promErr).Inc()
	promDatabaseMySQLProviderStatus.WithLabelValues(instance.Name, instance.Spec.Scope).Set(0)
	r.Recorder.Event(instance, v1.EventTypeWarning, errTypeToEventReason(promErr), err.Error())

	// set the status condition to false
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  errTypeToEventReason(promErr),
		Message: err.Error(),
	})

	// update the status
	if err := r.Status().Update(ctx, instance); err != nil {
		promDatabaseMySQLProviderReconcileErrorCounter.WithLabelValues(
			instance.Name, instance.Spec.Scope, "update-status").Inc()
		log.FromContext(ctx).Error(err, "Failed to update status")
	}

	return ctrl.Result{}, err
}

// mysqlConn is the connection to a MySQL database
type mySQLConn struct {
	name             string
	hostname         string
	replicaHostnames []string
	password         string
	port             int
	username         string
	enabled          bool
}

// getDSN constructs the DSN string for the MySQL connection.
func (mc *mySQLConn) getDSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/", mc.username, mc.password, mc.hostname, mc.port)
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseMySQLProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// register metrics
	metrics.Registry.MustRegister(
		promDatabaseMySQLProviderReconcileCounter,
		promDatabaseMySQLProviderReconcileErrorCounter,
		promDatabaseMySQLProviderStatus,
		promDatabaseMySQLProviderConnectionVersion,
	)
	r.Recorder = mgr.GetEventRecorderFor("databasemysqlprovider_controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&crdv1alpha1.DatabaseMySQLProvider{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		// let's set the max concurrent reconciles to 1 as we don't want to run multiple reconciles at the same time
		// although we could also change this and guard it by the name of the database provider
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}
