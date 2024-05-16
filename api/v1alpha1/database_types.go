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

// ConnectionStatus defines the status of a  database connection, it can be either relational database or mongodb
type ConnectionStatus struct {
	//+kubebuilder:required
	// Name is the name of the database connection
	// it is used to identify the connection. Please use a unique name
	// for each connection. This field will be used in the DatabaseRequest
	// to reference the connection. The relationaldatabaseprovider and mongodbprovider
	// controllers will error if the name is not unique.
	Name string `json:"name"`

	//+kubebuilder:required
	// Hostname is the hostname of the database
	Hostname string `json:"hostname"`

	//+kubebuilder:required
	// DatabaseVersion is the version of the database
	DatabaseVersion string `json:"databaseVersion"`

	//+kubebuilder:required
	//+kubebuilder:validation:Required
	// Enabled is a flag to indicate whether a database is enabled or not
	Enabled bool `json:"enabled"`

	//+kubebuilder:required
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:Enum=available;unavailable
	// Status is the status of the database
	Status string `json:"status"`
}
