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
	v1 "k8s.io/api/apps/v1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// DBaaSProviderReconciler reconciles a DBaaSProvider object
type DBaaSProviderReconciler struct {
	client.Client
	Scheme                   *runtime.Scheme
	operatorInstallNamespace string
	operatorNameVersion      string
}

//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;create;update;delete;watch
//+kubebuilder:rbac:groups=dbaas.redhat.com,resources=dbaasproviders,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=dbaas.redhat.com,resources=dbaasproviders/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dbaas.redhat.com,resources=dbaasproviders/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DbaasProvider object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *DBaaSProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// your logic here

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DBaaSProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	customRateLimiter := workqueue.NewItemExponentialFailureRateLimiter(30*time.Second, 30*time.Minute)

	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{RateLimiter: customRateLimiter}).
		For(&v1.Deployment{}).
		WithEventFilter(r.ignoreOtherDeployments()).
		Complete(r)
}

// ignoreOtherDeployments  only on a 'create' event is issued for the deployment
func (r *DBaaSProviderReconciler) ignoreOtherDeployments() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return r.evaluatePredicateObject(e.Object)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}
}

func (r *DBaaSProviderReconciler) evaluatePredicateObject(obj client.Object) bool {
	lbls := obj.GetLabels()
	if obj.GetNamespace() == r.operatorInstallNamespace {
		if val, keyFound := lbls["olm.owner.kind"]; keyFound {
			if val == "ClusterServiceVersion" {
				if val, keyFound := lbls["olm.owner"]; keyFound {
					return val == r.operatorNameVersion
				}
			}
		}
	}
	return false
}
