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

package controllers

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	natsv1alpha1 "github.com/deinstapel/nats-jwt-operator/api/v1alpha1"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	corev1 "k8s.io/api/core/v1"
)

// NatsOperatorReconciler reconciles a NatsOperator object
type NatsOperatorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const JWT_OPERATOR_FINALIZER = "nats.deinstapel.de/jwt-operator"
const OPERATOR_SEED_KEY = "seed.nk"
const OPERATOR_PUBLIC_KEY = "key.pub"
const OPERATOR_JWT = "key.jwt"
const OPERATOR_CREDS = "user.creds"

//+kubebuilder:rbac:groups=nats.deinstapel.de,resources=natsoperators,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nats.deinstapel.de,resources=natsoperators/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nats.deinstapel.de,resources=natsoperators/finalizers,verbs=update
//+kubebuilder:rbac:groups=,resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *NatsOperatorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	operator := &natsv1alpha1.NatsOperator{}
	if err := r.Get(ctx, req.NamespacedName, operator); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if operator.DeletionTimestamp != nil {
		// TODO: Check if deletion is ok.
		logger.Info("Processing deletion of operator")
		if controllerutil.RemoveFinalizer(operator, JWT_OPERATOR_FINALIZER) {
			if err := r.Update(ctx, operator); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if controllerutil.AddFinalizer(operator, JWT_OPERATOR_FINALIZER) {
		if err := r.Update(ctx, operator); err != nil {
			return ctrl.Result{}, err
		}
	}
	_, err := r.reconcileSecret(ctx, req, operator)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Create / reconcile system account
	systemAccount := &natsv1alpha1.NatsAccount{}
	systemAccountName := client.ObjectKey{
		Namespace: req.Namespace,
		Name:      fmt.Sprintf("%v-system", req.Name),
	}
	for {
		if err := r.Get(ctx, systemAccountName, systemAccount); errors.IsNotFound(err) {
			logger.Info("creating system account")
			systemAccount.Name = systemAccountName.Name
			systemAccount.Namespace = systemAccountName.Namespace
			if err := controllerutil.SetOwnerReference(operator, systemAccount, r.Scheme); err != nil {
				return ctrl.Result{}, err
			}

			systemAccount.Spec = natsv1alpha1.NatsAccountSpec{
				AllowUserNamespaces: []string{req.Namespace},
				OperatorRef: corev1.ObjectReference{
					Namespace: req.Namespace,
					Name:      req.Name,
				},
			}
			if err := r.Create(ctx, systemAccount); err != nil {
				return ctrl.Result{}, err
			}

			// After the user has been created, we need to requeue this operator nkey
			// because we need to enqueue the account and the user in order to create the server config
			return ctrl.Result{
				RequeueAfter: 5 * time.Second,
			}, nil
		} else if err != nil {
			return ctrl.Result{}, err
		} else if systemAccount.Status.JWT == "" {
			// Object has been found, but JWT hasn't been issued, wait until it has been issued
			logger.Info("waiting for system account to become ready")
			<-time.After(5 * time.Second)
		} else {
			break
		}
	}

	// Create / reconcile system JWT user
	systemUser := &natsv1alpha1.NatsUser{}
	systemUserName := client.ObjectKey{
		Namespace: req.Namespace,
		Name:      fmt.Sprintf("%v-jwt", req.Name),
	}
	for {
		if err := r.Get(ctx, systemUserName, systemUser); errors.IsNotFound(err) {
			logger.Info("creating jwt system user")
			systemUser.Name = systemUserName.Name
			systemUser.Namespace = systemUserName.Namespace
			if err := controllerutil.SetOwnerReference(operator, systemUser, r.Scheme); err != nil {
				return ctrl.Result{}, err
			}
			// Allow this user to publish and subscribe, i.e. interact with the server for JWT permissions
			systemUser.Spec = natsv1alpha1.NatsUserSpec{
				AccountRef: corev1.ObjectReference{
					Namespace: systemAccount.Namespace,
					Name:      systemAccount.Name,
				},
				Permissions: natsv1alpha1.Permissions{
					Pub: natsv1alpha1.Permission{
						Allow: []string{"$SYS.REQ.ACCOUNT.*.CLAIMS.UPDATE", "$SYS.REQ.CLAIMS.>"},
					},
					Sub: natsv1alpha1.Permission{
						Allow: []string{"$SYS.REQ.ACCOUNT.*.CLAIMS.LOOKUP", "$SYS.REQ.CLAIMS.>"},
					},
					Resp: &jwt.ResponsePermission{
						MaxMsgs: 1,
						Expires: -1,
					},
				},
			}
			if err := r.Create(ctx, systemUser); err != nil {
				return ctrl.Result{}, err
			}

			// After the user has been created, we need to requeue this operator nkey
			// because we need to enqueue the account and the user in order to create the server config
			return ctrl.Result{
				RequeueAfter: 5 * time.Second,
			}, nil
		} else if err != nil {
			return ctrl.Result{}, err
		} else if systemUser.Status.JWT == "" {
			// Object has been found but jwt not issued yet
			logger.Info("waiting for system user to become ready")
			<-time.After(5 * time.Second)
		} else {
			break
		}
	}

	// Finally, reconcile server configuration snippet
	serverConfig := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: req.Namespace,
		Name:      fmt.Sprintf("%v-jwt", req.Name),
	}, serverConfig); errors.IsNotFound(err) {
		logger.Info("creating server config")
		// TODO: create server config file, issue operator JWT
	} else if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *NatsOperatorReconciler) reconcileSecret(ctx context.Context, req ctrl.Request, operator *natsv1alpha1.NatsOperator) (*corev1.Secret, error) {
	// Try reconcile the secret containing the seed key for the operator
	logger := log.FromContext(ctx)
	operatorKeySecret := &corev1.Secret{}
	hasSecret := true
	hasChanges := false
	if err := r.Get(ctx, req.NamespacedName, operatorKeySecret); errors.IsNotFound(err) {
		operatorKeySecret.Namespace = req.Namespace
		operatorKeySecret.Name = req.Name
		operatorKeySecret.Type = "deinstapel.de/nats-operator"
		hasSecret = false
		if err := controllerutil.SetOwnerReference(operator, operatorKeySecret, r.Scheme); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	logger.Info("reconciling operator keys")
	hasChanges, err := r.reconcileKey(ctx, operatorKeySecret, operator)
	if err != nil {
		return nil, err
	}

	if !hasSecret {
		if err := r.Create(ctx, operatorKeySecret); err != nil {
			return nil, err
		}
	} else if hasChanges {
		if err := r.Update(ctx, operatorKeySecret); err != nil {
			return nil, err
		}
	}

	if !hasSecret || hasChanges {
		// Update operator status if we encountered changes
		operator.Status.OperatorSecretName = operatorKeySecret.Name
		operator.Status.PublicKey = string(operatorKeySecret.Data[OPERATOR_PUBLIC_KEY])
		operator.Status.JWT = string(operatorKeySecret.Data[OPERATOR_JWT])
		if err := r.Status().Update(ctx, operator); err != nil {
			return nil, err
		}
	}
	return operatorKeySecret, nil
}

func (r *NatsOperatorReconciler) reconcileKey(ctx context.Context, secret *corev1.Secret, operator *natsv1alpha1.NatsOperator) (bool, error) {
	logger := log.FromContext(ctx)
	keys, needsKeyUpdate, err := extractOrCreateKeys(secret, nkeys.CreateOperator)
	if err != nil {
		return false, err
	}

	seed, _ := keys.Seed()
	public, _ := keys.PublicKey()

	token := jwt.NewOperatorClaims(public)
	token.Operator.SigningKeys = operator.Spec.SigningKeys
	needsClaimsUpdate := secret.Data == nil

	if secret.Data != nil {
		oldToken, err := jwt.DecodeOperatorClaims(string(secret.Data[OPERATOR_JWT]))
		if err == nil {
			needsClaimsUpdate = needsClaimsUpdate || !reflect.DeepEqual(token.Operator.SigningKeys, oldToken.Operator.SigningKeys)
		} else {
			// Claims could not be decoded, need update.
			needsClaimsUpdate = true
		}
	}

	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}
	logger.Info("updating secret if needed", "claimsChanged", needsClaimsUpdate)

	if needsKeyUpdate {
		secret.Data[OPERATOR_SEED_KEY] = seed
		secret.Data[OPERATOR_PUBLIC_KEY] = []byte(public)
	}
	if needsKeyUpdate || needsClaimsUpdate {
		// Whenerver our keys changed, we also need to force renew the token
		jwt, err := token.Encode(keys)
		if err != nil {
			return false, err
		}
		secret.Data[OPERATOR_JWT] = []byte(jwt)
	}
	return needsKeyUpdate || needsClaimsUpdate, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NatsOperatorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&natsv1alpha1.NatsOperator{}).
		Complete(r)
}
