# FinOps Prometheus Exporter
This repository is part of the wider exporting architecture for the Krateo Composable FinOps and exports the API endpoints of FOCUS cost reports and usage metrics in the Prometheus format.

## Summary
1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Configuration](#configuration)

## Overview
This component is tasked with exporting in the Prometheus format a standard FOCUS report or usage metrics. The exporter runs on the port 2112.

## Architecture
![Krateo Composable FinOps Prometheus Exporter Generic](resources/images/KCF-exporter.png)

## Configuration
This container is automatically started by the FinOps Operator Exporter and you do not need to install it manually.

To build the executable: 
```
make build REPO=<your-registry-here>
```

To build and push the Docker images:
```
make container REPO=<your-registry-here>
```

