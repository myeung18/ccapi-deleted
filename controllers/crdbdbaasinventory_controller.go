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
	"fmt"
	dbaasv1alpha1 "github.com/RHEcosystemAppEng/dbaas-operator/api/v1alpha1"
	"github.com/cockroachdb/ccapi-k8s-operator/api/v1alpha1"
	"github.com/cockroachdb/ccapi-k8s-operator/controllers/crdb"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"strconv"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	InventoryConditionReadyType    string = "SpecSynced"
	InventoryConditionReason       string = "SyncOK"
	InventoryConditionReadyMessage string = "Cluster details in sync"
)

// CrdbDBaaSInventoryReconciler reconciles a CrdbDBaaSInventory object
type CrdbDBaaSInventoryReconciler struct {
	client.Client
	Scheme              *runtime.Scheme
	ClusterServiceCloud crdb.ClusterService
}

//+kubebuilder:rbac:groups=dbaas.redhat.com,resources=crdbdbaasinventories,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=dbaas.redhat.com,resources=crdbdbaasinventories/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dbaas.redhat.com,resources=crdbdbaasinventories/finalizers,verbs=update

func (r *CrdbDBaaSInventoryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx, "CockroachDB CrdbDBaaSInventoryReconciler Reconciler", req.NamespacedName)

	var inventory v1alpha1.CrdbDBaaSInventory
	if err := r.Get(ctx, req.NamespacedName, &inventory); err != nil {
		if apierrors.IsNotFound(err) {
			// CR deleted since request queued, child objects getting GC'd, no requeue
			log.Info("CrdbDBaaSInventory resource not found, may have been deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to fetch CrdbDBaaSInventory for reconcile")
		return ctrl.Result{}, err
	}

	log.Info("create API client for cockroachdb cloud")
	secretSelector := client.ObjectKey{
		Namespace: inventory.Spec.CredentialsRef.Namespace,
		Name:      inventory.Spec.CredentialsRef.Name,
	}
	log.Info("create API client for cockroachdb cloud")
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
			return ctrl.Result{RequeueAfter: DefaultRetryDelay}, nil
		}
		log.Info("Using the existing ClusterServiceCloud, but with updated API credential")
	}

	log.Info("Discovering clusters from cockroachdb cloud")
	instanceLst, err := discoverClusters(r.ClusterServiceCloud)
	if err != nil {
		log.Error(err, "Failed to discover Clusters")
		return ctrl.Result{RequeueAfter: DefaultRetryDelay}, nil
	}

	log.Info("Starting to reconcile inventory object")
	curCondition := metav1.Condition{
		Type:    InventoryConditionReadyType,
		Status:  metav1.ConditionTrue,
		Reason:  InventoryConditionReason,
		Message: InventoryConditionReadyMessage}

	log.Info("Updating inventory condition")
	apimeta.SetStatusCondition(&inventory.Status.Conditions, curCondition)

	log.Info("Updating inventory status")
	inventory.Status.Instances = instanceLst
	if err = r.Status().Update(ctx, &inventory); err != nil {
		log.Error(err, fmt.Sprintf("Could not update inventory status:%v", inventory.Name))
		return ctrl.Result{Requeue: true}, err
	}
	return ctrl.Result{}, nil
}

func discoverClusters(clusterServiceCloud crdb.ClusterService) ([]dbaasv1alpha1.Instance, error) {
	clusters, _, err := clusterServiceCloud.ListClusters()
	if err != nil {
		return nil, err
	}

	var instanceLst []dbaasv1alpha1.Instance
	for _, c := range clusters {

		details, _, err := clusterServiceCloud.GetCluster(c.ID)
		if err != nil {
			return nil, err
		}

		data := make(map[string]string)
		data["serverless.tenantName"] = c.Serverless.TenantName
		for i := range details.Regions {
			key := fmt.Sprintf("regions.%v.name", strconv.Itoa(i+1))
			data[key] = details.Regions[i].Name
			key = fmt.Sprintf("regions.%v.sqlDns", strconv.Itoa(i+1))
			data[key] = details.Regions[i].SqlDns
		}
		data["cockroachVersion"] = c.CockroachVersion
		data["creatorId"] = c.CreatorId
		data["cloudProvider"] = c.CloudProvider
		data["plan"] = c.Plan
		data["state"] = c.State
		data["longRunningOperationStatus"] = c.LongRunningOperationStatus
		data["createAt"] = c.CreatedAt.String()
		data["updateAt"] = c.UpdatedAt.String()

		cur := dbaasv1alpha1.Instance{
			InstanceID:   c.ID,
			Name:         c.Name,
			InstanceInfo: data,
		}
		instanceLst = append(instanceLst, cur)
	}
	return instanceLst, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CrdbDBaaSInventoryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.CrdbDBaaSInventory{}).
		Complete(r)
}
