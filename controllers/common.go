package controllers

import (
	"context"
	"errors"
	"github.com/cockroachdb/ccapi-k8s-operator/controllers/crdb"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DefaultRetryDelay = time.Second * 10
)

func CreateClusterServiceClient(selector client.ObjectKey, ctx context.Context, client client.Client) (crdb.ClusterService, error) {
	cred, err := retrieveClientCredential(selector, ctx, client)
	if err != nil {
		return nil, err
	}
	return crdb.NewClusterServiceCloud(cred), nil
}

func retrieveClientCredential(selector client.ObjectKey, ctx context.Context, client client.Client) (*crdb.Credential, error) {
	secret := &corev1.Secret{}
	if err := client.Get(ctx, selector, secret); err != nil {
		return nil, err
	}
	cred := &crdb.Credential{
		OrgID:  string(secret.Data["orgId"]),
		APIKey: string(secret.Data["apiKey"]),
	}
	if cred.OrgID == "" || cred.APIKey == "" {
		return nil, errors.New("failed to retrieve a complete API Credential")
	}

	return cred, nil
}

func updateClusterServiceCloudCredential(service crdb.ClusterService, selector client.ObjectKey,
	ctx context.Context, client client.Client) error {
	cred, err := retrieveClientCredential(selector, ctx, client)
	if err != nil {
		return err
	}
	service.SetCredential(cred)
	return nil
}
