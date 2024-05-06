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

// Connection defines the connection to a relational database like MySQL or PostgreSQL
type Connection struct {
	// Name is the name of the relational database like MySQL or PostgreSQL connection
	// it is used to identify the connection. Please use a unique name
	// for each connection. This field will be used in the DatabaseRequest
	// to reference the connection. The relationaldatabaseprovider controller will
	// error if the name is not unique.
	Name string `json:"name"`

	//+kubebuilder:required
	// Hostname is the hostname of the relational database
	Hostname string `json:"hostname"`

	//+kubebuilder:optional
	// ReplicaHostnames is the list of hostnames of the relational database replicas
	ReplicaHostnames []string `json:"replicaHostnames,omitempty"`

	//+kubebuilder:required
	// PasswordSecretRef is the reference to the secret containing the password
	PasswordSecretRef v1.SecretReference `json:"passwordSecretRef"`

	//+kubebuilder:required
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:Minimum=1
	//+kubebuilder:validation:Maximum=65535
	// Port is the port of the relational database
	Port int `json:"port"`

	//+kubebuilder:required
	// Username is the username of the relational database
	Username string `json:"username"`

	//+kubebuilder:required
	//+kubebuilder:default:=true
	// Enabled is a flag to enable or disable the relational database
	Enabled bool `json:"enabled"`
}

// RelationalDatabaseProviderSpec defines the desired state of RelationalDatabaseProvider
type RelationalDatabaseProviderSpec struct {
	//+kubebuilder:required
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:Enum=mysql;postgres
	// Type is the type of the relational database provider
	// it can be either "mysql" or "postgres"
	Type string `json:"type"`

	//+kubebuilder:required
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:Enum=production;development;custom
	//+kubebuilder:default:=development
	// Scope is the scope of the database request
	// it can be either "production" or "development" or "custom"
	Scope string `json:"scope"`

	//+kubebuilder:validation:MinItems=1
	// Connections defines the connection to a relational database
	Connections []Connection `json:"connections"`
}

// ConnectionStatus defines the status of a relational database connection
type ConnectionStatus struct {
	//+kubebuilder:required
	// Name is the name of the relational database connection
	// it is used to identify the connection. Please use a unique name
	// for each connection. This field will be used in the DatabaseRequest
	// to reference the connection. The relationaldatabaseprovider controller will
	// error if the name is not unique.
	Name string `json:"name"`

	//+kubebuilder:required
	// Hostname is the hostname of the relational database
	Hostname string `json:"hostname"`

	//+kubebuilder:required
	// DatabaseVersion is the version of the relational database
	DatabaseVersion string `json:"databaseVersion"`

	//+kubebuilder:required
	//+kubebuilder:validation:Required
	// Enabled is a flag to indicate whether a MySQL database is enabled or not
	Enabled bool `json:"enabled"`

	//+kubebuilder:required
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:Enum=available;unavailable
	// Status is the status of the relational database
	Status string `json:"status"`
}

// RelationalDatabaseProviderStatus defines the observed state of RelationalDatabaseProvider
type RelationalDatabaseProviderStatus struct {
	// Conditions defines the status conditions
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ConnectionStatus provides the status of the relational database
	ConnectionStatus []ConnectionStatus `json:"connectionStatus,omitempty"` // nolint:lll

	// ObservedGeneration is the last observed generation
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster

// RelationalDatabaseProvider is the Schema for the relationaldatabaseprovider API
type RelationalDatabaseProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RelationalDatabaseProviderSpec   `json:"spec,omitempty"`
	Status RelationalDatabaseProviderStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RelationalDatabaseProviderList contains a list of RelationalDatabaseProvider
type RelationalDatabaseProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RelationalDatabaseProvider `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RelationalDatabaseProvider{}, &RelationalDatabaseProviderList{})
}
