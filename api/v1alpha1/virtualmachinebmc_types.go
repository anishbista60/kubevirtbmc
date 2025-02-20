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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NamespacedName contains the namespace and name of the depicted resource
type NamespacedName struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

// VirtualMachineBMCSpec defines the desired state of VirtualMachineBMC
type VirtualMachineBMCSpec struct {
	// A Secret object reference for the IPMI/Redfish authentication.
	// It should contain a pair of username and password.
	AuthSecret NamespacedName `json:"authSecret"`

	// The VM this emulated BMC manages
	VirtualMachine NamespacedName `json:"virtualMachine"`
}

// Condition represents the state of the VirtualMachineBMC at a certain point
type Condition struct {
	// Type of condition
	Type string `json:"type"`

	// Status of the condition, one of True or False
	Status string `json:"status"`

	// Last time the condition was updated
	LastUpdateTime metav1.Time `json:"lastUpdateTime"`
}

// VirtualMachineBMCStatus defines the observed state of VirtualMachineBMC
type VirtualMachineBMCStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// The listen IP address for the IPMI service.
	ServiceIP string `json:"serviceIP"`

	Ready bool `json:"ready"`

	// Conditions represent the latest available observations of an object's state
	Conditions []Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// VirtualMachineBMC is the Schema for the virtualmachinebmcs API
type VirtualMachineBMC struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineBMCSpec   `json:"spec,omitempty"`
	Status VirtualMachineBMCStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// VirtualMachineBMCList contains a list of VirtualMachineBMC
type VirtualMachineBMCList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualMachineBMC `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VirtualMachineBMC{}, &VirtualMachineBMCList{})
}
