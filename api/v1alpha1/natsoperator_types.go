/*
Copyright 2023.

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
	"github.com/nats-io/jwt/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NatsOperatorSpec defines the operator NKey for a single NATS cluster or server.
// It will generate several things:
// 1. A NKey key pair for the nats operator, stored in a secret
// 2. A NatsAccount object for the system account, named $NAME-system
// 3. A NatsUser object for the system user, named $NAME-jwt-operator
// 4. A Secret containing a configuration file to include for the nats server
// 4.1. We're using a secret here, because the secret file will be updated while the pod is running, which would
//      allow us to use the config reloader that's coming with the nats helm chart.

type NatsOperatorSpec struct {
	// SigningKeys is a Slice of other operator NKeys that can be used to sign on behalf of the main
	// operator identity.
	SigningKeys jwt.StringList `json:"signing_keys,omitempty"`

	// ServerURLs will be used further on down the line to connect to the NATS server to push account information using the system user.
	ServerURLs jwt.StringList `json:"serverUrls,omitempty"`
}

// NatsOperatorStatus defines the observed state of NatsOperator
type NatsOperatorStatus struct {
	// OperatorSecretName contains the name of the secret where the seed keys for the operator key pair are stored
	OperatorSecretName string `json:"operatorSecretName,omitempty"`

	// PublicKey is the root public key used to sign all other accounts
	PublicKey string `json:"publicKey,omitempty"`
	JWT       string `json:"jwt,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// NatsOperator is the Schema for the natsoperators API
type NatsOperator struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NatsOperatorSpec   `json:"spec,omitempty"`
	Status NatsOperatorStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NatsOperatorList contains a list of NatsOperator
type NatsOperatorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NatsOperator `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NatsOperator{}, &NatsOperatorList{})
}
