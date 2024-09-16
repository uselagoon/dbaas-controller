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

// MongoDBAuth defines the authorisation mechanisms that mongo can use
type MongoDBAuth struct {
	//+kubebuilder:required
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:Enum=SCRAM-SHA-1;SCRAM-SHA-256;MONGODB-CR;MongoDB-AWS;X509
	// Mechanism is the authentication mechanism for the MongoDB connection
	// https://www.mongodb.com/docs/drivers/go/current/fundamentals/auth/#std-label-golang-authentication-mechanisms
	Mechanism string `json:"mechanism"`

	//+kubebuilder:optional
	//+kubebuilder:default:admin
	// Source is the source of the authentication mechanism for the MongoDB connection
	Source string `json:"source,omitempty"`

	//+kubebuilder:optional
	//+kubebuilder:default:true
	// TLS is the flag to enable or disable TLS for the MongoDB connection
	TLS bool `json:"tls,omitempty"`
}

type MongoDBConnection struct {
	//+kubebuilder:required
	// Name is the name of the MongoDB connection
	// it is used to identify the connection. Please use a unique name
	// for each connection. This field will be used in the MongoDBProvider
	// to reference the connection. The MongoDBProvider controller will
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

	//+kubebuilder:optional
	//+kubebuilder:default:=27017
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:Minimum=1
	//+kubebuilder:validation:Maximum=65535
	// Port is the port of the relational database
	Port int `json:"port,omitempty"`

	//+kubebuilder:optional
	//+kubebuilder:default:=root
	// Username is the username of the relational database
	Username string `json:"username,omitempty"`

	//+kubebuilder:required
	// Auth is the authentication mechanism for the MongoDB connection
	Auth MongoDBAuth `json:"auth"`

	//+kubebuilder:optional
	//+kubebuilder:default:=true
	// Enabled is a flag to enable or disable the relational database
	Enabled bool `json:"enabled,omitempty"`
}

// MongoDBProviderSpec defines the desired state of MongoDBProvider
type MongoDBProviderSpec struct {
	//+kubebuilder:required
	//+kubebuilder:validation:Required
	// Scope is the scope of the database request, this is used to select a provider from a pool of scopes
	Scope string `json:"scope"`

	//+kubebuilder:validation:MinItems=1
	// Connections defines the connection to a relational database
	Connections []MongoDBConnection `json:"connections"`
}

// MongoDBProviderStatus defines the observed state of MongoDBProvider
type MongoDBProviderStatus struct {
	// Conditions defines the status conditions
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ConnectionStatus provides the status of the relational database
	ConnectionStatus []ConnectionStatus `json:"connectionStatus,omitempty"` // nolint:lll

	// ObservedGeneration is the last observed generation
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// MongoDBProvider is the Schema for the mongodbproviders API
type MongoDBProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MongoDBProviderSpec   `json:"spec,omitempty"`
	Status MongoDBProviderStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MongoDBProviderList contains a list of MongoDBProvider
type MongoDBProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MongoDBProvider `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MongoDBProvider{}, &MongoDBProviderList{})
}
