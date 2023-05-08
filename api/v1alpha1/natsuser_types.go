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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// Copied from nats-io/jwt to get codegen
type Permissions struct {
	Pub  Permission              `json:"pub,omitempty"`
	Sub  Permission              `json:"sub,omitempty"`
	Resp *jwt.ResponsePermission `json:"resp,omitempty"`
}

func (p Permissions) toNats() jwt.Permissions {
	return jwt.Permissions{
		Pub:  p.Pub.toNats(),
		Sub:  p.Sub.toNats(),
		Resp: p.Resp,
	}
}

type Permission struct {
	Allow jwt.StringList `json:"allow,omitempty"`
	Deny  jwt.StringList `json:"deny,omitempty"`
}

func (p Permission) toNats() jwt.Permission {
	return jwt.Permission{
		Allow: p.Allow,
		Deny:  p.Deny,
	}
}

// NatsUserSpec defines the desired state of NatsUser
type NatsUserSpec struct {
	// AccountRef is the reference to the account that should sign this user
	AccountRef             corev1.ObjectReference `json:"accountRef"`
	Permissions            Permissions            `json:"permissions,omitempty"`
	Limits                 Limits                 `json:"limits,omitempty"`
	BearerToken            bool                   `json:"bearer_token,omitempty"`
	AllowedConnectionTypes jwt.StringList         `json:"allowed_connection_types,omitempty"`
}

type UserLimits struct {
	Src    jwt.CIDRList    `json:"src,omitempty"`
	Times  []jwt.TimeRange `json:"times,omitempty"`
	Locale string          `json:"times_location,omitempty"`
}

func (u UserLimits) toNats() jwt.UserLimits {
	return jwt.UserLimits{
		Src:    u.Src,
		Times:  u.Times,
		Locale: u.Locale,
	}
}

type Limits struct {
	UserLimits     `json:",inline"`
	jwt.NatsLimits `json:",inline"`
}

func (l Limits) toNats() jwt.Limits {
	return jwt.Limits{
		UserLimits: l.UserLimits.toNats(),
		NatsLimits: l.NatsLimits,
	}
}
func (s NatsUserSpec) ToNatsJWT() jwt.User {
	return jwt.User{
		UserPermissionLimits: jwt.UserPermissionLimits{
			Permissions:            s.Permissions.toNats(),
			Limits:                 s.Limits.toNats(),
			BearerToken:            s.BearerToken,
			AllowedConnectionTypes: s.AllowedConnectionTypes,
		},
	}
}

// NatsUserStatus defines the observed state of NatsUser
type NatsUserStatus struct {
	UserSecretName string `json:"userSecretName,omitempty"`
	PublicKey      string `json:"publicKey,omitempty"`
	JWT            string `json:"jwt,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// NatsUser is the Schema for the natsusers API
type NatsUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NatsUserSpec   `json:"spec,omitempty"`
	Status NatsUserStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NatsUserList contains a list of NatsUser
type NatsUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NatsUser `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NatsUser{}, &NatsUserList{})
}
