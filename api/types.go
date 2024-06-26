package api

import "time"

type AppRequest struct {
	Name           string            `json:"name"`
	Replicas       int32             `json:"replicas"`
	Image          string            `json:"image"`
	ImageTag       string            `json:"image_tag"`
	DomainAddress  string            `json:"domain_address"`
	Port           int32             `json:"port"`
	Resources      string            `json:"resources"` // includes CPU,RAM,DISK respectively. For instance: "500m,128Mi,1Gi"
	Envs           map[string]string `json:"envs"`
	Secrets        map[string]string `json:"secrets"`
	ExternalAccess bool              `json:"external_access"`
}

type PodStatus struct {
	Name      string    `json:"name"`
	Phase     string    `json:"phase"`
	HostIP    string    `json:"hostIP"`
	PodIP     string    `json:"podIP"`
	StartTime time.Time `json:"startTime"`
}

type AppStatus struct {
	DeploymentName string      `json:"deployment_name"`
	Namespace      string      `json:"namespace"`
	Replicas       int32       `json:"replicas"`
	ReadyReplicas  int32       `json:"ready_replicas"`
	PodStatuses    []PodStatus `json:"pod_statuses"`
	ErrMsg         string      `json:"err_msg"`
}

type AllAppsStatus struct {
	Apps []AppStatus `json:"apps"`
}
