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
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

func DeleteResource(r client.Client, ctx context.Context, key client.ObjectKey, obj client.Object) error {
	if !IsExistResource(r, ctx, key, obj) {
		return nil
	}

	return r.Delete(ctx, obj)
}

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

func (obj *ResourceLimit) equ(other *ResourceLimit) bool {

	if obj == nil || other == nil {
		return false
	}

	return obj.Cpu == other.Cpu && obj.Memory == other.Memory
}

type Label struct {
	Key string `json:"key"`
	Val string `json:"value"`
}

type MySQLHostNode struct {
	//MySQL调度到该节点
	NodeName     string `json:"nodename,omitempty"`
	Dir          string `json:"dir"`
	NodeSelector *Label `json:"nodeSelector,omitempty"`
}

// 表示一个msyqld服务的配置
type MySQLd struct {
	Image                string         `json:"image"`
	RootPassword         string         `json:"rootpassword"`
	MycnfConfigMapName   string         `json:"mycnf_cm,omitempty"`
	InitSQLConfigMapName string         `json:"init_sql_cm,omitempty"`
	Request              *ResourceLimit `json:"request,omitempty"`
	Limit                *ResourceLimit `json:"limit,omitempty"`
}

type DataDir struct {
	Host *MySQLHostNode `json:"host,omitempty"`
	Pvc  string         `json:"pvc,omitempty"`
}

type MySQLService struct {
	Mysqld  MySQLd  `json:"mysqld"`
	Datadir DataDir `json:"datadir"`
}

func (obj *MySQLService) Update(r client.Client, ctx context.Context, podName, namespace string, spec *MySQLSpec) error {

	found := &corev1.Pod{}
	if !IsExistResource(r, ctx, types.NamespacedName{Name: podName, Namespace: namespace}, found) {
		return obj.Install(r, ctx, podName, namespace, spec)
	}

	return nil
}

func (obj *MySQLService) Delete(r client.Client, ctx context.Context, podName, namespace string) error {
	found := &corev1.Pod{}
	return DeleteResource(r, ctx, types.NamespacedName{Name: podName, Namespace: namespace}, found)
}

func (obj *MySQLService) Install(r client.Client, ctx context.Context, podName, namespace string, spec *MySQLSpec) error {

	log := log.FromContext(ctx)
	found := &corev1.Pod{}

	if IsExistResource(r, ctx, types.NamespacedName{Name: podName, Namespace: namespace}, found) {
		log.Info("pod has exists", "podName", podName, "namespace", namespace)
		return nil
	}

	pathDir := corev1.HostPathDirectoryOrCreate

	//默认的cpu和memory限制
	defaultResource := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("1000m"),
		corev1.ResourceMemory: resource.MustParse("2G"),
	}

	resourceRequest := defaultResource
	if obj.Mysqld.Request != nil {
		resourceRequest = corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(obj.Mysqld.Request.Cpu),
			corev1.ResourceMemory: resource.MustParse(obj.Mysqld.Request.Memory)}
	}

	resourceLimit := defaultResource
	if obj.Mysqld.Limit != nil {
		resourceLimit = corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(obj.Mysqld.Limit.Cpu),
			corev1.ResourceMemory: resource.MustParse(obj.Mysqld.Limit.Memory)}
	}

	volumes := []corev1.Volume{}

	if obj.Datadir.Host != nil {
		volumes = append(volumes,
			corev1.Volume{
				Name: "data",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: obj.Datadir.Host.Dir,
						Type: &pathDir,
					},
				}})
	} else if obj.Datadir.Pvc != "" {
		volumes = append(volumes,
			corev1.Volume{
				Name: "data",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: obj.Datadir.Pvc,
						ReadOnly:  false,
					},
				}})
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

	containerArray := []corev1.Container{
		corev1.Container{
			Name: "mysql",
			Resources: corev1.ResourceRequirements{
				Limits:   resourceLimit,
				Requests: resourceRequest,
			},
			Args:  []string{"--socket=/var/lib/mysql/mysqld.sock"},
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
		},
	}

	if spec.GlobObj != nil && spec.GlobObj.Xtra != nil {

		// /usr/local/bin/my_xtrabackup --no-defaults --no-lock --binlog-log=off --parallel=16 --user=root --password=iU8yFSvDP6FeuMJl --host=${HOSTNAME} --backup --backup-listen --listen-port=9000 --stream=xbstream --datadir=/source_mysql_data --target-dir=/tmp/a

		containerArray = append(containerArray, corev1.Container{
			Name: "xtrabackup",
			Command: []string{"/usr/local/bin/my_xtrabackup",
				"--no-defaults",
				"--no-lock",
				"--user=root",
				"--password=" + obj.Mysqld.RootPassword,
				"--binlog-log=off",
				"--parallel=16",
				"--socket=/var/lib/mysql/mysqld.sock",
				"--backup",
				"--backup-listen",
				"--listen-port=9000",
				"--stream=xbstream",
				"--datadir=/var/lib/mysql",
				"--target-dir=/tmp/",
			},
			Image: spec.GlobObj.Xtra.Image,
			VolumeMounts: []corev1.VolumeMount{
				corev1.VolumeMount{
					Name:      "data",
					MountPath: "/var/lib/mysql",
					SubPath:   "mysql",
					ReadOnly:  true,
				},
			},
		})
	}

	ImagePullSecretsArray := []corev1.LocalObjectReference{}
	if spec.GlobObj != nil && spec.GlobObj.ImagePullSecrets != "" {
		ImagePullSecretsArray = append(ImagePullSecretsArray, corev1.LocalObjectReference{Name: spec.GlobObj.ImagePullSecrets})
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			Labels:    map[string]string{"app": podName, "mysql": "true"},
		},
		Spec: corev1.PodSpec{
			Volumes:          volumes,
			Containers:       containerArray,
			ImagePullSecrets: ImagePullSecretsArray,
			Affinity: &corev1.Affinity{
				PodAntiAffinity: &corev1.PodAntiAffinity{ //POD反亲和，尽量避免将mysql调度到同一个机器节点上
					PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
						corev1.WeightedPodAffinityTerm{
							Weight: 1,
							PodAffinityTerm: corev1.PodAffinityTerm{
								LabelSelector: &metav1.LabelSelector{ //这个标签针对的不是Node，而是POD
									MatchLabels: map[string]string{
										"mysql": "true",
									}},
								TopologyKey: "kubernetes.io/hostname",
							},
						},
					},
				},
			},
		},
	}

	if obj.Datadir.Host != nil {
		if obj.Datadir.Host.NodeName != "" {
			pod.Spec.NodeName = obj.Datadir.Host.NodeName
		} else if obj.Datadir.Host.NodeSelector != nil {
			pod.Spec.Affinity.NodeAffinity = &corev1.NodeAffinity{ //Node亲和
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						corev1.NodeSelectorTerm{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								corev1.NodeSelectorRequirement{
									Key:      obj.Datadir.Host.NodeSelector.Key,
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{obj.Datadir.Host.NodeSelector.Val},
								},
							},
						},
					},
				},
			}
		}
	}

	if err := r.Create(ctx, pod); err != nil {
		log.Error(err, fmt.Sprintf("create pod [%s/%s] fails", podName, namespace))
		return nil
	}

	log.Info(fmt.Sprintf("create pod [%s/%s] success", podName, namespace))
	return nil
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

func (obj *DeployCase) Install(r client.Client, ctx context.Context, spec *MySQLSpec) error {

	if obj.Single != nil {
		podName := fmt.Sprintf("mysql-single-%s", obj.Name)

		if err := obj.installService(r, ctx, podName, spec); err != nil {
			return err
		}

		if err := obj.Single.Install(r, ctx, podName, obj.Namespace, spec); err != nil {
			obj.deleteService(r, ctx)
			return err
		}
	}

	return nil
}

func (obj *DeployCase) Update(r client.Client, ctx context.Context, spec *MySQLSpec) error {

	if obj.Single != nil {
		podName := fmt.Sprintf("mysql-single-%s", obj.Name)

		if err := obj.updateService(r, ctx, podName, spec); err != nil {
			return err
		}

		return obj.Single.Update(r, ctx, podName, obj.Namespace, spec)
	}

	return nil
}

func (obj *DeployCase) Delete(r client.Client, ctx context.Context) error {

	if err := obj.deleteService(r, ctx); err != nil {
		return err
	}

	if obj.Single != nil {
		podName := fmt.Sprintf("mysql-single-%s", obj.Name)
		return obj.Single.Delete(r, ctx, podName, obj.Namespace)
	}

	return nil
}

func (obj *DeployCase) getServiceName() string {
	if obj.ServiceName == "" {
		return fmt.Sprintf("svc-mysql-%s", obj.Name)
	}

	return obj.ServiceName
}

func (obj *DeployCase) deleteService(r client.Client, ctx context.Context) error {
	found := &corev1.Service{}
	return DeleteResource(r, ctx, types.NamespacedName{Name: obj.getServiceName(), Namespace: obj.Namespace}, found)
}

func (obj *DeployCase) updateService(r client.Client, ctx context.Context, selectorAppLabel string, spec *MySQLSpec) error {
	found := &corev1.Service{}

	if !IsExistResource(r, ctx, types.NamespacedName{Name: obj.getServiceName(), Namespace: obj.Namespace}, found) {
		return obj.installService(r, ctx, selectorAppLabel, spec)
	}

	if found.Spec.Selector == nil {
		found.Spec.Selector = make(map[string]string)
	}

	if found.Spec.Selector["app"] != selectorAppLabel {
		found.Spec.Selector["app"] = selectorAppLabel
		return r.Update(ctx, found)
	}

	return nil
}

func (obj *DeployCase) installService(r client.Client, ctx context.Context, selectorAppLabel string, spec *MySQLSpec) error {

	found := &corev1.Service{}

	if IsExistResource(r, ctx, types.NamespacedName{Name: obj.getServiceName(), Namespace: obj.Namespace}, found) {
		return nil
	}

	portArray := []corev1.ServicePort{corev1.ServicePort{
		Name:       "mysql",
		Port:       3306,
		TargetPort: intstr.FromInt(3306),
	}}

	if spec.GlobObj != nil && spec.GlobObj.Xtra != nil {
		portArray = append(portArray, corev1.ServicePort{
			Name:       "xtrabackup",
			Port:       9000,
			TargetPort: intstr.FromInt(9000),
		})
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.getServiceName(),
			Namespace: obj.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports:    portArray,
			Selector: map[string]string{"app": selectorAppLabel},
		},
	}

	return r.Create(ctx, service)
}

type Xtrabackup struct {
	Image string `json:"image"`
}

type Global struct {
	ImagePullSecrets string      `json:"imagePullSecrets,omitempty"`
	Xtra             *Xtrabackup `json:"xtrabackup,omitempty"`
}

// MySQLSpec defines the desired state of MySQL
type MySQLSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	GlobObj *Global      `json:"global,omitempty"`
	Group   []DeployCase `json:"group"`
}

func (obj *MySQLSpec) Install(r client.Client, ctx context.Context, name, namespace string, callback func(name string)) error {
	//log := log.FromContext(ctx)

	for _, item := range obj.Group {
		if item.Namespace == "" {
			item.Namespace = namespace
		}

		if err := item.Install(r, ctx, obj); err != nil {
			return err
		}

		callback(item.Name)
	}

	return nil
}

func (obj *MySQLSpec) Update(r client.Client, ctx context.Context, name, namespace string, prevGroup map[string]int, spec *MySQLSpec) error {
	log := log.FromContext(ctx)

	for _, item := range obj.Group {
		delete(prevGroup, item.Name)

		if item.Namespace == "" {
			item.Namespace = namespace
		}

		if err := item.Update(r, ctx, spec); err != nil {
			log.Error(err, "update fails")
		}
	}

	return nil
}

func (obj *MySQLSpec) Delete(r client.Client, ctx context.Context, name, namespace string) error {
	log := log.FromContext(ctx)

	for _, item := range obj.Group {
		if item.Namespace == "" {
			item.Namespace = namespace
		}

		if err := item.Delete(r, ctx); err != nil {
			log.Error(err, "delete fails")
		}
	}

	return nil
}

// MySQLStatus defines the observed state of MySQL
type MySQLStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	GroupStatus map[string]int `json:"group"`
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
	log := log.FromContext(ctx)

	if err := obj.Spec.Install(r, ctx, obj.Name, obj.Namespace, func(gName string) {
		obj.Status.GroupStatus[gName] = 1
	}); err != nil {
		log.Error(err, fmt.Sprintf("CRD/MySQL [%s/%s] Install fails", obj.Name, obj.Namespace))
		return err
	}

	r.Status().Update(ctx, obj)
	log.Info(fmt.Sprintf("CRD/MySQL [%s/%s] Install success", obj.Name, obj.Namespace))
	return nil
}

func (obj *MySQL) Update(r client.Client, ctx context.Context) error {
	log := log.FromContext(ctx)
	if err := obj.Spec.Update(r, ctx, obj.Name, obj.Namespace, obj.Status.GroupStatus, &obj.Spec); err != nil {
		log.Error(err, fmt.Sprintf("CRD/MySQL [%s/%s] Update fails", obj.Name, obj.Namespace))
		return err
	}

	log.Info(fmt.Sprintf("CRD/MySQL [%s/%s] Update success", obj.Name, obj.Namespace))
	return nil
}

func (obj *MySQL) Delete(r client.Client, ctx context.Context) error {
	log := log.FromContext(ctx)
	if err := obj.Spec.Delete(r, ctx, obj.Name, obj.Namespace); err != nil {
		log.Error(err, fmt.Sprintf("CRD/MySQL [%s/%s] Delete fails", obj.Name, obj.Namespace))
		return err
	}

	log.Info(fmt.Sprintf("CRD/MySQL [%s/%s] Delete success", obj.Name, obj.Namespace))
	return nil
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
