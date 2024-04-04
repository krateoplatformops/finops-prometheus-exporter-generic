package config

type Kind struct {
	Kind   string `json:"kind"`
	Status string `json:"status"`
}

/*
type ExporterScraperConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ExporterScraperConfigSpec `json:"spec,omitempty"`
}

type ExporterScraperConfigSpec struct {
	ExporterConfig ExporterConfig `yaml:"exporterConfig" json:"exporterConfig"`
	ScraperConfig  ScraperConfig  `yaml:"scraperConfig" json:"scraperConfig"`
}

type ExporterConfig struct {
	Name                  string `yaml:"name" json:"name"`
	Url                   string `yaml:"url" json:"url"`
	Urlparsed             string
	RequireAuthentication bool              `yaml:"requireAuthentication" json:"requireAuthentication"`
	AuthenticationMethod  string            `yaml:"authenticationMethod" json:"authenticationMethod"`
	PollingIntervalHours  int               `yaml:"pollingIntervalHours" json:"pollingIntervalHours"`
	AdditionalVariables   map[string]string `yaml:"additionalVariables" json:"additionalVariables"`
}

type ScraperConfig struct {
	TableName            string `yaml:"tableName" json:"tableName"`
	PollingIntervalHours int    `yaml:"pollingIntervalHours" json:"pollingIntervalHours"`
	// +optional
	Url                      string                   `yaml:"url" json:"url,omitempty"`
	ScraperDatabaseConfigRef ScraperDatabaseConfigRef `yaml:"scraperDatabaseConfigRef" json:"scraperDatabaseConfigRef"`
}

type ScraperDatabaseConfigRef struct {
	Name      string `yaml:"name" json:"name"`
	Namespace string `yaml:"namespace" json:"namespace"`
}
*/
