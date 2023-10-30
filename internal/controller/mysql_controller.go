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

	//appsv1 "k8s.io/api/apps/v1"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	atlasaibeecnv1beta1 "github.com/openzee/mysql-operator/api/v1beta1"
)

// MySQLReconciler reconciles a MySQL object
type MySQLReconciler struct {
	client.Client
	MyScheme        *runtime.Scheme
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

		if err := instance.Delete(r, ctx); err != nil {
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
		instance.Update(r, ctx)
		return ctrl.Result{}, nil
	}

	//初始化安装或是定时检查服务状态是否存在
	instance.Install(r, ctx)

	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MySQLReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&atlasaibeecnv1beta1.MySQL{}).
		Complete(r)
}
