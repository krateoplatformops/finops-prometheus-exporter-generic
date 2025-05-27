package config

import finopsdatatypes "github.com/krateoplatformops/finops-data-types/api/v1"

type Endpoint struct {
	ResourcePrefixAPI *finopsdatatypes.API `json:"resourcePrefixAPI" yaml:"resourcePrefixAPI"`
	ResourceSuffix    string               `json:"resourceSuffix" yaml:"resourceSuffix"`
}
