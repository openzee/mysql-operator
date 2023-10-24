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

	"k8s.io/apimachinery/pkg/api/resource"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	atlasaibeecnv1beta1 "github.com/openzee/mysql-operator/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// MySQLReconciler reconciles a MySQL object
type MySQLReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	resourceVersion string
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
		log.Error(err, "GET MySQL fails")
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	for _, g := range instance.Spec.Group {
		if g.Namespace == "" {
			g.Namespace = instance.Namespace
		}

		r.ReconcileGroup(ctx, &g, r.resourceVersion != instance.ResourceVersion)
	}

	r.resourceVersion = instance.ResourceVersion

	return ctrl.Result{RequeueAfter: time.Second * 5}, nil
}

func (r *MySQLReconciler) ReconcileMasterSlave(ctx context.Context, g *atlasaibeecnv1beta1.MySQLGroup) error {

	return nil
}

func (r *MySQLReconciler) ReconcileBackup(ctx context.Context, g *atlasaibeecnv1beta1.MySQLGroup) error {

	return nil
}

func (r *MySQLReconciler) IsExistsConfigMap(ctx context.Context, name, namespace string) (bool, error) {

	found := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, found); err != nil {

		if apierrors.IsNotFound(err) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

func (r *MySQLReconciler) NewPod(ctx context.Context, podName, namespace string, mysql_service *atlasaibeecnv1beta1.MySQLService) *corev1.Pod {

	pathDir := corev1.HostPathDirectoryOrCreate

	//默认的cpu和memory限制
	defaultResource := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("1000m"),
		corev1.ResourceMemory: resource.MustParse("2G"),
	}

	resourceRequest := defaultResource
	if mysql_service.Host.Request != nil {
		resourceRequest = corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(mysql_service.Host.Request.Cpu),
			corev1.ResourceMemory: resource.MustParse(mysql_service.Host.Request.Memory)}
	}

	resourceLimit := defaultResource
	if mysql_service.Host.Limit != nil {
		resourceLimit = corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(mysql_service.Host.Limit.Cpu),
			corev1.ResourceMemory: resource.MustParse(mysql_service.Host.Limit.Memory)}
	}

	volumes := []corev1.Volume{
		corev1.Volume{
			Name: "data",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: mysql_service.Host.Dir,
					Type: &pathDir},
			}},
	}

	volumeMounts := []corev1.VolumeMount{
		corev1.VolumeMount{
			Name:      "data",
			MountPath: "/var/lib/mysql",
			SubPath:   "mysql",
		},
	}

	if ok, _ := r.IsExistsConfigMap(ctx, mysql_service.Mysqld.MycnfConfigMapName, namespace); ok == true {
		volumes = append(volumes, corev1.Volume{
			Name: "mycnf",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: mysql_service.Mysqld.MycnfConfigMapName,
					}},
			}})

		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "mycnf",
			MountPath: "/etc/mysql/mysql.conf.d/mysqld.cnf",
			SubPath:   "mysqld.cnf",
		})
	}

	if ok, _ := r.IsExistsConfigMap(ctx, mysql_service.Mysqld.InitSQLConfigMapName, namespace); ok == true {
		volumes = append(volumes, corev1.Volume{
			Name: "initsql",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: mysql_service.Mysqld.InitSQLConfigMapName,
					}},
			}})

		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "initsql",
			MountPath: "docker-entrypoint-initdb.d/init.sql",
			SubPath:   "init.sql",
		})
	}

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			Volumes:  volumes,
			NodeName: mysql_service.Host.NodeName,
			Containers: []corev1.Container{corev1.Container{
				Name: "mysql",
				Resources: corev1.ResourceRequirements{
					Limits:   resourceLimit,
					Requests: resourceRequest,
				},
				Image: mysql_service.Mysqld.Image,
				Ports: []corev1.ContainerPort{
					corev1.ContainerPort{
						Name:          "mysql",
						ContainerPort: 3306,
					}},
				VolumeMounts: volumeMounts,
				Env: []corev1.EnvVar{corev1.EnvVar{
					Name:  "MYSQL_ROOT_PASSWORD",
					Value: mysql_service.Mysqld.RootPassword,
				}},
			}},
		},
	}
}

func (r *MySQLReconciler) ReconcileSingle(ctx context.Context, g *atlasaibeecnv1beta1.MySQLGroup, update bool) error {
	log := log.FromContext(ctx)
	podName := fmt.Sprintf("mysql-single-%s", g.Name)
	found := &corev1.Pod{}

	if err := r.Get(ctx, types.NamespacedName{Name: podName, Namespace: g.Namespace}, found); err != nil {

		if !apierrors.IsNotFound(err) {
			log.Error(err, "get pod fails")
			return err
		}

		if err := r.Create(ctx, r.NewPod(ctx, podName, g.Namespace, g.Single)); err != nil {
			log.Error(err, "create pod fails")
			return err
		}

		log.Info("create pod success")
		return nil
	}

	if update {

		found.Spec.Containers[0].Image = g.Single.Mysqld.Image

		if err := r.Update(ctx, found); err != nil {
			fmt.Println("update fails", err)
			return err
		}

		fmt.Println("update success")
		return nil
	}

	fmt.Println("none")
	return nil
}

func (r *MySQLReconciler) ReconcileGroup(ctx context.Context, g *atlasaibeecnv1beta1.MySQLGroup, update bool) error {

	//单实例模式和主从模式仅选一个，优先支持单实例模式

	if g.Single != nil {
		return r.ReconcileSingle(ctx, g, update)
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
