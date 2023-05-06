package controllers

import (
	"github.com/nats-io/nkeys"
	corev1 "k8s.io/api/core/v1"
)

func extractOrCreateKeys(secret *corev1.Secret, generator func() (nkeys.KeyPair, error)) (nkeys.KeyPair, bool, error) {
	var keys nkeys.KeyPair
	needsKeyUpdate := true
	if secret.Data != nil {
		parsedKeys, err := nkeys.FromSeed(secret.Data[OPERATOR_SEED_KEY])
		if err == nil {
			keys = parsedKeys
			needsKeyUpdate = false
		}
	}
	if keys == nil {
		// No keys present or failed to extract, create new key pair
		createdKeys, err := generator()
		if err != nil {
			return nil, false, err
		}
		keys = createdKeys
	}
	return keys, needsKeyUpdate, nil
}
