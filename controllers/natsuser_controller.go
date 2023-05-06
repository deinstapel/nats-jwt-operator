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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/strings/slices"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	natsv1alpha1 "github.com/deinstapel/nats-jwt-operator/api/v1alpha1"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

// NatsUserReconciler reconciles a NatsUser object
type NatsUserReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=nats.deinstapel.de,resources=natsusers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nats.deinstapel.de,resources=natsusers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nats.deinstapel.de,resources=natsusers/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NatsUser object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *NatsUserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	user := &natsv1alpha1.NatsUser{}
	if err := r.Get(ctx, req.NamespacedName, user); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if user.DeletionTimestamp != nil {
		// TODO: Check if deletion is ok.
		logger.Info("Processing deletion of user")
		if controllerutil.RemoveFinalizer(user, JWT_OPERATOR_FINALIZER) {
			if err := r.Update(ctx, user); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if controllerutil.AddFinalizer(user, JWT_OPERATOR_FINALIZER) {
		if err := r.Update(ctx, user); err != nil {
			return ctrl.Result{}, err
		}
	}

	issuingAccount := &natsv1alpha1.NatsAccount{}
	signerSecret := &corev1.Secret{}
	for {
		if err := r.Get(ctx, client.ObjectKey{
			Namespace: user.Spec.AccountRef.Namespace,
			Name:      user.Spec.AccountRef.Name,
		}, issuingAccount); err != nil {
			// TODO: post event to apiserver
			return ctrl.Result{}, err
		}
		if issuingAccount.Status.AccountSecretName == "" {
			logger.Info("waiting for issuing account secret to appear")
			<-time.After(5 * time.Second)
			continue
		}

		if !slices.Contains(issuingAccount.Spec.AllowUserNamespaces, req.Namespace) {
			// TODO: post event to apiserver
			return ctrl.Result{}, nil
		}

		if err := r.Get(ctx, client.ObjectKey{
			Namespace: issuingAccount.Namespace,
			Name:      issuingAccount.Status.AccountSecretName,
		}, signerSecret); err != nil {
			return ctrl.Result{}, err
		}
		break
	}

	_, err := r.reconcileSecret(ctx, req, user, signerSecret)
	return ctrl.Result{}, err
}
func (r *NatsUserReconciler) reconcileSecret(ctx context.Context, req ctrl.Request, user *natsv1alpha1.NatsUser, signerSecret *corev1.Secret) (*corev1.Secret, error) {
	// Try reconcile the secret containing the seed key for the operator
	logger := log.FromContext(ctx)
	keySecret := &corev1.Secret{}
	hasSecret := true
	hasChanges := false
	if err := r.Get(ctx, req.NamespacedName, keySecret); errors.IsNotFound(err) {
		keySecret.Namespace = req.Namespace
		keySecret.Name = req.Name
		keySecret.Type = "deinstapel.de/nats-user"
		hasSecret = false
		if err := controllerutil.SetOwnerReference(user, keySecret, r.Scheme); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	logger.Info("reconciling user keys")
	hasChanges, err := r.reconcileKey(ctx, keySecret, user, signerSecret.Data[OPERATOR_SEED_KEY])
	if err != nil {
		return nil, err
	}

	if !hasSecret {
		if err := r.Create(ctx, keySecret); err != nil {
			return nil, err
		}
	} else if hasChanges {
		if err := r.Update(ctx, keySecret); err != nil {
			return nil, err
		}
	}

	if !hasSecret || hasChanges {
		// Update operator status if we encountered changes
		user.Status.UserSecretName = keySecret.Name
		user.Status.PublicKey = string(keySecret.Data[OPERATOR_PUBLIC_KEY])
		user.Status.JWT = string(keySecret.Data[OPERATOR_JWT])
		if err := r.Status().Update(ctx, user); err != nil {
			return nil, err
		}
	}
	return keySecret, nil
}

func (r *NatsUserReconciler) reconcileKey(ctx context.Context, secret *corev1.Secret, account *natsv1alpha1.NatsUser, signer []byte) (bool, error) {
	logger := log.FromContext(ctx)
	keys, needsKeyUpdate, err := extractOrCreateKeys(secret, nkeys.CreateAccount)
	if err != nil {
		return false, err
	}

	seed, _ := keys.Seed()
	public, _ := keys.PublicKey()

	token := jwt.NewUserClaims(public)
	token.User = account.Spec.ToNatsJWT()
	needsClaimsUpdate := secret.Data == nil
	signerKp, err := nkeys.FromSeed(signer)
	if err != nil {
		return false, fmt.Errorf("failed decoding seed: %v, signer: %v", err, signer)
	}

	if secret.Data != nil {
		oldToken, err := jwt.DecodeUserClaims(string(secret.Data[OPERATOR_JWT]))
		if err == nil {
			needsClaimsUpdate = needsClaimsUpdate || !reflect.DeepEqual(token.User, oldToken.User)
			// Check if the signing keys changed
			needsClaimsUpdate = needsClaimsUpdate || oldToken.Issuer != token.Issuer
		} else {
			// Claims could not be decoded, need update.
			needsClaimsUpdate = true
		}
	}

	logger.Info("updating secret if needed", "needsUpdate", needsClaimsUpdate)

	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}
	if needsKeyUpdate {
		secret.Data[OPERATOR_SEED_KEY] = seed
		secret.Data[OPERATOR_PUBLIC_KEY] = []byte(public)
	}
	if needsKeyUpdate || needsClaimsUpdate {
		jwt, err := token.Encode(signerKp)
		if err != nil {
			return false, err
		}
		secret.Data[OPERATOR_JWT] = []byte(jwt)
	}
	return needsKeyUpdate || needsClaimsUpdate, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NatsUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&natsv1alpha1.NatsUser{}).
		Complete(r)
}
