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

// PostgreSQLConnection defines the connection to a PostgreSQL database
type PostgreSQLConnection struct {
	// Name is the name of the postgres database connection
	// it is used to identify the connection. Please use a unique name
	// for each connection. This field will be used in the DatabaseRequest
	// to reference the connection. The postgres provider controller will
	// error if the name is not unique.
	Name string `json:"name"`

	//+kubebuilder:required
	// Hostname is the hostname of the PostgreSQL database
	Hostname string `json:"hostname"`

	//+kubebuilder:required
	// PasswordSecretRef is the reference to the secret containing the password
	PasswordSecretRef v1.ObjectReference `json:"passwordSecretRef"`

	//+kubebuilder:required
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:Minimum=1
	//+kubebuilder:validation:Maximum=65535
	// Port is the port of the PostgreSQL database
	Port int `json:"port"`

	//+kubebuilder:required
	// Username is the username of the PostgreSQL database
	Username string `json:"username"`
}

// DatabasePostgreSQLProviderSpec defines the desired state of DatabasePostgreSQLProvider
type DatabasePostgreSQLProviderSpec struct {
	//+kubebuilder:required
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:Enum=production;development;custom
	//+kubebuilder:default:=development
	// Scope is the scope of the database request
	// it can be either "production" or "development" or "custom"
	Scope string `json:"scope"`

	//+kubebuilder:validation:MinItems=1
	// PostgreSQLConnections defines the connection to a PostgreSQL database
	PostgreSQLConnections []PostgreSQLConnection `json:"postgresqlConnections"`
}

// DatabasePostgreSQLProviderStatus defines the observed state of DatabasePostgreSQLProvider
type DatabasePostgreSQLProviderStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster

// DatabasePostgreSQLProvider is the Schema for the databasepostgresqlproviders API
type DatabasePostgreSQLProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabasePostgreSQLProviderSpec   `json:"spec,omitempty"`
	Status DatabasePostgreSQLProviderStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DatabasePostgreSQLProviderList contains a list of DatabasePostgreSQLProvider
type DatabasePostgreSQLProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabasePostgreSQLProvider `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatabasePostgreSQLProvider{}, &DatabasePostgreSQLProviderList{})
}
