//go:build !ignore_autogenerated

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

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AdditionalUser) DeepCopyInto(out *AdditionalUser) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AdditionalUser.
func (in *AdditionalUser) DeepCopy() *AdditionalUser {
	if in == nil {
		return nil
	}
	out := new(AdditionalUser)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseConnectionReference) DeepCopyInto(out *DatabaseConnectionReference) {
	*out = *in
	out.DatabaseObjectReference = in.DatabaseObjectReference
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseConnectionReference.
func (in *DatabaseConnectionReference) DeepCopy() *DatabaseConnectionReference {
	if in == nil {
		return nil
	}
	out := new(DatabaseConnectionReference)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseInfo) DeepCopyInto(out *DatabaseInfo) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseInfo.
func (in *DatabaseInfo) DeepCopy() *DatabaseInfo {
	if in == nil {
		return nil
	}
	out := new(DatabaseInfo)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseMySQLProvider) DeepCopyInto(out *DatabaseMySQLProvider) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseMySQLProvider.
func (in *DatabaseMySQLProvider) DeepCopy() *DatabaseMySQLProvider {
	if in == nil {
		return nil
	}
	out := new(DatabaseMySQLProvider)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DatabaseMySQLProvider) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseMySQLProviderList) DeepCopyInto(out *DatabaseMySQLProviderList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]DatabaseMySQLProvider, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseMySQLProviderList.
func (in *DatabaseMySQLProviderList) DeepCopy() *DatabaseMySQLProviderList {
	if in == nil {
		return nil
	}
	out := new(DatabaseMySQLProviderList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DatabaseMySQLProviderList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseMySQLProviderSpec) DeepCopyInto(out *DatabaseMySQLProviderSpec) {
	*out = *in
	if in.MySQLConnections != nil {
		in, out := &in.MySQLConnections, &out.MySQLConnections
		*out = make([]MySQLConnection, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseMySQLProviderSpec.
func (in *DatabaseMySQLProviderSpec) DeepCopy() *DatabaseMySQLProviderSpec {
	if in == nil {
		return nil
	}
	out := new(DatabaseMySQLProviderSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseMySQLProviderStatus) DeepCopyInto(out *DatabaseMySQLProviderStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]v1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.MySQLConnectionStatus != nil {
		in, out := &in.MySQLConnectionStatus, &out.MySQLConnectionStatus
		*out = make([]MySQLConnectionStatus, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseMySQLProviderStatus.
func (in *DatabaseMySQLProviderStatus) DeepCopy() *DatabaseMySQLProviderStatus {
	if in == nil {
		return nil
	}
	out := new(DatabaseMySQLProviderStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseRequest) DeepCopyInto(out *DatabaseRequest) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseRequest.
func (in *DatabaseRequest) DeepCopy() *DatabaseRequest {
	if in == nil {
		return nil
	}
	out := new(DatabaseRequest)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DatabaseRequest) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseRequestList) DeepCopyInto(out *DatabaseRequestList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]DatabaseRequest, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseRequestList.
func (in *DatabaseRequestList) DeepCopy() *DatabaseRequestList {
	if in == nil {
		return nil
	}
	out := new(DatabaseRequestList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DatabaseRequestList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseRequestSpec) DeepCopyInto(out *DatabaseRequestSpec) {
	*out = *in
	if in.Seed != nil {
		in, out := &in.Seed, &out.Seed
		*out = new(corev1.SecretReference)
		**out = **in
	}
	if in.AdditionalUsers != nil {
		in, out := &in.AdditionalUsers, &out.AdditionalUsers
		*out = make([]AdditionalUser, len(*in))
		copy(*out, *in)
	}
	if in.DatabaseConnectionReference != nil {
		in, out := &in.DatabaseConnectionReference, &out.DatabaseConnectionReference
		*out = new(DatabaseConnectionReference)
		**out = **in
	}
	if in.ForcedReconcilation != nil {
		in, out := &in.ForcedReconcilation, &out.ForcedReconcilation
		*out = (*in).DeepCopy()
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseRequestSpec.
func (in *DatabaseRequestSpec) DeepCopy() *DatabaseRequestSpec {
	if in == nil {
		return nil
	}
	out := new(DatabaseRequestSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseRequestStatus) DeepCopyInto(out *DatabaseRequestStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]v1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.ObservedDatabaseConnectionReference != nil {
		in, out := &in.ObservedDatabaseConnectionReference, &out.ObservedDatabaseConnectionReference
		*out = new(DatabaseConnectionReference)
		**out = **in
	}
	if in.DatabaseInfo != nil {
		in, out := &in.DatabaseInfo, &out.DatabaseInfo
		*out = new(DatabaseInfo)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseRequestStatus.
func (in *DatabaseRequestStatus) DeepCopy() *DatabaseRequestStatus {
	if in == nil {
		return nil
	}
	out := new(DatabaseRequestStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MySQLConnection) DeepCopyInto(out *MySQLConnection) {
	*out = *in
	if in.ReplicaHostnames != nil {
		in, out := &in.ReplicaHostnames, &out.ReplicaHostnames
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	out.PasswordSecretRef = in.PasswordSecretRef
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MySQLConnection.
func (in *MySQLConnection) DeepCopy() *MySQLConnection {
	if in == nil {
		return nil
	}
	out := new(MySQLConnection)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MySQLConnectionStatus) DeepCopyInto(out *MySQLConnectionStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MySQLConnectionStatus.
func (in *MySQLConnectionStatus) DeepCopy() *MySQLConnectionStatus {
	if in == nil {
		return nil
	}
	out := new(MySQLConnectionStatus)
	in.DeepCopyInto(out)
	return out
}
