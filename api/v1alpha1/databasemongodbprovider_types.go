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

// MongoDBConnection defines the connection to a MongoDB database
type MongoDBConnection struct {
	// Name is the name of the mongodb database connection
	// it is used to identify the connection. Please use a unique name
	// for each connection. This field will be used in the DatabaseRequest
	// to reference the connection. The mongodb provider controller will
	// error if the name is not unique.
	Name string `json:"name"`

	//+kubebuilder:required
	// Hostname is the hostname of the MongoDB database
	Hostname string `json:"hostname"`

	//+kubebuilder:required
	// PasswordSecretRef is the reference to the secret containing the password
	PasswordSecretRef v1.ObjectReference `json:"passwordSecretRef"`

	//+kubebuilder:required
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:Minimum=1
	//+kubebuilder:validation:Maximum=65535
	// Port is the port of the MongoDB database
	Port int `json:"port"`

	//+kubebuilder:required
	// Username is the username of the MongoDB database
	Username string `json:"username"`

	//+kubebuilder:optional
	//+kubebuilder:validation:Enum=SCRAM-SHA-1;SCRAM-SHA-256
	//+kubebuilder:default:=SCRAM-SHA-1
	// Mechanism is the authentication mechanism to use
	Mechanism string `json:"mechanism,omitempty"`

	//+kubebuilder:optional
	//+kubebuilder:validation:Enum=admin;source
	//+kubebuilder:default:=admin
	// Source is the authentication source to use
	Source string `json:"source,omitempty"`

	//+kubebuilder:optional
	//+kubebuilder:default:=true
	// UseTLS is a flag to enable or disable TLS
	UseTLS bool `json:"useTLS,omitempty"`

	//+kubebuilder:optional
	//+kubebuilder:default:=false
	// InsecureTLS is a flag to enable or disable insecure TLS
	InsecureTLS bool `json:"insecureTLS,omitempty"`
}

// DatabaseMongoDBProviderSpec defines the desired state of DatabaseMongoDBProvider
type DatabaseMongoDBProviderSpec struct {
	//+kubebuilder:required
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:Enum=production;development;custom
	//+kubebuilder:default:=development
	// Scope is the scope of the database request
	// it can be either "production" or "development" or "custom"
	Scope string `json:"scope"`

	//+kubebuilder:required
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:MinItems=1
	// MongoDBConnections is the list of MongoDB connections
	MongoDBConnections []MongoDBConnection `json:"mongodbConnections"`
}

// DatabaseMongoDBProviderStatus defines the observed state of DatabaseMongoDBProvider
type DatabaseMongoDBProviderStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster

// DatabaseMongoDBProvider is the Schema for the databasemongodbproviders API
type DatabaseMongoDBProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseMongoDBProviderSpec   `json:"spec,omitempty"`
	Status DatabaseMongoDBProviderStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DatabaseMongoDBProviderList contains a list of DatabaseMongoDBProvider
type DatabaseMongoDBProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseMongoDBProvider `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatabaseMongoDBProvider{}, &DatabaseMongoDBProviderList{})
}
