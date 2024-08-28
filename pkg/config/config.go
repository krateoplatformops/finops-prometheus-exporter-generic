package config

import finopsDataTypes "github.com/krateoplatformops/finops-data-types/api/v1"

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
	ResourcesRef []finopsDataTypes.ObjectRef `json:"resourcesRef" yaml:"resourcesRef"`
}

type ResourceConfigSpec struct {
	ResourceFocusName string                      `json:"resourceFocusName" yaml:"resourceFocusName"`
	MetricsRef        []finopsDataTypes.ObjectRef `json:"metricsRef" yaml:"metricsRef"`
}

type MetricConfigSpec struct {
	MetricName string   `json:"metricName" yaml:"metricName"`
	Endpoint   Endpoint `json:"endpoint" yaml:"endpoint"`
	Interval   string   `json:"interval" yaml:"interval"`
	Timespan   string   `json:"timespan" yaml:"timespan"`
}

type Endpoint struct {
	ResourcePrefix string `json:"resourcePrefix" yaml:"resourcePrefix"`
	ResourceSuffix string `json:"resourceSuffix" yaml:"resourceSuffix"`
}
