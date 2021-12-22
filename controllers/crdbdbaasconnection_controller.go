/*
Copyright 2021.

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
	"errors"
	"fmt"
	dbaasv1alpha1 "github.com/RHEcosystemAppEng/dbaas-operator/api/v1alpha1"
	"github.com/cockroachdb/ccapi-k8s-operator/api/v1alpha1"
	"github.com/cockroachdb/ccapi-k8s-operator/controllers/crdb"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// CrdbDBaaSConnectionReconciler reconciles a CrdbDBaaSConnection object
type CrdbDBaaSConnectionReconciler struct {
	client.Client
	Scheme              *runtime.Scheme
	ClusterServiceCloud crdb.ClusterService
}

const (
	ConnectionConditionReadyType         string = "ReadyForBinding"
	ConnectionConditionNotReadyType      string = "NotReady"
	ConnectionConditionNotReadyReason    string = "InventoryNotReady"
	ConnectionConditionNotReadyReasonMsg string = "Wait for Inventory"
)

//+kubebuilder:rbac:groups=dbaas.redhat.com,resources=crdbdbaasconnections,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=dbaas.redhat.com,resources=crdbdbaasconnections/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dbaas.redhat.com,resources=crdbdbaasconnections/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;delete

func (r *CrdbDBaaSConnectionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx, "reconcile", "CockroachDB CrdbDBaaSConnectionReconciler Reconciler")

	var connection v1alpha1.CrdbDBaaSConnection
	if err := r.Get(ctx, req.NamespacedName, &connection); err != nil {
		if apierrors.IsNotFound(err) {
			// CR deleted since request queued, child objects getting GC'd, no requeue
			log.Info("CrdbDBaaSConnection resource not found, or has been deleted")
			return ctrl.Result{Requeue: true}, nil
		}
		log.Error(err, "Error fetching CrdbDBaaSConnection for reconcile")
		return ctrl.Result{}, err
	}

	for _, cond := range connection.Status.Conditions {
		//skip the connection that has been synced already
		if cond.Type == ConnectionConditionReadyType && cond.Status == metav1.ConditionTrue {
			log.Info("Connection Status updated already")
			return ctrl.Result{}, nil
		}
	}

	inventory := v1alpha1.CrdbDBaaSInventory{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: connection.Spec.InventoryRef.Namespace, Name: connection.Spec.InventoryRef.Name}, &inventory); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("CrdbDBaaSInventory resource not found, may have been deleted")
			return ctrl.Result{RequeueAfter: DefaultRetryDelay}, err
		}
		log.Error(err, "Failed to fetch CrdbDBaaSInventory")
		return ctrl.Result{}, err
	}

	log.Info("check if inventory is with a valid cluster")
	instance, err := getClusterInstance(inventory, connection.Spec.InstanceID)
	if err != nil {
		if statusErr := r.updateStatus(ctx, connection, metav1.ConditionFalse, ConnectionConditionNotReadyReason, ConnectionConditionNotReadyReasonMsg); statusErr != nil {
			log.Error(statusErr, "Failed to update CrdbDBaasConnection status")
			return ctrl.Result{Requeue: true}, statusErr
		}
		return ctrl.Result{}, err
	}

	log.Info("Creating API client for cockroachdb cloud")
	secretSelector := client.ObjectKey{
		Namespace: inventory.Spec.CredentialsRef.Namespace,
		Name:      inventory.Spec.CredentialsRef.Name,
	}
	if r.ClusterServiceCloud == nil {
		var err error
		r.ClusterServiceCloud, err = CreateClusterServiceClient(secretSelector, ctx, r.Client)
		if err != nil {
			log.Error(err, "Failed to create ClusterServiceCloud")
			return ctrl.Result{RequeueAfter: DefaultRetryDelay}, nil
		}
		log.Info("Created ClusterService for crdb cloud")
	} else {
		if err := updateClusterServiceCloudCredential(r.ClusterServiceCloud, secretSelector, ctx, r.Client); err != nil {
			log.Error(err, "Failed to update ClusterServiceCloud API credential")
		}
		log.Info("Using the ClusterService in cache, but with updated API credential")
	}

	log.Info("create a new sql user in cockroachDB cloud for this connection")
	sqlUser, _, errs := r.ClusterServiceCloud.CreateUser(connection.Spec.InstanceID)
	if errs != nil {
		log.Error(errs, "failed to create a sql user at cockroachDB cloud")
		return ctrl.Result{RequeueAfter: DefaultRetryDelay}, nil
	}

	log.Info("download cluster CA certificate from cockroachDB Cloud")
	caCert, _, errs := r.ClusterServiceCloud.GetClusterCertificate(connection.Spec.InstanceID)
	if errs != nil {
		log.Error(errs, "failed to download cluster CA certificate from cockroachDB cloud")
		return ctrl.Result{RequeueAfter: DefaultRetryDelay}, nil
	}

	log.Info("save this sql user's secret and cluster ca-cert in k8s")
	userSecret := buildSecret(&connection, sqlUser, caCert)
	if err = r.Create(ctx, userSecret); err != nil {
		log.Error(err, "Failed to create secret object for the sql user")
		removeSqlUser(log, r.ClusterServiceCloud, connection.Spec.InstanceID, sqlUser)
		return ctrl.Result{RequeueAfter: DefaultRetryDelay}, nil
	}

	log.Info("save this instance's connection info in a configMap")
	dbConfigMap := buildConfigMap(&connection, instance)
	if err = r.Create(ctx, dbConfigMap); err != nil {
		log.Error(err, "Failed to create configmap object for the cluster")
		removeSqlUser(log, r.ClusterServiceCloud, connection.Spec.InstanceID, sqlUser)
		return ctrl.Result{RequeueAfter: DefaultRetryDelay}, nil
	}

	log.Info("update connection status")
	connection.Status.CredentialsRef = &corev1.LocalObjectReference{Name: userSecret.Name}
	connection.Status.ConnectionInfoRef = &corev1.LocalObjectReference{Name: dbConfigMap.Name}
	if err := r.updateStatus(ctx, connection, metav1.ConditionTrue, "Ready", "Connection is ready"); err != nil {
		log.Error(err, "Failed to update connection status")
		return ctrl.Result{Requeue: true}, nil
	}

	return ctrl.Result{}, nil
}

func removeSqlUser(log logr.Logger, apiClient crdb.ClusterService, clusterID string, sqlUser *crdb.SqlUser) {
	_, errs := apiClient.DeleteUser(clusterID, sqlUser.User.Name)
	if errs != nil {
		log.Error(errs, fmt.Sprintf("Failed to clean the sql user in cockroachDB cloud %v", sqlUser.User.Name))
	}
}

func (r *CrdbDBaaSConnectionReconciler) updateStatus(ctx context.Context, conn v1alpha1.CrdbDBaaSConnection,
	status metav1.ConditionStatus, reason, msg string) error {
	statusType := ConnectionConditionNotReadyType
	if status == metav1.ConditionTrue {
		statusType = ConnectionConditionReadyType
	}
	curCondition := metav1.Condition{
		Type:    statusType,
		Status:  status,
		Reason:  reason,
		Message: msg}

	apimeta.SetStatusCondition(&conn.Status.Conditions, curCondition)
	if err := r.Status().Update(ctx, &conn); err != nil {
		return err
	}
	return nil
}

func getClusterInstance(inventory v1alpha1.CrdbDBaaSInventory, instanceID string) (*dbaasv1alpha1.Instance, error) {
	var conSynced *metav1.Condition
	for i := range inventory.Status.Conditions {
		if inventory.Status.Conditions[i].Type == "SpecSynced" && inventory.Status.Conditions[i].Status == metav1.ConditionTrue {
			conSynced = &inventory.Status.Conditions[i]
			break
		}
	}
	if conSynced == nil {
		return nil, errors.New("CrdbDBaaSInventory is not yet in-synced, or is invalid")
	}
	for _, instance := range inventory.Status.Instances {
		if instance.InstanceID == instanceID {
			return &instance, nil
		}
	}
	return nil, fmt.Errorf("instance with id:%v not found in CrdbDBaaSInventory", instanceID)
}

func buildConfigMap(connection *v1alpha1.CrdbDBaaSConnection, instance *dbaasv1alpha1.Instance) *corev1.ConfigMap {
	//TODO how to know the sslrootcert path??
	dbOptions := fmt.Sprintf("sslmode=verify-full&sslrootcert=$HOME/.postgresql/root.crt&options=--cluster=%v",
		instance.InstanceInfo["serverless.tenantName"])

	dataMap := map[string]string{
		"type":     "postgresql",
		"provider": "CockroachDB Cloud",
		"host":     instance.InstanceInfo["regions.1.sqlDns"],
		"port":     "26257",
		"database": "defaultdb",
		"options":  dbOptions,
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "crdb-cloud-conn-cm-",
			Namespace:    connection.Namespace,
			Labels: map[string]string{
				"managed-by":      "ccapi-k8s-operator",
				"owner":           connection.Name,
				"owner.kind":      connection.Kind,
				"owner.namespace": connection.Namespace,
			},
			OwnerReferences: []metav1.OwnerReference{{
				UID:                connection.GetUID(),
				APIVersion:         connection.APIVersion,
				BlockOwnerDeletion: pointer.BoolPtr(false),
				Controller:         pointer.BoolPtr(true),
				Kind:               connection.Kind,
				Name:               connection.Name,
			}},
		},
		Data: dataMap,
	}
	cm.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
	return cm
}

func buildSecret(connection *v1alpha1.CrdbDBaaSConnection, sqlUser *crdb.SqlUser, caCert *crdb.ClusterCertificate) *corev1.Secret {
	secret := &corev1.Secret{
		Type: corev1.SecretTypeOpaque,
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "crdb-cloud-user-credentials-",
			Namespace:    connection.Namespace,
			Labels: map[string]string{
				"managed-by":      "ccapi-k8s-operator",
				"owner":           connection.Name,
				"owner.kind":      connection.Kind,
				"owner.namespace": connection.Namespace,
			},
			OwnerReferences: []metav1.OwnerReference{{
				UID:                connection.GetUID(),
				APIVersion:         connection.APIVersion,
				BlockOwnerDeletion: pointer.BoolPtr(false),
				Controller:         pointer.BoolPtr(true),
				Kind:               connection.Kind,
				Name:               connection.Name,
			}},
		},
		Data: map[string][]byte{
			"username":      []byte(sqlUser.User.Name),
			"password":      []byte(sqlUser.Password),
			caCert.FileName: []byte(caCert.CaCertificate),
		},
	}
	secret.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Secret"))
	return secret
}

// SetupWithManager sets up the controller with the Manager.
func (r *CrdbDBaaSConnectionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.CrdbDBaaSConnection{}).
		Complete(r)
}
