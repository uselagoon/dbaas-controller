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

// MySQLConnection defines the connection to a MySQL database
type MySQLConnection struct {
	// Name is the name of the MySQL database connection
	// it is used to identify the connection. Please use a unique name
	// for each connection. This field will be used in the DatabaseRequest
	// to reference the connection. The databasemysqlprovider controller will
	// error if the name is not unique.
	Name string `json:"name"`

	//+kubebuilder:required
	// Hostname is the hostname of the MySQL database
	Hostname string `json:"hostname"`

	//+kubebuilder:optional
	// ReplicaHostnames is the list of hostnames of the MySQL database replicas
	ReplicaHostnames []string `json:"replicaHostnames,omitempty"`

	//+kubebuilder:required
	// PasswordSecretRef is the reference to the secret containing the password
	PasswordSecretRef v1.SecretReference `json:"passwordSecretRef"`

	//+kubebuilder:required
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:Minimum=1
	//+kubebuilder:validation:Maximum=65535
	// Port is the port of the MySQL database
	Port int `json:"port"`

	//+kubebuilder:required
	// Username is the username of the MySQL database
	Username string `json:"username"`

	//+kubebuilder:required
	//+kubebuilder:default:=true
	// Enabled is a flag to enable or disable the MySQL database
	Enabled bool `json:"enabled"`
}

// DatabaseMySQLProviderSpec defines the desired state of DatabaseMySQLProvider
type DatabaseMySQLProviderSpec struct {
	//+kubebuilder:required
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:Enum=production;development;custom
	//+kubebuilder:default:=development
	// Scope is the scope of the database request
	// it can be either "production" or "development" or "custom"
	Scope string `json:"scope"`

	//+kubebuilder:validation:MinItems=1
	// MySQLConnections defines the connection to a MySQL database
	MySQLConnections []MySQLConnection `json:"mysqlConnections"`
}

type MySQLConnectionVersion struct {
	//+kubebuilder:required
	// Hostname is the hostname of the MySQL database
	Hostname string `json:"hostname"`

	//+kubebuilder:required
	// MySQLVersion is the version of the MySQL database
	MySQLVersion string `json:"mysqlVersion"`
}

// DatabaseMySQLProviderStatus defines the observed state of DatabaseMySQLProvider
type DatabaseMySQLProviderStatus struct {
	// Conditions defines the status conditions
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// MySQLConnectionVersions defines the version of the MySQL database
	MySQLConnectionVersions []MySQLConnectionVersion `json:"mysqlConnectionVersions,omitempty"`

	// ObservedGeneration is the last observed generation
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster

// DatabaseMySQLProvider is the Schema for the databasemysqlproviders API
type DatabaseMySQLProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseMySQLProviderSpec   `json:"spec,omitempty"`
	Status DatabaseMySQLProviderStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DatabaseMySQLProviderList contains a list of DatabaseMySQLProvider
type DatabaseMySQLProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseMySQLProvider `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatabaseMySQLProvider{}, &DatabaseMySQLProviderList{})
}
