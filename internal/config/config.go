package config

import finopsdatatypes "github.com/krateoplatformops/finops-data-types/api/v1"

type Kind struct {
	Kind   string `json:"kind"`
	Status string `json:"status"`
}

type ProviderConfig struct {
	Spec ProviderConfigSpec `json:"spec" yaml:"spec"`
}

type ResourceConfig struct {
	Spec ResourceConfigSpec `json:"spec" yaml:"spec"`
}

type MetricConfig struct {
	Spec MetricConfigSpec `json:"spec" yaml:"spec"`
}

type ProviderConfigSpec struct {
	ResourcesRef []finopsdatatypes.ObjectRef `json:"resourcesRef" yaml:"resourcesRef"`
}

type ResourceConfigSpec struct {
	ResourceFocusName string                      `json:"resourceFocusName" yaml:"resourceFocusName"`
	MetricsRef        []finopsdatatypes.ObjectRef `json:"metricsRef" yaml:"metricsRef"`
}

type MetricConfigSpec struct {
	MetricName string   `json:"metricName" yaml:"metricName"`
	Endpoint   Endpoint `json:"endpoint" yaml:"endpoint"`
	Interval   string   `json:"interval" yaml:"interval"`
	Timespan   string   `json:"timespan" yaml:"timespan"`
}

type Endpoint struct {
	ResourcePrefixAPI *finopsdatatypes.API `json:"resourcePrefixAPI" yaml:"resourcePrefixAPI"`
	ResourceSuffix    string               `json:"resourceSuffix" yaml:"resourceSuffix"`
}
