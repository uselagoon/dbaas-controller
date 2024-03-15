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

// AdditionalUsers defines the additional users to be created
type AdditionalUsers struct {
	//+kubebuilder:required
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:MinItems=1
	Names []string `json:"names"`

	//+kubebuilder:validation:Required
	//+kubebuilder:validation:Enum=read-only;read-write
	//+kubebuilder:default:=read-only
	// Type is the type of user to be created
	// it can be either "read-only" or "read-write"
	Type string `json:"type"`
}

// DatabaseConnectionReference defines the reference to a database connection
type DatabaseConnectionReference struct {
	//+kubebuilder:required
	// DatabaseObjectReference is the reference to the database object
	DatabaseObjectReference v1.ObjectReference `json:"databaseObjectReference"`

	//+kubebuilder:required
	// Name is the name of the database connection.
	Name string `json:"name"`
}

// DatabaseRequestSpec defines the desired state of DatabaseRequest
type DatabaseRequestSpec struct {
	//+kubebuilder:required
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:Enum=production;development;custom
	//+kubebuilder:default:=development
	// Scope is the scope of the database request
	// it can be either "production" or "development" or "custom"
	Scope string `json:"scope"`

	//+kubebuilder:required
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:Enum=mysql;mariadb;postgres;mongodb
	// Type is the type of the database request
	// it can be either "mysql" or "mariadb" or "postgres" or "mongodb"
	Type string `json:"type"`

	//+kubebuilder:optional
	// Seed is the seed for the database request
	// it is a reference to a local secret within the same namespace
	Seed *v1.LocalObjectReference `json:"seed,omitempty"`

	//+kubebuilder:optional
	// AdditionalUsers defines the additional users to be created
	AdditionalUsers *AdditionalUsers `json:"additionalUsers,omitempty"`

	//+kubebuilder:default:=true
	// DropDatabaseOnDelete defines if the database should be dropped when the request is deleted
	DropDatabaseOnDelete bool `json:"dropDatabaseOnDelete,omitempty"`

	//+kubebuilder:optional
	// DatabaseConnectionReference is the reference to a database connection
	DatabaseConnectionReference *DatabaseConnectionReference `json:"databaseConnectionReference,omitempty"`
}

// DatabaseRequestStatus defines the observed state of DatabaseRequest
type DatabaseRequestStatus struct {
	// Conditions is the observed conditions
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the last observed generation
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
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
