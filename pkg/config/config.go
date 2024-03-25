package config

type Config struct {
	Name                  string `yaml:"name" json:"name"`
	URL                   string `yaml:"url" json:"url"`
	URLparsed             string
	RequireAuthentication bool              `yaml:"requireAuthentication" json:"requireAuthentication"`
	AuthenticationMethod  string            `yaml:"authenticationMethod" json:"authenticationMethod"`
	PollingIntervalHours  int               `yaml:"pollingIntervalHours" json:"pollingIntervalHours"`
	AdditionalVariables   map[string]string `yaml:"additionalVariables" json:"additionalVariables"`
}
