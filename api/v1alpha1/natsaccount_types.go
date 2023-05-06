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
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NATS Account import, duplicated here to have codegen

// NATS Account export, duplicated here to have codegen
type Export struct {
	Name                 string              `json:"name,omitempty"`
	Subject              jwt.Subject         `json:"subject,omitempty"`
	Type                 jwt.ExportType      `json:"type,omitempty"`
	TokenReq             bool                `json:"token_req,omitempty"`
	Revocations          jwt.RevocationList  `json:"revocations,omitempty"`
	ResponseType         jwt.ResponseType    `json:"response_type,omitempty"`
	ResponseThreshold    time.Duration       `json:"response_threshold,omitempty"`
	Latency              *jwt.ServiceLatency `json:"service_latency,omitempty"`
	AccountTokenPosition uint                `json:"account_token_position,omitempty"`
	Advertise            bool                `json:"advertise,omitempty"`
	jwt.Info             `json:",inline"`
}

// OperatorLimits are used to limit access by an account
type OperatorLimits struct {
	jwt.NatsLimits            `json:",inline"`
	jwt.AccountLimits         `json:",inline"`
	jwt.JetStreamLimits       `json:",inline"`
	jwt.JetStreamTieredLimits `json:"tiered_limits,omitempty"`
}

// NatsAccountSpec defines the desired state of NatsAccount
type NatsAccountSpec struct {
	// OperatorRef contains the NATS operator that should issue this account.
	OperatorRef corev1.ObjectReference `json:"operatorRef,omitempty"`
	// Namespaces that are allowed for user creation.
	// If a NatsUser is referencing this account outside of these namespaces, the operator will create an event for it saying that it's not allowed.
	AllowUserNamespaces []string `json:"allowedUserNamespaces,omitempty"`

	// These fields are directly mappejwtd into the NATS JWT claim
	Imports     []*jwt.Import      `json:"imports,omitempty"`
	Exports     []Export           `json:"exports,omitempty"`
	Limits      OperatorLimits     `json:"limits,omitempty"`
	Revocations jwt.RevocationList `json:"revocations,omitempty"`

	// FIXME: Scoped signing keys
}

func (s NatsAccountSpec) ToJWTAccount() jwt.Account {
	exports := lo.Map(s.Exports, func(e Export, _ int) *jwt.Export {
		return &jwt.Export{
			Name:                 e.Name,
			Subject:              e.Subject,
			Type:                 e.Type,
			TokenReq:             e.TokenReq,
			Revocations:          e.Revocations,
			ResponseType:         e.ResponseType,
			ResponseThreshold:    e.ResponseThreshold,
			Latency:              e.Latency,
			AccountTokenPosition: e.AccountTokenPosition,
			Advertise:            e.Advertise,
			Info:                 e.Info,
		}
	})
	return jwt.Account{
		Imports: jwt.Imports(s.Imports),
		Exports: jwt.Exports(exports),
		Limits: jwt.OperatorLimits{
			NatsLimits:            s.Limits.NatsLimits,
			AccountLimits:         s.Limits.AccountLimits,
			JetStreamLimits:       s.Limits.JetStreamLimits,
			JetStreamTieredLimits: s.Limits.JetStreamTieredLimits,
		},
		// FIXME: scoped signing keys
		SigningKeys: jwt.SigningKeys{},
		Revocations: s.Revocations,
	}
}

// NatsAccountStatus defines the observed state of NatsAccount
type NatsAccountStatus struct {
	AccountSecretName string `json:"accountSecretName,omitempty"`
	PublicKey         string `json:"publicKey,omitempty"`
	JWT               string `json:"jwt,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// NatsAccount is the Schema for the natsaccounts API
type NatsAccount struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NatsAccountSpec   `json:"spec,omitempty"`
	Status NatsAccountStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NatsAccountList contains a list of NatsAccount
type NatsAccountList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NatsAccount `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NatsAccount{}, &NatsAccountList{})
}
