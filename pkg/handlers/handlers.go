package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/SepehrNoey/KaaS/api"
	"github.com/SepehrNoey/KaaS/pkg/cluster"
	"github.com/gorilla/mux"
)

type Handler struct {
	ClusterManager *cluster.ClusterManager
}

func NewHandler(cm *cluster.ClusterManager) *Handler {
	return &Handler{ClusterManager: cm}
}

func (h *Handler) AddApp(w http.ResponseWriter, r *http.Request) {
	var req api.AppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	err := h.ClusterManager.DeployApp(ctx, &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *Handler) GetAppStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	ctx := r.Context()
	statuses, err := h.ClusterManager.GetAppStatus(ctx, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := json.NewEncoder(w).Encode(statuses); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) GetAllAppsStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	allStatuses, err := h.ClusterManager.GetAllAppsStatus(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	response := api.AllAppsStatus{Apps: allStatuses}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
