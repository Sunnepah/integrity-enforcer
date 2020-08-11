//
// Copyright 2020 IBM Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package v1alpha1

import (
	policy "github.com/IBM/integrity-enforcer/enforcer/pkg/policy"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IESignerPolicySpec defines the desired state of IESignerPolicy
type IESignerPolicySpec struct {
	IESignerPolicy *policy.IESignerPolicy `json:"policy,omitempty"`
}

// IESignerPolicyStatus defines the observed state of IESignerPolicy
type IESignerPolicyStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +resource:path=iesignerpolicy,scope=Namespaced

// EnforcePolicy is the CRD. Use this command to generate deepcopy for it:
// ./k8s.io/code-generator/generate-groups.sh all github.com/IBM/pas-client-go/pkg/crd/packageadmissionsignature/v1/apis github.com/IBM/pas-client-go/pkg/crd/ "packageadmissionsignature:v1"
// For more details of code-generator, please visit https://github.com/kubernetes/code-generator
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// EnforcePolicy is the CRD. Use this command to generate deepcopy for it:
type IESignerPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IESignerPolicySpec   `json:"spec,omitempty"`
	Status IESignerPolicyStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// IESignerPolicyList contains a list of EnforcePolicy
type IESignerPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IESignerPolicy `json:"items"`
}
