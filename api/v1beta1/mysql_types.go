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

package v1beta1

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

func IsExistResource(r client.Client, ctx context.Context, key client.ObjectKey, obj client.Object) bool {

	if err := r.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return false
		}

		panic(err)
	}

	return true
}

type ResourceLimit struct {
	Cpu    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
}

type MySQLHostNode struct {
	//MySQL调度到该节点
	NodeName string         `json:"nodename"`
	Dir      string         `json:"dir"`
	Request  *ResourceLimit `json:"request,omitempty"`
	Limit    *ResourceLimit `json:"limit,omitempty"`
}

// 表示一个msyqld服务的配置
type MySQLd struct {
	Image                string `json:"image"`
	RootPassword         string `json:"rootpassword,omitempty"`
	MycnfConfigMapName   string `json:"mycnf_cm,omitempty"`
	InitSQLConfigMapName string `json:"init_sql_cm,omitempty"`
}

type MySQLService struct {
	Host   MySQLHostNode `json:"host"`
	Mysqld MySQLd        `json:"mysqld"`
}

func (obj *MySQLService) Install(r client.Client, ctx context.Context, podName, namespace string) error {

	found := &corev1.Pod{}

	if IsExistResource(r, ctx, types.NamespacedName{Name: podName, Namespace: namespace}, found) {
		return nil
	}

	pathDir := corev1.HostPathDirectoryOrCreate

	//默认的cpu和memory限制
	defaultResource := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("1000m"),
		corev1.ResourceMemory: resource.MustParse("2G"),
	}

	resourceRequest := defaultResource
	if obj.Host.Request != nil {
		resourceRequest = corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(obj.Host.Request.Cpu),
			corev1.ResourceMemory: resource.MustParse(obj.Host.Request.Memory)}
	}

	resourceLimit := defaultResource
	if obj.Host.Limit != nil {
		resourceLimit = corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(obj.Host.Limit.Cpu),
			corev1.ResourceMemory: resource.MustParse(obj.Host.Limit.Memory)}
	}

	volumes := []corev1.Volume{
		corev1.Volume{
			Name: "data",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: obj.Host.Dir,
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

	if IsExistResource(r, ctx, types.NamespacedName{Name: obj.Mysqld.MycnfConfigMapName, Namespace: namespace}, &corev1.ConfigMap{}) {
		volumes = append(volumes, corev1.Volume{
			Name: "mycnf",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: obj.Mysqld.MycnfConfigMapName,
					}},
			}})

		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "mycnf",
			MountPath: "/etc/mysql/mysql.conf.d/mysqld.cnf",
			SubPath:   "mysqld.cnf",
		})
	}

	if IsExistResource(r, ctx, types.NamespacedName{Name: obj.Mysqld.InitSQLConfigMapName, Namespace: namespace}, &corev1.ConfigMap{}) {
		volumes = append(volumes, corev1.Volume{
			Name: "initsql",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: obj.Mysqld.InitSQLConfigMapName,
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
			NodeName: obj.Host.NodeName,
			Containers: []corev1.Container{corev1.Container{
				Name: "mysql",
				Resources: corev1.ResourceRequirements{
					Limits:   resourceLimit,
					Requests: resourceRequest,
				},
				Image: obj.Mysqld.Image,
				Ports: []corev1.ContainerPort{
					corev1.ContainerPort{
						Name:          "mysql",
						ContainerPort: 3306,
					}},
				VolumeMounts: volumeMounts,
				Env: []corev1.EnvVar{corev1.EnvVar{
					Name:  "MYSQL_ROOT_PASSWORD",
					Value: obj.Mysqld.RootPassword,
				}},
			}},
		},
	}

	return r.Create(ctx, pod)
}

type MySQLMasterSlave struct {
	Master MySQLService `json:"master"`
	Slave  MySQLService `json:"slave"`
}

type BackupPolicy struct {
	Host string `json:"host"`
	Dir  string `json:"dir"`
}

type DeployCase struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace,omitempty"`
	ServiceName string            `json:"service_name,omitempty"`
	Backup      *BackupPolicy     `json:"backup,omitempty"`
	Single      *MySQLService     `json:"single,omitempty"`
	MasterSlave *MySQLMasterSlave `json:"masterslave,omitempty"`
}

func (obj *DeployCase) Install(r client.Client, ctx context.Context, name, namespace string) error {

	if err := obj.InstallService(r, ctx); err != nil {
		return err
	}

	if obj.Single != nil {
		return obj.Single.Install(r, ctx, fmt.Sprintf("mysql-single-%s", name), namespace)
	}

	return nil
}

func (obj *DeployCase) Update(r client.Client, ctx context.Context, name, namespace string) error {
	return nil
}

func (obj *DeployCase) Delete(r client.Client, ctx context.Context, name, namespace string) error {
	return nil
}

func (obj *DeployCase) serviceSelectorValue() string {

	return fmt.Sprintf("master-node-%s", obj.Name)
}

func (obj *DeployCase) UpdateService(r client.Client, ctx context.Context, svc_name, svc_ns, selectorAppValue string) error {
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

func (obj *DeployCase) InstallService(r client.Client, ctx context.Context) error {

	found := &corev1.Service{}

	if obj.ServiceName == "" {
		obj.ServiceName = fmt.Sprintf("svc-mysql-%s", obj.Name)
	}

	if IsExistResource(r, ctx, types.NamespacedName{Name: obj.ServiceName, Namespace: obj.Namespace}, found) {
		return nil
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.ServiceName,
			Namespace: obj.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{corev1.ServicePort{
				Name:       "mysql",
				Port:       3306,
				TargetPort: intstr.FromInt(3306),
			}},
			Selector: map[string]string{"app": obj.serviceSelectorValue()},
		},
	}

	return r.Create(ctx, service)
}

// MySQLSpec defines the desired state of MySQL
type MySQLSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Group []DeployCase `json:"group"`

	// Foo is an example field of MySQL. Edit mysql_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

func (obj *MySQLSpec) Install(r client.Client, ctx context.Context, name, namespace string) error {

	for _, item := range obj.Group {
		if item.Namespace == "" {
			item.Namespace = namespace
		}

		item.Install(r, ctx, name, namespace)
	}

	return nil
}

func (obj *MySQLSpec) Update(r client.Client, ctx context.Context, name, namespace string) error {
	for _, item := range obj.Group {
		if item.Namespace == "" {
			item.Namespace = namespace
		}

		item.Update(r, ctx, name, namespace)
	}

	return nil
}

func (obj *MySQLSpec) Delete(r client.Client, ctx context.Context, name, namespace string) error {
	for _, item := range obj.Group {
		if item.Namespace == "" {
			item.Namespace = namespace
		}

		item.Delete(r, ctx, name, namespace)
	}

	return nil
}

// +kubebuilder:object:root=true
// MySQLStatus defines the observed state of MySQL
type MySQLStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// MySQL is the Schema for the mysqls API
type MySQL struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MySQLSpec   `json:"spec,omitempty"`
	Status MySQLStatus `json:"status,omitempty"`
}

func (obj *MySQL) Install(r client.Client, ctx context.Context) error {
	return obj.Spec.Install(r, ctx, obj.Name, obj.Namespace)
}

func (obj *MySQL) Update(r client.Client, ctx context.Context) error {
	return obj.Spec.Update(r, ctx, obj.Name, obj.Namespace)
}

func (obj *MySQL) Delete(r client.Client, ctx context.Context) error {
	return obj.Spec.Delete(r, ctx, obj.Name, obj.Namespace)
}

//+kubebuilder:object:root=true

// MySQLList contains a list of MySQL
type MySQLList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MySQL `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MySQL{}, &MySQLList{})
}
