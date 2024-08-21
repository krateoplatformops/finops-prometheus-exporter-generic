# FinOps Prometheus Exporter Generic (Costs)
This repository is part of the wider exporting architecture for the Krateo Composable FinOps and exports the API endpoints of FOCUS cost reports in the Prometheus format.

## Summary
1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Configuration](#configuration)

## Overview
This component is tasked with exporting in the Prometheus format a standard FOCUS report. The report is obtained from a file mounted inside the container in "/config/config.yaml". The exporter runs on the port 2112. For each resource, it checks the provider and resource CRs, and creates new CRs for the FinOps Operator Exporter to bootstrap a new exporting pipeline for usage metrics.

## Architecture
![Krateo Composable FinOps Prometheus Exporter Generic](resources/images/KCF-exporter.png)

## Configuration
This container is automatically started by the FinOps Operator Exporter.

To build the executable: 
```
make build REPO=<your-registry-here>
```

To build and push the Docker images:
```
make container REPO=<your-registry-here>
```

### Dependencies
To run this repository in your Kubernetes cluster, you need to have the following images in the same container registry:
 - finops-operator-exporter
 - finops-prometheus-exporter-generic
 - finops-prometheus-resource-exporter-azure

