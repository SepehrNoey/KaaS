package cluster

import (
	"context"
	"fmt"
	"strings"

	"github.com/SepehrNoey/KaaS/api"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type ClusterManager struct {
	Clientset *kubernetes.Clientset
}

func NewClusterManager() (*ClusterManager, error) {
	config, err := clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return &ClusterManager{Clientset: clientset}, nil
}

func (c *ClusterManager) DeployApp(ctx context.Context, appreq *api.AppRequest) error {
	namespace := "default"
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
							Name:  appreq.Name,
							Image: appreq.Image,
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
		pathType := netv1.PathTypePrefix
		host := fmt.Sprintf("%s.kaas.local", appreq.Name)
		ingress := &netv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appreq.Name,
				Namespace: namespace,
				Annotations: map[string]string{
					"nginx.ingress.kubernetes.io/rewrite-target": "/",
				},
			},
			Spec: netv1.IngressSpec{
				Rules: []netv1.IngressRule{
					{
						Host: host,
						IngressRuleValue: netv1.IngressRuleValue{
							HTTP: &netv1.HTTPIngressRuleValue{
								Paths: []netv1.HTTPIngressPath{
									{
										Path:     "/",
										PathType: &pathType,
										Backend: netv1.IngressBackend{
											Service: &netv1.IngressServiceBackend{
												Name: service.Name,
												Port: netv1.ServiceBackendPort{
													Number: appreq.Port,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		_, err := c.Clientset.NetworkingV1().Ingresses(namespace).Create(ctx, ingress, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create ingress: %v", err)
		}
	}

	return nil
}

func (c *ClusterManager) GetAppStatus(ctx context.Context, name string) (api.AppStatus, error) {
	namespace := "default"
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
	namespace := "default"
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
