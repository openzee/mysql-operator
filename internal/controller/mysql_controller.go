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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"k8s.io/apimachinery/pkg/util/intstr"

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

/*
**派生的事件来源有两个
** 1、EnqueueRequestForObject  controller-runtime/pkg/handler/enqueue.go
**    1) Create
**    2) Update
**	  3) Delete
**
**	  事件派生核心代码为：
**	  q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
**		Name:      evt.Object.GetName(),
**		Namespace: evt.Object.GetNamespace(),
**	  }})
**
**	  触发条件：
**	  1、MySQL对象新建：触发 Add
**	  2、MySQL对象删除：触发 Delete
**	  3、MySQL对象更新：触发Update
**    4、operator内通过r.Update更新：也会触发Update事件
**	  4、Operator启动：触发Add
**
** 2、产生的事件,根据Reconcile处理结果或重新入队，再次触发Reconcile的调用
**    1) 返回的err不为空：       ctrl.Request将重新进入延迟队列，后续触发
**	  2) result.Requeue == true: ctrl.Request将重新进入延迟队列，后续触发
**	  3) result.RequeueAfter>0：请求一段事件后，重新入队
**
**   crd/MySQL 对象删除逻辑：
** 	 如果finalizers字段不为空：
		1) crd/MySQL的 DeletionTimestamp 字段被设置为非空
		2) update事件触发
		3) Reconcile执行清理工作
		4) Reconcile更新crd/MySQL将finalizers字段清理
		5) delete事件触发
**当返回 ctrl.Result{}, nil，即表示该事件处理完毕，在新的事件到来前，Reconcile是不会被调用的.
*/

//自定义对象MySQL新建时，

func (r *MySQLReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	_ = log.FromContext(ctx)
	instance := &atlasaibeecnv1beta1.MySQL{}
	// name of our custom finalizer
	myFinalizerName := "mysql.atlas.aibee.cn/finalizer"

	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err //该事件会重新入队，并再次延迟触发
	}

	defer func() {
		r.resourceVersion = instance.ResourceVersion
	}()

	//监测到MySQL对象被删除
	if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is being deleted

		if err := r.deleteExternalResources(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}

		if !controllerutil.ContainsFinalizer(instance, myFinalizerName) {
			return ctrl.Result{}, nil
		}

		controllerutil.RemoveFinalizer(instance, myFinalizerName)
		return ctrl.Result{}, r.Update(ctx, instance)
	}

	//先添加finalizer
	if !controllerutil.ContainsFinalizer(instance, myFinalizerName) {
		controllerutil.AddFinalizer(instance, myFinalizerName)
		fmt.Println("add finalizer and update")
		return ctrl.Result{}, r.Update(ctx, instance)
	}

	//处理更新事件
	//但是在operator没有running期间，crd/MySQL对象的变更应该抓取不到
	if r.resourceVersion != "" && r.resourceVersion != instance.ResourceVersion {
		r.updateExternalResources(ctx, instance)
		return ctrl.Result{}, nil
	}

	//初始化安装或是定时检查服务状态是否存在
	r.installIfNotExistsExternalResources(ctx, instance)

	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *MySQLReconciler) updateExternalResources(ctx context.Context, instance *atlasaibeecnv1beta1.MySQL) error {
	return nil
}

func (r *MySQLReconciler) installIfNotExistsExternalResources(ctx context.Context, instance *atlasaibeecnv1beta1.MySQL) error {

	for _, g := range instance.Spec.Group {
		if g.Namespace == "" {
			g.Namespace = instance.Namespace
		}

		r.InstallIfNotExistsDeployCase(ctx, &g)
	}

	return nil
}

func (r *MySQLReconciler) deleteExternalResources(ctx context.Context, instance *atlasaibeecnv1beta1.MySQL) error {
	//
	// delete any external resources associated with the cronJob
	//
	// Ensure that delete implementation is idempotent and safe to invoke
	// multiple times for same object.
	return nil
}

func (r *MySQLReconciler) ReconcileMasterSlave(ctx context.Context, g *atlasaibeecnv1beta1.DeployCase) error {

	return nil
}

func (r *MySQLReconciler) ReconcileBackup(ctx context.Context, g *atlasaibeecnv1beta1.DeployCase) error {

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

func (r *MySQLReconciler) UpdateService(ctx context.Context, svc_name, svc_ns, selectorAppValue string) error {
	found := &corev1.Service{}

	if err := r.Get(ctx, types.NamespacedName{Name: svc_name, Namespace: svc_ns}, found); err != nil {
		return err
	}

	if found.Spec.Selector["app"] == selectorAppValue {
		return nil
	}

	found.Spec.Selector["app"] = selectorAppValue

	return r.Update(ctx, found)
}

func (r *MySQLReconciler) CreateServiceIfNotExists(ctx context.Context, svc_name, svc_ns, selectorAppValue string) error {

	found := &corev1.Service{}

	if err := r.Get(ctx, types.NamespacedName{Name: svc_name, Namespace: svc_ns}, found); err != nil {

		if !apierrors.IsNotFound(err) {
			return err
		}

		service := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      svc_name,
				Namespace: svc_ns,
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{corev1.ServicePort{
					Name:       "mysql",
					Port:       3306,
					TargetPort: intstr.FromInt(3306),
				}},
				Selector: map[string]string{"app": selectorAppValue},
			},
		}

		return r.Create(ctx, service)
	}

	return nil
}

func (r *MySQLReconciler) CreateMySQLPodIfNotExists(ctx context.Context, podName, namespace string, mysql_service *atlasaibeecnv1beta1.MySQLService) error {

	found := &corev1.Pod{}

	err := r.Get(ctx, types.NamespacedName{Name: podName, Namespace: namespace}, found)

	if err == nil {
		return nil
	}

	if !apierrors.IsNotFound(err) {
		return err
	}

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

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			Labels:    map[string]string{"app": podName},
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

	return r.Create(ctx, pod)
}

func (r *MySQLReconciler) InstallIfNotExistsSingle(ctx context.Context, g *atlasaibeecnv1beta1.DeployCase) error {
	_ = log.FromContext(ctx)

	//podName := fmt.Sprintf("mysql-single-%s", g.Name)

	//found := &corev1.Pod{}

	//if err := r.Get(ctx, types.NamespacedName{Name: podName, Namespace: g.Namespace}, found); err != nil {

	//	if !apierrors.IsNotFound(err) {
	//		log.Error(err, "get pod fails")
	//		return err
	//	}

	//	if err := r.Create(ctx, r.NewPod(ctx, podName, g.Namespace, g.Single)); err != nil {
	//		log.Error(err, "create pod fails")
	//		return err
	//	}

	//	log.Info("create pod success")
	//	return nil
	//}

	//if update {

	//	if err := r.UpdateService(ctx, servcieName, g.Namespace, podName); err != nil {
	//		log.Error(err, "update service fails")
	//	}

	//	flags := false

	//	if found.Spec.Containers[0].Image != g.Single.Mysqld.Image {
	//		found.Spec.Containers[0].Image = g.Single.Mysqld.Image
	//		flags = true
	//	}

	//	if flags {
	//		if err := r.Update(ctx, found); err != nil {
	//			log.Error(err, "update fails")
	//			return err
	//		}
	//		log.Info("update success")
	//	} else {
	//		log.Info("not support update")
	//	}

	//	return nil
	//}

	return nil
}

// 单实例模式和主从模式仅选一个，优先支持单实例模式
func (r *MySQLReconciler) InstallIfNotExistsDeployCase(ctx context.Context, g *atlasaibeecnv1beta1.DeployCase) error {

	log := log.FromContext(ctx)

	if g.ServiceName == "" {
		g.ServiceName = fmt.Sprintf("svc-mysql-%s", g.Name)
	}

	selectorAppValue := fmt.Sprintf("master-node-%s", g.Name)

	if err := r.CreateServiceIfNotExists(ctx, g.ServiceName, g.Namespace, selectorAppValue); err != nil {
		log.Error(err, "create service fails")
	}

	if g.Single != nil {
		return r.InstallIfNotExistsSingle(ctx, g)
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
