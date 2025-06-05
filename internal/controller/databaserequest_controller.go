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
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/client_golang/prometheus"
	crdv1alpha1 "github.com/uselagoon/dbaas-controller/api/v1alpha1"
	"github.com/uselagoon/dbaas-controller/internal/database"
)

const databaseRequestFinalizer = "databaserequest.crd.lagoon.sh/finalizer"

var (
	// ErrInvalidDatabaseType is the error for an invalid database type
	ErrInvalidDatabaseType = errors.New("invalid database type")

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
		[]string{"name", "namespace", "scope", "type", "username", "databasename", "error"},
	)

	// promDatabaseRequestReconcileStatus is the status of the reconciled database requests
	promDatabaseRequestReconcileStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "databaserequest_reconcile_status",
			Help: "The status of the reconciled database requests",
		},
		[]string{"name", "namespace", "scope", "type", "username", "databasename"},
	)
)

// DatabaseRequestReconciler reconciles a DatabaseRequest object
type DatabaseRequestReconciler struct {
	client.Client
	Scheme                   *runtime.Scheme
	Recorder                 record.EventRecorder
	RelationalDatabaseClient database.RelationalDatabaseInterface
	Locks                    sync.Map
}

const (
	// mysqlType is the MySQL database type
	mysqlType = "mysql"
	// postgresType is the PostgreSQL database type
	postgresType = "postgres"
	// mongodbType is the MongoDB database type
	mongodbType = "mongodb"
)

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

	// We guard any operation by a lock per name and namespace. This is to avoid
	// creating or deleting the same database in parallel. Note that the controller
	// is running in parallel and we allow it to run in parallel for different databases.
	lockKey := fmt.Sprintf("%s/%s", req.Namespace, req.Name)
	r.lock(lockKey)
	defer r.unlock(lockKey)

	databaseRequest := &crdv1alpha1.DatabaseRequest{}
	if err := r.Get(ctx, req.NamespacedName, databaseRequest); err != nil {
		if !apierrors.IsNotFound(err) {
			promDatabaseRequestReconcileErrorCounter.WithLabelValues(req.Name, req.Namespace, "", "", "get-dbreq", "", "").Inc()
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	logger = logger.WithValues("scope", databaseRequest.Spec.Scope, "type", databaseRequest.Spec.Type)
	log.IntoContext(ctx, logger)

	if databaseRequest.DeletionTimestamp != nil && !databaseRequest.DeletionTimestamp.IsZero() {
		return r.deleteDatabase(ctx, databaseRequest)
	}

	if controllerutil.AddFinalizer(databaseRequest, databaseRequestFinalizer) {
		if err := r.Update(ctx, databaseRequest); err != nil {
			return r.handleError(ctx, databaseRequest, "add-finalizer", err, false)
		}
	}

	// Check if we need to reconcile based on Generation and ObservedGeneration but only if
	// the status condition is not false. This makes sure that in case of an error the controller
	// will try to reconcile again.
	if databaseRequest.Status.Conditions != nil && meta.IsStatusConditionTrue(databaseRequest.Status.Conditions, "Ready") {
		if databaseRequest.Status.ObservedGeneration >= databaseRequest.Generation {
			logger.Info("No updates to reconcile")
			r.Recorder.Event(databaseRequest, v1.EventTypeNormal, "ReconcileSkipped", "No updates to reconcile")
			return ctrl.Result{}, nil
		}
	}

	var dbInfo *dbInfo
	if databaseRequest.Spec.Seed != nil {
		seedInfo, err := r.handleSeed(ctx, databaseRequest)
		if err != nil {
			if errIsInvalidCredentials(err) || errIsDatabaseDoesNotExist(err) {
				return r.handleError(ctx, databaseRequest, "invalid-seed", err, true)
			}
			return r.handleError(ctx, databaseRequest, "handle-seed", err, false)
		}
		// now let's update the database request with the connection reference
		databaseRequest.Spec.DatabaseConnectionReference = seedInfo.databaseProviderRef
		databaseRequest.Status.ObservedDatabaseConnectionReference = databaseRequest.Spec.DatabaseConnectionReference
		databaseRequest.Spec.Seed = nil
		dbInfo = seedInfo.dbInfo
		logger.Info("Seed database setup complete")
	} else {
		if databaseRequest.Spec.DatabaseConnectionReference == nil {
			if err := r.createDatabase(ctx, databaseRequest); err != nil {
				return r.handleError(ctx, databaseRequest, "create-database", err, false)
			}
			if databaseRequest.Spec.DatabaseConnectionReference == nil {
				return r.handleError(
					ctx, databaseRequest, "missing-connection-reference", errors.New("missing database connection reference"), false)
			}
		}

		if databaseRequest.Status.ObservedDatabaseConnectionReference != databaseRequest.Spec.DatabaseConnectionReference {
			logger.Info("Database connection reference changed")
			// This means that the database provider has changed and we need to test the connection.
			// We will also update the service and secret BUT we do not create a new database, user or password.
			//
			databaseRequest.Status.ObservedDatabaseConnectionReference = databaseRequest.Spec.DatabaseConnectionReference
		}

		// Note at the moment we only have one "primary" connection per database request
		// Implementing additional users would require to extend the logic here
		// check if the database request is already created and the secret and service exist
		switch databaseRequest.Spec.Type {
		case mysqlType, postgresType:
			logger.Info("Get relational database info")
			// get the database info
			var err error
			dbInfo, err = r.relDBInfo(ctx, databaseRequest)
			if err != nil {
				return r.handleError(
					ctx, databaseRequest, fmt.Sprintf("get-%s-database-info", databaseRequest.Spec.Type), err, false)
			}
		case mongodbType:
			logger.Info("Get mongodb database info")
		default:
			logger.Error(ErrInvalidDatabaseType, "Unsupported database type", "type", databaseRequest.Spec.Type)
		}
	}

	serviceChanged, err := r.handleService(ctx, dbInfo, databaseRequest)
	if err != nil {
		return r.handleError(ctx, databaseRequest, "handle-service", err, false)
	}

	secretChanged, err := r.handleSecret(ctx, dbInfo, databaseRequest)
	if err != nil {
		return r.handleError(ctx, databaseRequest, "handle-secret", err, false)
	}

	promDatabaseRequestReconcileStatus.With(promLabels(databaseRequest, "")).Set(1)
	databaseRequest.Status.ObservedGeneration = databaseRequest.Generation
	// update the CR
	if err := r.Update(ctx, databaseRequest); err != nil {
		promDatabaseRequestReconcileErrorCounter.With(
			promLabels(databaseRequest, "cr-update")).Inc()
		return ctrl.Result{}, err
	}

	// clear the error condition if set
	meta.RemoveStatusCondition(&databaseRequest.Status.Conditions, "Error")
	if serviceChanged || secretChanged {
		if meta.SetStatusCondition(&databaseRequest.Status.Conditions, metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionTrue,
			Reason:  "DatabaseRequestChanged",
			Message: "The database request has been changed",
		}) {
			if err := r.Status().Update(ctx, databaseRequest); err != nil {
				return r.handleError(ctx, databaseRequest, "update-status", err, false)
			}
		}
		r.Recorder.Event(databaseRequest, "Normal", "DatabaseRequestUpdated", "The database request has been updated")
	} else {
		// set the status condition to true if the database request has been created
		if meta.SetStatusCondition(&databaseRequest.Status.Conditions, metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionTrue,
			Reason:  "DatabaseRequestCreated",
			Message: "The database request has been created",
		}) {
			if err := r.Status().Update(ctx, databaseRequest); err != nil {
				return r.handleError(ctx, databaseRequest, "update-status", err, false)
			}
		}
		r.Recorder.Event(databaseRequest, "Normal", "DatabaseRequestUnchanged", "The database request has been created")
	}
	return ctrl.Result{}, nil
}

// handleError handles the error and updates the prometheus metrics. It returns the result and the error
func (r *DatabaseRequestReconciler) handleError(
	ctx context.Context,
	databaseRequest *crdv1alpha1.DatabaseRequest,
	promErr string,
	err error,
	ignoreError bool,
) (ctrl.Result, error) {
	promDatabaseRequestReconcileErrorCounter.With(
		promLabels(databaseRequest, promErr)).Inc()
	promDatabaseRequestReconcileStatus.With(promLabels(databaseRequest, "")).Set(0)
	r.Recorder.Event(databaseRequest, v1.EventTypeWarning, "ReconcileError", err.Error())

	// set status condition to false
	meta.SetStatusCondition(&databaseRequest.Status.Conditions, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  errTypeToEventReason(promErr),
		Message: err.Error(),
	})

	// add additional condition to reflect the error state more clearly
	meta.SetStatusCondition(&databaseRequest.Status.Conditions, metav1.Condition{
		Type:    "Error",
		Status:  metav1.ConditionTrue,
		Reason:  "ReconcileFailed",
		Message: fmt.Sprintf("An error occurred during reconciliation: %v", err),
	})

	// update the status
	if err := r.Status().Update(ctx, databaseRequest); err != nil {
		promDatabaseRequestReconcileErrorCounter.With(
			promLabels(databaseRequest, "update-status")).Inc()
		log.FromContext(ctx).Error(err, "Failed to update status")
	}

	if ignoreError {
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, err

}

func errIsInvalidCredentials(err error) bool {
	return strings.Contains(err.Error(), "invalid credentials")
}

func errIsDatabaseDoesNotExist(err error) bool {
	return strings.Contains(err.Error(), "database does not exist")
}

func (r *DatabaseRequestReconciler) handleSeed(
	ctx context.Context,
	databaseRequest *crdv1alpha1.DatabaseRequest,
) (*seedDatabaseInfo, error) {
	logger := log.FromContext(ctx)
	logger.Info("Get seed database info")
	seedInfo, err := r.relationalDatabaseInfoFromSeed(
		ctx,
		databaseRequest.Spec.Seed,
		databaseRequest.Spec.Type,
		databaseRequest.Spec.Scope,
	)
	if err != nil {
		return nil, err
	}
	logger.Info("Found seed connection", "connectionName", seedInfo.conn.name)

	// if we got a provider connection and db info we set the database info.
	if err = r.RelationalDatabaseClient.SetDatabaseInfo(
		ctx,
		seedInfo.conn.getDSN(false),
		databaseRequest.Name,
		databaseRequest.Namespace,
		databaseRequest.Spec.Type,
		database.RelationalDatabaseInfo{
			Username: seedInfo.dbInfo.userName,
			Password: seedInfo.dbInfo.password,
			Dbname:   seedInfo.dbInfo.database,
		},
	); err != nil {
		return nil, err
	}
	logger.Info("Set database info", "username", seedInfo.dbInfo.userName, "database", seedInfo.dbInfo.database)
	// get rid of the seed secret
	if err := r.Delete(ctx, &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      databaseRequest.Spec.Seed.Name,
			Namespace: databaseRequest.Spec.Seed.Namespace,
		},
	}); err != nil {
		return nil, err
	}
	logger.Info("Deleted seed secret", "secret", databaseRequest.Spec.Seed.Name)
	return seedInfo, nil
}

// handleService creates or updates the service for the database request
// returns true if the service has been updated
func (r *DatabaseRequestReconciler) handleService(
	ctx context.Context, dbInfo *dbInfo, databaseRequest *crdv1alpha1.DatabaseRequest) (bool, error) {
	// Note at the moment we only have one "primary" connection per database request
	// Implementing additional users would require to extend the logic here
	service := &v1.Service{}
	serviceName := databaseRequest.Spec.Name
	if err := r.Get(ctx, types.NamespacedName{
		Name:      serviceName,
		Namespace: databaseRequest.Namespace,
	}, service); err != nil {
		if apierrors.IsNotFound(err) {
			log.FromContext(ctx).Info("Creating service", "service", serviceName)
			r.Recorder.Event(databaseRequest, "Normal", "CreateService", "Creating service")
			service = &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: databaseRequest.Namespace,
					Labels: map[string]string{
						"dbaas.lagoon.sh/service":    "true", // The label could be used to find services in case the hostname changed.
						"app.kubernetes.io/instance": databaseRequest.Name,
					},
				},
				Spec: v1.ServiceSpec{
					Type:         v1.ServiceTypeExternalName,
					ExternalName: dbInfo.hostName,
				},
			}
			if err := r.Create(ctx, service); err != nil {
				return false, fmt.Errorf("failed to create service %s: %w", serviceName, err)
			}
		} else {
			return false, fmt.Errorf("failed to get service %s: %w", serviceName, err)
		}
	} else {
		// update the service if the hostname has changed
		if service.Spec.ExternalName != dbInfo.hostName {
			log.FromContext(ctx).Info("Updating service", "service", service.Name, "hostname", dbInfo.hostName)
			r.Recorder.Event(databaseRequest, "Normal", "UpdateService", "Updating service")
			service.Spec.ExternalName = dbInfo.hostName
			if err := r.Update(ctx, service); err != nil {
				return false, fmt.Errorf("failed to update service %s: %w", serviceName, err)
			}
			return true, nil
		}
	}
	return false, nil
}

// handleSecret creates or updates the secret for the database request
func (r *DatabaseRequestReconciler) handleSecret(
	ctx context.Context, dbInfo *dbInfo, databaseRequest *crdv1alpha1.DatabaseRequest) (bool, error) {
	// Note at the moment we only have one "primary" connection per database request
	// Implementing additional users would require to extend the logic here
	serviceName := databaseRequest.Spec.Name
	secret := &v1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      databaseRequest.Name,
		Namespace: databaseRequest.Namespace,
	}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			log.FromContext(ctx).Info("Creating secret", "secret", databaseRequest.Name)
			r.Recorder.Event(databaseRequest, "Normal", "CreateSecret", "Creating secret")
			secret = &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      databaseRequest.Name,
					Namespace: databaseRequest.Namespace,
					Labels: map[string]string{
						"dbaas.lagoon.sh/secret":     "true",
						"app.kubernetes.io/instance": databaseRequest.Name,
					},
				},
				Data: dbInfo.getSecretData(databaseRequest.Spec.Name, serviceName),
			}
			if err := r.Create(ctx, secret); err != nil {
				return false, fmt.Errorf("failed to create secret %s: %w", databaseRequest.Name, err)
			}
		} else {
			return false, fmt.Errorf("failed to get secret %s: %w", databaseRequest.Name, err)
		}
	} else {
		diff := cmp.Diff(secret.Data, dbInfo.getSecretData(databaseRequest.Spec.Name, serviceName))
		if diff != "" {
			log.FromContext(ctx).Info("Updating secret due to diff")
			r.Recorder.Event(databaseRequest, "Normal", "UpdateSecret", "Updating secret")
			secret.Data = dbInfo.getSecretData(databaseRequest.Spec.Name, serviceName)
			if err := r.Update(ctx, secret); err != nil {
				return false, fmt.Errorf("failed to update secret %s, %w", databaseRequest.Name, err)
			}
			return true, nil
		}
	}
	return false, nil
}

// deleteDatabase deletes the database based on the database request
func (r *DatabaseRequestReconciler) deleteDatabase(
	ctx context.Context, databaseRequest *crdv1alpha1.DatabaseRequest) (ctrl.Result, error) {
	// handle deletion logic
	logger := log.FromContext(ctx)
	if databaseRequest.Spec.Seed == nil && databaseRequest.Spec.DropDatabaseOnDelete {
		switch databaseRequest.Spec.Type {
		case mysqlType, postgresType:
			// handle relational database deletion
			// Note at the moment we only have one "primary" connection per database request
			// Implementing additional users would require to extend the logic here
			logger.Info("Dropping relational database")
			if err := r.relDBDeletion(ctx, databaseRequest); err != nil {
				return r.handleError(ctx, databaseRequest, fmt.Sprintf("%s-drop", databaseRequest.Spec.Type), err, false)
			}
		case mongodbType:
			// handle mongodb deletion
			logger.Info("Dropping MongoDB database")
		default:
			// this should never happen, but just in case
			logger.Error(ErrInvalidDatabaseType, "Unsupported database type", "type", databaseRequest.Spec.Type)
			return r.handleError(ctx, databaseRequest, "invalid-database-type", ErrInvalidDatabaseType, false)
		}
	}
	serviceName := databaseRequest.Spec.Name
	if err := r.Delete(ctx, &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: databaseRequest.Namespace,
		},
	}); err != nil {
		if !apierrors.IsNotFound(err) {
			return r.handleError(ctx, databaseRequest, "delete-service", err, false)
		}
	}
	if err := r.Delete(ctx, &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      databaseRequest.Name,
			Namespace: databaseRequest.Namespace,
		},
	}); err != nil {
		if !apierrors.IsNotFound(err) {
			return r.handleError(ctx, databaseRequest, "delete-secret", err, false)
		}
	}
	if controllerutil.RemoveFinalizer(databaseRequest, databaseRequestFinalizer) {
		if err := r.Update(ctx, databaseRequest); err != nil {
			return r.handleError(ctx, databaseRequest, "remove-finalizer", err, false)
		}
	}
	// record the event
	r.Recorder.Event(
		databaseRequest,
		v1.EventTypeNormal,
		"DeletedDatabase",
		fmt.Sprintf("Deleted database %s/%s", databaseRequest.Namespace, databaseRequest.Name),
	)
	// cleanup metrics
	promDatabaseRequestReconcileStatus.DeletePartialMatch(prometheus.Labels{
		"name":      databaseRequest.Name,
		"namespace": databaseRequest.Namespace,
	})
	return ctrl.Result{}, nil
}

// createDatabase creates the database based on the database request
func (r *DatabaseRequestReconciler) createDatabase(
	ctx context.Context, databaseRequest *crdv1alpha1.DatabaseRequest) error {
	logger := log.FromContext(ctx)
	switch databaseRequest.Spec.Type {
	case mysqlType, postgresType:
		// handle relational database creation
		// Note at the moment we only have one "primary" connection per database request
		// Implementing additional users would require to extend the logic here
		logger.Info("Creating relational database")
		if err := r.relationalDatabaseOperation(ctx, create, databaseRequest, nil); err != nil {
			return fmt.Errorf("%s db creation failed: %w", databaseRequest.Spec.Type, err)
		}
		if databaseRequest.Spec.DatabaseConnectionReference == nil {
			return fmt.Errorf("%s db creation failed due to missing database connection reference", databaseRequest.Spec.Type)
		}
		if databaseRequest.Status.DatabaseInfo == nil {
			return fmt.Errorf("%s db creation failed due to missing database info", databaseRequest.Spec.Type)
		}
	case mongodbType:
		logger.Info("Creating MongoDB database")
	default:
		// this should never happen, but just in case
		logger.Error(ErrInvalidDatabaseType, "Unsupported database type", "type", databaseRequest.Spec.Type)
		return fmt.Errorf("failed to create database: %w", ErrInvalidDatabaseType)
	}

	return nil
}

// seedDatabase returns the database information from the seed secret
func (r *DatabaseRequestReconciler) seedDatabase(
	ctx context.Context,
	seed *v1.SecretReference,
) (*dbInfo, error) {
	secret := &v1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      seed.Name,
		Namespace: seed.Namespace,
	}, secret); err != nil {
		return nil, fmt.Errorf("failed to get seed secret %s: %w", seed.Name, err)
	}
	return dbInfoFromSeed(secret)
}

// promLabels returns the prometheus labels for the database request
func promLabels(databaseRequest *crdv1alpha1.DatabaseRequest, withError string) prometheus.Labels {
	var username, databaseName string
	if databaseRequest.Status.DatabaseInfo != nil {
		username = databaseRequest.Status.DatabaseInfo.Username
		databaseName = databaseRequest.Status.DatabaseInfo.Databasename
	}
	labels := prometheus.Labels{
		"name":         databaseRequest.Name,
		"namespace":    databaseRequest.Namespace,
		"scope":        databaseRequest.Spec.Scope,
		"type":         databaseRequest.Spec.Type,
		"username":     username,
		"databasename": databaseName,
	}
	if withError != "" {
		labels["error"] = withError
	}
	return labels
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseRequestReconciler) SetupWithManager(mgr ctrl.Manager, maxConcurrentReconciles int) error {
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
		// allow running in parallel
		WithOptions(controller.Options{
			MaxConcurrentReconciles: maxConcurrentReconciles,
		}).
		Complete(r)
}

const (
	drop   = "drop"
	create = "create"
	info   = "info"
)

// dbInfo is a simple struct to hold the database information
type dbInfo struct {
	database string
	hostName string
	userName string
	password string
	port     int
}

// getSecretData returns the secret data for the database
func (m *dbInfo) getSecretData(name, serviceName string) map[string][]byte {
	name = strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
	return map[string][]byte{
		fmt.Sprintf("%s_USERNAME", strings.ToUpper(name)): []byte(m.userName),
		fmt.Sprintf("%s_PASSWORD", strings.ToUpper(name)): []byte(m.password),
		fmt.Sprintf("%s_DATABASE", strings.ToUpper(name)): []byte(m.database),
		fmt.Sprintf("%s_HOST", strings.ToUpper(name)):     []byte(serviceName),
		fmt.Sprintf("%s_PORT", strings.ToUpper(name)):     []byte(fmt.Sprintf("%d", m.port)),
	}
}

// dbInfoFromSeed returns a dbInfo struct from the seed secret
func dbInfoFromSeed(secret *v1.Secret) (*dbInfo, error) {
	// check if the secret has all the required keys
	info := &dbInfo{}
	if val, ok := secret.Data["database"]; !ok {
		return nil, errors.New("missing database key in seed secret")
	} else {
		info.database = string(val)
	}

	if val, ok := secret.Data["hostname"]; !ok {
		return nil, errors.New("missing hostname key in seed secret")
	} else {
		info.hostName = string(val)
	}

	if val, ok := secret.Data["username"]; !ok {
		return nil, errors.New("missing username key in seed secret")
	} else {
		info.userName = string(val)
	}

	if val, ok := secret.Data["password"]; !ok {
		return nil, errors.New("missing password key in seed secret")
	} else {
		info.password = string(val)
	}

	if val, ok := secret.Data["port"]; !ok {
		return nil, errors.New("missing port key in seed secret")
	} else {
		port, err := strconv.Atoi(string(val))
		if err != nil {
			return nil, fmt.Errorf("failed to convert port to int: %w", err)
		}
		info.port = port
	}

	return info, nil
}

// relationalDatabaseOperation performs the relational database operations to create, drop and get database information
func (r *DatabaseRequestReconciler) relationalDatabaseOperation(
	ctx context.Context,
	operation string,
	databaseRequest *crdv1alpha1.DatabaseRequest,
	databaseInfo *dbInfo,
) error {
	log.FromContext(ctx).Info("Performing relational database operation", "operation", operation)

	// get the database provider, for info and drop we use the reference which is already set on the database request
	// if not we error out.
	// For create we list all database providers and check if the scope matches and if
	// there are more than one provider with the same scope, we select the one with lower load.
	databaseProvider := &crdv1alpha1.RelationalDatabaseProvider{}
	connectionName := ""
	if operation == create {
		var err error
		databaseProvider, connectionName, err = r.findRelationalDatabaseProvider(ctx, databaseRequest)
		if err != nil {
			return fmt.Errorf(
				"%s db operation %s failed to find database provider: %w", databaseRequest.Spec.Type, operation, err)
		}
		log.FromContext(ctx).Info(
			"Found relational database provider", "provider", databaseProvider.Name, "connection", connectionName)
	} else {
		if databaseRequest.Spec.DatabaseConnectionReference == nil {
			return fmt.Errorf(
				"%s db operation %s failed due to missing database connection reference", databaseRequest.Spec.Type, operation)
		}
		if err := r.Get(ctx, client.ObjectKey{
			Name: databaseRequest.Spec.DatabaseConnectionReference.DatabaseObjectReference.Name,
		}, databaseProvider); err != nil {
			return fmt.Errorf(
				"%s db operation %s failed to get database provider: %w", databaseRequest.Spec.Type, operation, err)
		}
		connectionName = databaseRequest.Spec.DatabaseConnectionReference.Name
		log.FromContext(ctx).Info(
			"Found relational database provider", "provider", databaseProvider.Name, "connection", connectionName)
	}

	var connection *crdv1alpha1.Connection
	for _, c := range databaseProvider.Spec.Connections {
		log.FromContext(ctx).Info("Checking relational database provider database connection", "connection", c.Name)
		if c.Name == connectionName {
			conn := c          // Create a new variable and assign the value of c to it
			connection = &conn // Assign the address of the new variable to connection
		}
	}
	if connection == nil {
		return fmt.Errorf("%s db operation %s failed to find database connection", databaseRequest.Spec.Type, operation)
	}

	secret := &v1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      connection.PasswordSecretRef.Name,
		Namespace: connection.PasswordSecretRef.Namespace,
	}, secret); err != nil {
		return fmt.Errorf(
			"%s db operation %s failed to get connection password from secret: %w", databaseRequest.Spec.Type, operation, err)
	}

	password := string(secret.Data["password"])
	if password == "" {
		return fmt.Errorf("%s db operation %s failed due to empty password", databaseRequest.Spec.Type, operation)
	}

	conn := reldbConn{
		dbType:   databaseRequest.Spec.Type,
		name:     connection.Name,
		hostname: connection.Hostname,
		username: connection.Username,
		password: password,
		port:     connection.Port,
	}

	switch operation {
	case create:
		log.FromContext(ctx).Info("Creating relational database", "database", databaseRequest.Name)
		info, err := r.RelationalDatabaseClient.CreateDatabase(
			ctx,
			conn.getDSN(false),
			databaseRequest.Name,
			databaseRequest.Namespace,
			databaseRequest.Spec.Type,
		)
		if err != nil {
			return fmt.Errorf("%s db operation %s failed: %w", databaseRequest.Spec.Type, operation, err)
		}
		dbRef := &crdv1alpha1.DatabaseConnectionReference{
			Name: connection.Name,
			DatabaseObjectReference: v1.ObjectReference{
				Kind:            databaseProvider.Kind,
				Name:            databaseProvider.Name,
				UID:             databaseProvider.UID,
				ResourceVersion: databaseProvider.ResourceVersion,
			},
		}
		databaseRequest.Status.ObservedDatabaseConnectionReference = dbRef
		databaseRequest.Status.DatabaseInfo = &crdv1alpha1.DatabaseInfo{
			Username:     info.Username,
			Databasename: info.Dbname,
		}
		if err := r.Status().Update(ctx, databaseRequest); err != nil {
			return fmt.Errorf(
				"%s db operation %s failed to update database request: %w", databaseRequest.Spec.Type, operation, err)
		}
		databaseRequest.Spec.DatabaseConnectionReference = dbRef
		return nil
	case drop:
		if err := r.RelationalDatabaseClient.DropDatabase(
			ctx,
			conn.getDSN(false),
			databaseRequest.Name,
			databaseRequest.Namespace,
			databaseRequest.Spec.Type,
		); err != nil {
			return fmt.Errorf("%s db opration %s failed: %w", databaseRequest.Spec.Type, operation, err)
		}
		databaseRequest.Status.ObservedDatabaseConnectionReference = nil
		if err := r.Status().Update(ctx, databaseRequest); err != nil {
			return fmt.Errorf(
				"%s db operation %s failed to update database request: %w", databaseRequest.Spec.Type, operation, err)
		}
		databaseRequest.Spec.DatabaseConnectionReference = nil
		return nil
	case info:
		// check if the dbInfo is not nil
		if databaseInfo == nil {
			return fmt.Errorf("%s db operation %s failed due to missing dbInfo", databaseRequest.Spec.Type, operation)
		}
		// get the database information
		info, err := r.RelationalDatabaseClient.GetDatabaseInfo(
			ctx,
			conn.getDSN(false),
			databaseRequest.Name,
			databaseRequest.Namespace,
			databaseRequest.Spec.Type,
		)
		if err != nil {
			return fmt.Errorf(
				"%s db operation %s failed to get database information: %w", databaseRequest.Spec.Type, operation, err)
		}
		databaseInfo.userName = info.Username
		databaseInfo.password = info.Password
		databaseInfo.database = info.Dbname
		databaseInfo.hostName = conn.hostname
		databaseInfo.port = conn.port
		return nil
	default:
		return fmt.Errorf("%s db operation %s failed due to invalid operation", databaseRequest.Spec.Type, operation)
	}
}

// seedDatabaseInfo is a struct to hold the seed database information
type seedDatabaseInfo struct {
	dbInfo              *dbInfo
	conn                *reldbConn
	databaseProviderRef *crdv1alpha1.DatabaseConnectionReference
}

// relationalDatabaseInfoFromSeed finds the relational database provider based on the seed secret
func (r *DatabaseRequestReconciler) relationalDatabaseInfoFromSeed(
	ctx context.Context,
	seed *v1.SecretReference,
	dbType string,
	scope string,
) (*seedDatabaseInfo, error) {
	dbInfo, err := r.seedDatabase(ctx, seed)
	if err != nil {
		return nil, fmt.Errorf("%s db find connection from seed failed to get seed database: %w", dbType, err)
	}

	// test if the connection works with the seed
	if err := r.relDBTestSeedConnection(ctx, dbInfo, dbType); err != nil {
		return nil, fmt.Errorf("%s db find connection from seed failed to test connection: %w", dbType, err)
	}

	dbProviders := &crdv1alpha1.RelationalDatabaseProviderList{}
	if err := r.List(ctx, dbProviders); err != nil {
		return nil, fmt.Errorf("%s db find connection from seed failed to list database providers: %w",
			dbType, err,
		)
	}

	var connection *crdv1alpha1.Connection
	var databaseProviderRef *crdv1alpha1.DatabaseConnectionReference
	for _, dbProvider := range dbProviders.Items {
		if dbProvider.Spec.Scope == scope && dbProvider.Spec.Type == dbType {
			for _, dbConnection := range dbProvider.Spec.Connections {
				if dbConnection.Enabled && dbConnection.Hostname == dbInfo.hostName &&
					dbConnection.Port == dbInfo.port {
					log.FromContext(ctx).Info("Found provider", "provider", dbProvider.Name)
					conn := dbConnection
					connection = &conn
					databaseProviderRef = &crdv1alpha1.DatabaseConnectionReference{
						Name: connection.Name,
						DatabaseObjectReference: v1.ObjectReference{
							Kind:            dbProvider.Kind,
							Name:            dbProvider.Name,
							UID:             dbProvider.UID,
							ResourceVersion: dbProvider.ResourceVersion,
						},
					}
				}
			}
		}
	}

	if connection == nil {
		return nil, fmt.Errorf("%s db find connection from seed failed due to provider not found", dbType)
	}

	secret := &v1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      connection.PasswordSecretRef.Name,
		Namespace: connection.PasswordSecretRef.Namespace,
	}, secret); err != nil {
		return nil, fmt.Errorf("%s db find connection from seed failed to get connection password from secret: %w",
			dbType, err,
		)
	}

	password := string(secret.Data["password"])
	if password == "" {
		return nil, fmt.Errorf("%s db find connection from seed failed due to empty password", dbType)
	}

	conn := &reldbConn{
		dbType:   dbType,
		name:     connection.Name,
		hostname: connection.Hostname,
		username: connection.Username,
		password: password,
		port:     connection.Port,
	}

	return &seedDatabaseInfo{dbInfo: dbInfo, conn: conn, databaseProviderRef: databaseProviderRef}, nil
}

// findRelationalDatabaseProvider finds the relational database provider with the same scope and the lower load
// returns the provider, connection name and an error
func (r *DatabaseRequestReconciler) findRelationalDatabaseProvider(
	ctx context.Context,
	databaseRequest *crdv1alpha1.DatabaseRequest,
) (*crdv1alpha1.RelationalDatabaseProvider, string, error) {
	dbProviders := &crdv1alpha1.RelationalDatabaseProviderList{}
	if err := r.List(ctx, dbProviders); err != nil {
		return nil, "", fmt.Errorf("%s db find provider failed to list database providers: %w",
			databaseRequest.Spec.Type, err,
		)
	}

	// find the provider with the same scope
	// set load to the max int value to find the provider with the lower
	load := int(^uint(0) >> 1)
	var provider *crdv1alpha1.RelationalDatabaseProvider
	var connName string
	for _, dbProvider := range dbProviders.Items {
		if dbProvider.Spec.Scope == databaseRequest.Spec.Scope && dbProvider.Spec.Type == databaseRequest.Spec.Type {
			log.FromContext(ctx).Info("Found provider", "provider", dbProvider.Name)
			for _, dbConnection := range dbProvider.Spec.Connections {
				if dbConnection.Enabled {
					// fetch the password from the secret
					secret := &v1.Secret{}
					if err := r.Get(ctx, types.NamespacedName{
						Name:      dbConnection.PasswordSecretRef.Name,
						Namespace: dbConnection.PasswordSecretRef.Namespace,
					}, secret); err != nil {
						return nil, "", fmt.Errorf("%s db find provider failed to get connection password from secret: %w",
							databaseRequest.Spec.Type, err,
						)
					}

					password := string(secret.Data["password"])
					if password == "" {
						return nil, "", fmt.Errorf("%s db find provider failed due to empty password", databaseRequest.Spec.Type)
					}

					conn := reldbConn{
						dbType:   databaseRequest.Spec.Type,
						name:     dbConnection.Name,
						hostname: dbConnection.Hostname,
						username: dbConnection.Username,
						password: password,
						port:     dbConnection.Port,
					}

					// check the load of the provider connection
					// we select the provider with the lower load
					log.FromContext(ctx).Info("Checking provider database connection", "connection", dbConnection.Name)
					dbLoad, err := r.RelationalDatabaseClient.Load(ctx, conn.getDSN(false), databaseRequest.Spec.Type)
					if err != nil {
						return nil, "", fmt.Errorf("%s db find provider failed to get load: %w", databaseRequest.Spec.Type, err)
					}
					if dbLoad < load {
						p := dbProvider
						provider = &p
						connName = dbConnection.Name
						load = dbLoad
						log.FromContext(ctx).Info("Found relational database provider", "provider",
							dbProvider.Name, "connection", dbConnection.Name, "load", dbLoad)
					}
				}
			}
		}
	}
	if provider == nil {
		return nil, "", fmt.Errorf("%s db find provider failed due to provider not found", databaseRequest.Spec.Type)
	}
	return provider, connName, nil
}

// relDBDeletion deletes the relational database
func (r *DatabaseRequestReconciler) relDBDeletion(
	ctx context.Context,
	databaseRequest *crdv1alpha1.DatabaseRequest,
) error {
	log.FromContext(ctx).Info("Deleting relational database")

	// check the status to find the object reference to the database provider
	if databaseRequest.Spec.DatabaseConnectionReference == nil {
		// if there is no reference, we can't delete the database.
		return errors.New("relational db drop failed due to connection reference is missing")
	}
	return r.relationalDatabaseOperation(ctx, drop, databaseRequest, nil)
}

// relDBInfo retrieves the relational database information
func (r *DatabaseRequestReconciler) relDBInfo(
	ctx context.Context,
	databaseRequest *crdv1alpha1.DatabaseRequest,
) (*dbInfo, error) {
	log.FromContext(ctx).Info("Retrieving relational database information")

	dbInfo := dbInfo{}
	if err := r.relationalDatabaseOperation(ctx, info, databaseRequest, &dbInfo); err != nil {
		return nil, fmt.Errorf("relational db info failed: %w", err)
	}
	return &dbInfo, nil
}

// relDBTestSeedConnection tests a mysql or postgres connection
func (r *DatabaseRequestReconciler) relDBTestSeedConnection(
	ctx context.Context,
	dbi *dbInfo,
	dbType string,
) error {
	log.FromContext(ctx).Info("Testing relational database connection connection")
	conn := reldbConn{
		dbType:   dbType,
		hostname: dbi.hostName,
		username: dbi.userName,
		password: dbi.password,
		port:     dbi.port,
		name:     dbi.database,
	}
	if err := r.RelationalDatabaseClient.Ping(ctx, conn.getDSN(true), dbType); err != nil {
		return fmt.Errorf("relational database test connection failed: %w", err)
	}
	return nil
}

// lock is a simple lock implementation to avoid creating the same database in parallel
func (r *DatabaseRequestReconciler) lock(key string) {
	mu, _ := r.Locks.LoadOrStore(key, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
}

// unlock is a simple lock implementation to avoid creating the same database in parallel
func (r *DatabaseRequestReconciler) unlock(key string) {
	if mu, ok := r.Locks.Load(key); ok {
		mu.(*sync.Mutex).Unlock()
	} // the not ok case should never happen...
}
