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

package controller

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	//appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	atlasaibeecnv1beta1 "github.com/openzee/mysql-operator/api/v1beta1"
)

// MySQLReconciler reconciles a MySQL object
type MySQLReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	defaultNs string
}

//+kubebuilder:rbac:groups=atlas.aibee.cn,resources=mysqls,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=atlas.aibee.cn,resources=mysqls/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=atlas.aibee.cn,resources=mysqls/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the MySQL object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.2/pkg/reconcile
func (r *MySQLReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	instance := &atlasaibeecnv1beta1.MySQL{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {

		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get MySQL")
		return ctrl.Result{RequeueAfter: time.Second * 5}, err
	}

	r.defaultNs = instance.Namespace

	for _, g := range instance.Spec.Group {
		if g.Namespace == "" {
			g.Namespace = instance.Namespace
		}

		r.ReconcileGroup(ctx, &g)
	}

	return ctrl.Result{RequeueAfter: time.Second * 5}, nil
}

func (r *MySQLReconciler) ReconcileMasterSlave(ctx context.Context, g *atlasaibeecnv1beta1.MySQLGroup) error {

	return nil
}

func (r *MySQLReconciler) ReconcileBackup(ctx context.Context, g *atlasaibeecnv1beta1.MySQLGroup) error {

	return nil
}

func (r *MySQLReconciler) ReconcileSingle(ctx context.Context, g *atlasaibeecnv1beta1.MySQLGroup) error {

	podName := fmt.Sprintf("mysql-single-%s", g.Name)

	found := &corev1.Pod{}

	err := r.Get(ctx, types.NamespacedName{Name: podName, Namespace: g.Namespace}, found)
	if err == nil {
		fmt.Println("get ok")
		return nil
	}

	if apierrors.IsNotFound(err) {

		fmt.Println("not found")
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName,
				Namespace: g.Namespace,
			},
			Spec: corev1.PodSpec{
				NodeName: "bj-cpu077.aibee.cn",
				Containers: []corev1.Container{corev1.Container{
					Name:  "mysql",
					Image: g.InitConfig.Image,
					Env:   []corev1.EnvVar{corev1.EnvVar{Name: "MYSQL_ROOT_PASSWORD", Value: g.InitConfig.RootPassword}},
				}},
			},
		}

		if err := r.Create(ctx, pod); err != nil {
			fmt.Println(err)
		}
	}

	return nil
}

func (r *MySQLReconciler) ReconcileGroup(ctx context.Context, g *atlasaibeecnv1beta1.MySQLGroup) error {

	//单实例模式和主从模式仅选一个，优先支持单实例模式

	if g.Single != nil {
		return r.ReconcileSingle(ctx, g)
	}

	if g.MasterSlave != nil {
		return r.ReconcileMasterSlave(ctx, g)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MySQLReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&atlasaibeecnv1beta1.MySQL{}).
		Complete(r)
}
