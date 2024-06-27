package main

import (
	"log"
	"net/http"

	"github.com/SepehrNoey/KaaS/pkg/cluster"
	"github.com/SepehrNoey/KaaS/pkg/handlers"
	"github.com/gorilla/mux"
)

func main() {
	cm, err := cluster.NewClusterManager()
	if err != nil {
		log.Fatalf("Failed to create cluster manager: %v", err)
	}

	h := handlers.NewHandler(cm)

	router := mux.NewRouter()
	router.HandleFunc("/api/apps/", h.AddApp).Methods("POST")
	router.HandleFunc("/api/apps/{name}", h.GetAppStatus).Methods("GET")
	router.HandleFunc("/api/apps/", h.GetAllAppsStatus).Methods("GET")

	log.Println("Starting server on :2024")
	if err := http.ListenAndServe(":2024", router); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
