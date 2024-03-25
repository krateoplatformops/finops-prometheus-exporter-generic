# operator-exporter
This repository is part of a wider exporting architecture for the FinOps Cost and Usage Specification (FOCUS). This component is tasked with exporting in the Prometheus format a standard FOCUS report. The report is obtained from a file mounted inside the container in "/config/config.yaml". The exporter runs on the port 2112. If it detects resources of the type "Virtual Machine", it also creates new resources for the operator-exporter, to bootstrap a new exporting pipeline.

## Dependencies
To run this repository in your Kubernetes cluster, you need to have the following images in the same container registry:
 - operator-exporter
 - prometheus-exporter-generic
 - prometheus-resource-exporter-azure

## Configuration
To start the exporting process, see the "config-sample.yaml" file.

This container is automatically started by the operator-exporter.

## Installation
To build the executable: 
```
make build REPO=<your-registry-here>
```

To build and push the Docker images:
```
make container REPO=<your-registry-here>
```
