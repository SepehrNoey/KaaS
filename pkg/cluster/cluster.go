package cluster

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"github.com/SepehrNoey/KaaS/api"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type AppCnfMap struct {
	IngressName string
	Namespace   string
}

type DBCnfMap struct {
	Replica int32
	MaxConn int32
	Port    int32
	PVCSize string
	Image   Image
}

type Image struct {
	Repository string
	PullPolicy corev1.PullPolicy
}

type ClusterManager struct {
	Clientset *kubernetes.Clientset
	AppConf   AppCnfMap
	DBConf    DBCnfMap
}

func NewClusterManager() (*ClusterManager, error) {
	conf, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-cluster config: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(conf)
	if err != nil {
		return nil, err
	}

	appConf, err := clientset.CoreV1().ConfigMaps("default").Get(context.Background(), "kaas-config", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	dbConf, err := clientset.CoreV1().ConfigMaps("default").Get(context.Background(), "db-request-config", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return &ClusterManager{
		Clientset: clientset,
		AppConf: AppCnfMap{
			IngressName: appConf.Data["ingress.name"],
			Namespace:   appConf.Data["namespace"],
		},
		DBConf: DBCnfMap{
			Replica: parseInt32(dbConf.Data["replica"]),
			MaxConn: parseInt32(dbConf.Data["maxConnections"]),
			Port:    parseInt32(dbConf.Data["port"]),
			PVCSize: dbConf.Data["pvcSize"],
			Image: Image{
				Repository: dbConf.Data["image.repository"],
				PullPolicy: corev1.PullPolicy(dbConf.Data["image.pullPolicy"]),
			},
		},
	}, nil
}

func (c *ClusterManager) DeployApp(ctx context.Context, appreq *api.AppRequest) error {
	namespace := c.AppConf.Namespace

	if exists, err := c.resourceExists("deployment", appreq.Name); exists {
		return fmt.Errorf("deployment with this name exists: %v", err)
	}

	parts := strings.Split(appreq.Resources, ",")
	if len(parts) != 3 {
		return fmt.Errorf("expected 3 parts for resources, got %d", len(parts))
	}

	cpu := parts[0]
	mem := parts[1]
	disk := parts[2]
	resReqs := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:              resource.MustParse(cpu),
			corev1.ResourceMemory:           resource.MustParse(mem),
			corev1.ResourceEphemeralStorage: resource.MustParse(disk),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:              resource.MustParse(cpu),
			corev1.ResourceMemory:           resource.MustParse(mem),
			corev1.ResourceEphemeralStorage: resource.MustParse(disk),
		},
	}

	env := []corev1.EnvVar{}
	for key, value := range appreq.Envs {
		env = append(env, corev1.EnvVar{
			Name:  key,
			Value: value,
		})
	}

	for key, value := range appreq.Secrets {
		env = append(env, corev1.EnvVar{
			Name: key,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: value,
					},
					Key: key,
				},
			},
		})
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appreq.Name,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": appreq.Name},
			},
			Replicas: &appreq.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": appreq.Name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            appreq.Name,
							Image:           appreq.Image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: appreq.Port,
								},
							},
							Resources: resReqs,
							Env:       env,
						},
					},
				},
			},
		},
	}

	_, err := c.Clientset.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create deployment: %v", err)
	}

	serviceType := corev1.ServiceTypeClusterIP
	if appreq.ExternalAccess {
		serviceType = corev1.ServiceTypeNodePort
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appreq.Name,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": appreq.Name},
			Ports: []corev1.ServicePort{
				{
					Port: appreq.Port,
				},
			},
			Type: serviceType,
		},
	}

	_, err = c.Clientset.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create service: %v", err)
	}

	if appreq.ExternalAccess {
		if err := c.updateIngress(ctx, appreq); err != nil {
			return err
		}
	}

	return nil
}

func (c *ClusterManager) GetAppStatus(ctx context.Context, name string) (api.AppStatus, error) {
	namespace := c.AppConf.Namespace
	deployment, err := c.Clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return api.AppStatus{}, fmt.Errorf("failed to get deployment: %v", err)
	}

	pods, err := c.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", name),
	})
	if err != nil {
		return api.AppStatus{}, fmt.Errorf("failed to list pods: %v", err)
	}

	podStatuses := []api.PodStatus{}
	for _, pod := range pods.Items {
		podStatuses = append(podStatuses, api.PodStatus{
			Name:      pod.Name,
			Phase:     string(pod.Status.Phase),
			HostIP:    pod.Status.HostIP,
			PodIP:     pod.Status.PodIP,
			StartTime: pod.Status.StartTime.Time,
		})
	}

	return api.AppStatus{
		DeploymentName: deployment.Name,
		Namespace:      deployment.Namespace,
		Replicas:       *deployment.Spec.Replicas,
		ReadyReplicas:  deployment.Status.ReadyReplicas,
		PodStatuses:    podStatuses,
	}, nil
}

func (c *ClusterManager) GetAllAppsStatus(ctx context.Context) ([]api.AppStatus, error) {
	namespace := c.AppConf.Namespace
	deployments, err := c.Clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get deployments: %v", err)
	}

	var statuses []api.AppStatus
	for _, deployment := range deployments.Items {
		status, err := c.GetAppStatus(ctx, deployment.Name)
		if err != nil {
			statuses = append(statuses, api.AppStatus{
				DeploymentName: deployment.Name,
				Namespace:      deployment.Namespace,
				ErrMsg:         err.Error(),
			})
		}
		statuses = append(statuses, status)
	}

	return statuses, nil
}

func (c *ClusterManager) updateIngress(ctx context.Context, appreq *api.AppRequest) error {
	namespace := c.AppConf.Namespace
	ingName := c.AppConf.IngressName
	ingClient := c.Clientset.NetworkingV1().Ingresses(namespace)
	ingress, err := ingClient.Get(ctx, ingName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get Ingress resource: %v", err)
	}

	// updating rules
	pathType := netv1.PathTypePrefix
	host := appreq.DomainAddress
	ingress.Spec.Rules = append(ingress.Spec.Rules, netv1.IngressRule{
		Host: host,
		IngressRuleValue: netv1.IngressRuleValue{
			HTTP: &netv1.HTTPIngressRuleValue{
				Paths: []netv1.HTTPIngressPath{
					{
						Path:     "/",
						PathType: &pathType,
						Backend: netv1.IngressBackend{
							Service: &netv1.IngressServiceBackend{
								Name: appreq.Name,
								Port: netv1.ServiceBackendPort{
									Number: appreq.Port,
								},
							},
						},
					},
				},
			},
		},
	})

	_, err = ingClient.Update(ctx, ingress, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update Ingress resource: %v", err)
	}

	return nil
}

func (c *ClusterManager) DeployDatabase(ctx context.Context, dbreq *api.DBRequest) error {
	namespace := c.AppConf.Namespace

	maxRand := 100000
	username := fmt.Sprintf("user-%d", rand.Intn(maxRand))
	password := fmt.Sprintf("pass-%d", rand.Intn(maxRand))
	secretName := dbreq.DBName + "-secret"

	if exists, err := c.resourceExists("secret", secretName); exists {
		return fmt.Errorf("database with this name exists: %v", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		StringData: map[string]string{
			"username": username,
			"password": password,
		},
	}

	_, err := c.Clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create secret: %v", err)
	}

	// parse resources
	resources := strings.Split(dbreq.Resources, ",")
	if len(resources) != 3 {
		return fmt.Errorf("expected 3 parts for resources, got %d", len(resources))
	}
	cpu := resources[0]
	memory := resources[1]
	disk := resources[2]

	// create StatefulSet
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dbreq.DBName,
			Namespace: namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: dbreq.DBName,
			Replicas:    &c.DBConf.Replica,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": dbreq.DBName},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": dbreq.DBName},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "postgres",
							Image:           c.DBConf.Image.Repository,
							ImagePullPolicy: c.DBConf.Image.PullPolicy,
							Ports: []corev1.ContainerPort{
								{ContainerPort: c.DBConf.Port},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "POSTGRES_DB",
									Value: dbreq.DBName,
								},
								{
									Name: "POSTGRES_USER",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: secretName,
											},
											Key: "username",
										},
									},
								},
								{
									Name: "POSTGRES_PASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: secretName,
											},
											Key: "password",
										},
									},
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse(cpu),
									corev1.ResourceMemory: resource.MustParse(memory),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse(cpu),
									corev1.ResourceMemory: resource.MustParse(memory),
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "data",
									MountPath: "/var/lib/postgresql/data",
								},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "data",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteMany,
						},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse(disk),
							},
						},
					},
				},
			},
		},
	}

	_, err = c.Clientset.AppsV1().StatefulSets(namespace).Create(ctx, statefulSet, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create statefulset: %v", err)
	}

	// create service
	serviceType := corev1.ServiceTypeClusterIP
	if dbreq.ExternalAccess {
		serviceType = corev1.ServiceTypeNodePort
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dbreq.DBName,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": dbreq.DBName},
			Ports: []corev1.ServicePort{
				{
					Port: c.DBConf.Port,
				},
			},
			Type: serviceType,
		},
	}

	_, err = c.Clientset.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create service: %v", err)
	}

	return nil

}

func (c *ClusterManager) resourceExists(resourceType, resourceName string) (bool, error) {
	namespace := c.AppConf.Namespace

	switch resourceType {
	case "secret":
		_, err := c.Clientset.CoreV1().Secrets(namespace).Get(context.Background(), resourceName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return true, nil

	case "deployment":
		_, err := c.Clientset.AppsV1().Deployments(namespace).Get(context.Background(), resourceName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return true, nil

	default:
		return false, fmt.Errorf("unsupported resource type: %s", resourceType)
	}
}

func parseInt32(s string) int32 {
	num, _ := strconv.ParseInt(s, 10, 32)
	return int32(num)
}
