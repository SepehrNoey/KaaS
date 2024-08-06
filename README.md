# Kaas App (Kubernetes as a Service)

## Overview
This project is a simple Kubernetes as a Service (Kaas) application. It provides an API that allows users to:
1. Deploy applications to a Kubernetes cluster.
2. Retrieve the status of a specific deployment.
3. Retrieve the statuses of all deployments.

Additionally, there is a self-service API that brings up a PostgreSQL database for further uses.

## Features
- **Deployment Management:** Easily deploy your applications to Kubernetes using a simple API.
- **Status Monitoring:** Check the status of individual deployments or all deployments at once.
- **Scaling:** Utilize Kubernetes Horizontal Pod Autoscaler (HPA) to manage application scaling.
- **Monitoring:** Integration with Prometheus and Grafana for monitoring and visualizing deployment metrics.
- **Persistence:** Uses PostgreSQL for storing deployment data.

## Tools and Technologies
- **Golang:** The main language used for development.
- **Kubernetes Client (Go):** Communicate with the Kubernetes cluster.
- **PostgreSQL:** Database for storing deployment information and provided via a self-service API.
- **Helm:** Manage Kubernetes applications.
- **Horizontal Pod Autoscaler (HPA):** Automatically scale the number of pods in a deployment.
- **Monitoring Probes:** Ensure applications are running smoothly, including liveness, readiness and startup.
- **Prometheus:** Monitoring system and time series database.
- **Grafana:** Analytics and monitoring dashboard for visualizing metrics.

## API Endpoints
1. **Deploy Application:** Allows users to deploy a new application to the Kubernetes cluster.
2. **Get Deployment Status:** Retrieve the current status of a specific deployment.
3. **Get All Deployment Statuses:** Retrieve the current statuses of all deployments.
4. **Deploy PostgreSQL Database:** Bring up a PostgreSQL database for further uses.
