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

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AdditionalUser defines the additional user to be created
type AdditionalUser struct {
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:Enum=read-only;read-write
	//+kubebuilder:default:=read-only
	// Type is the type of user to be created
	// it can be either "read-only" or "read-write"
	Type string `json:"type"`

	//+kubebuilder:required
	//+kubebuilder:validation:Required
	// Name is the name of the service we are creating. Similar to the name of the DatabaseRequestSpec.
	// for example mariadb-read-only-0
	Name string `json:"name"`
}

// DatabaseConnectionReference defines the reference to a database connection
type DatabaseConnectionReference struct {
	//+kubebuilder:required
	// DatabaseObjectReference is the reference to the database object.
	// Note that this is a way for the provider to find all database requests
	// that are using the same database connection and update them if necessary.
	DatabaseObjectReference v1.ObjectReference `json:"databaseObjectReference"`

	//+kubebuilder:required
	// Name is the name of the database connection.
	Name string `json:"name"`
}

// DatabaseRequestSpec defines the desired state of DatabaseRequest
type DatabaseRequestSpec struct {
	//+kubebuilder:required
	//+kubebuilder:validation:Required
	// Name is used for the service name and the prefix in the secret data
	// for example mariadb-0
	Name string `json:"name"`

	//+kubebuilder:required
	//+kubebuilder:validation:Required
	// Selector is the name of the database request, this is used to select a provider from a pool of providers with the same selector
	Selector string `json:"selector"`

	//+kubebuilder:required
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:Enum=mysql;mariadb;postgres;mongodb
	// Type is the type of the database request
	// it can be either "mysql" or "mariadb" or "postgres" or "mongodb"
	Type string `json:"type"`

	//+kubebuilder:optional
	// Seed is the seed for the database request
	// it is a reference to a local secret within the same namespace
	Seed *v1.SecretReference `json:"seed,omitempty"`

	//+kubebuilder:optional
	// AdditionalUsers defines the additional users to be created
	AdditionalUsers []AdditionalUser `json:"additionalUsers,omitempty"`

	//+kubebuilder:optional
	// DatabaseConnectionReference is the reference to a database connection. This makes it possible for the
	// database provider to update the database request if necessary by updating the referenced object.
	DatabaseConnectionReference *DatabaseConnectionReference `json:"databaseConnectionReference,omitempty"`

	//+kubebuilder:default:=true
	// DropDatabaseOnDelete defines if the database should be dropped when the request is deleted
	DropDatabaseOnDelete bool `json:"dropDatabaseOnDelete,omitempty"`

	//+kubebuilder:optional
	// ForcedReconcilation is a timestamp based field to force the reconciliation of the database request
	// This field is used to force the reconciliation of the database request.
	ForcedReconcilation *metav1.Time `json:"forcedReconcilation,omitempty"`
}

// DatabaseInfo provides some database information
type DatabaseInfo struct {
	//+kubebuilder:required
	// Username is the username of the database
	Username string `json:"username"`
	//+kubebuilder:required
	// DatabaseName is the name of the database
	Databasename string `json:"databasename"`
}

// DatabaseRequestStatus defines the observed state of DatabaseRequest
type DatabaseRequestStatus struct {
	//+kubebuilder:optional
	// Conditions is the observed conditions
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	//+kubebuilder:optional
	// ObservedGeneration is the last observed generation
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	//+kubebuilder:optional
	// ObservedDatabaseConnectionReference is the observed database connection reference
	// This is a way for the controller to know if the database provider has updated the database connection.
	ObservedDatabaseConnectionReference *DatabaseConnectionReference `json:"observedDatabaseConnectionReference,omitempty"`

	//+kubebuilder:optional
	// DatabaseInfo is the database information
	DatabaseInfo *DatabaseInfo `json:"databaseInfo,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// DatabaseRequest is the Schema for the databaserequests API
type DatabaseRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseRequestSpec   `json:"spec,omitempty"`
	Status DatabaseRequestStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DatabaseRequestList contains a list of DatabaseRequest
type DatabaseRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatabaseRequest{}, &DatabaseRequestList{})
}
