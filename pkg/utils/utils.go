package utils

import (
	"context"
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"strings"

	"prometheus-exporter-generic/pkg/config"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var ResourceIds []string

func StartNewExporters(config config.Config) error {
	namespace := os.Getenv("NAMESPACE")
	inClusterConfig, err := rest.InClusterConfig()
	if err != nil {
		return err
	}

	clientset, err := kubernetes.NewForConfig(inClusterConfig)
	if err != nil {
		return err
	}

	// Get the original ExporterScraperConfig that generated this exporter
	// This is used to set the proper owner and get the scraper configuration
	originalExporterScraperConfigData, _ := clientset.RESTClient().Get().
		AbsPath("/apis/finops.krateo.io/v1").
		Namespace(namespace).
		Resource("exporterscraperconfigs").
		Name(strings.Split(os.Getenv("DEPLOYMENT"), "-deployment")[0]).
		DoRaw(context.TODO())
	var originalExporterScraperConfig ExporterScraperConfig
	_ = json.Unmarshal(originalExporterScraperConfigData, &originalExporterScraperConfig)

	// For each resourceId found in the FOCUS report
	for i, resourceId := range ResourceIds {
		// The name for the ExporterScraperConfig to be created now
		name := "exporterscraperconfig-" + strings.TrimSuffix(config.Name, "-exporter") + "-res" + strconv.FormatInt(int64(i), 10)
		// Compute the URL starting from this exporter's URL
		urlDomainRegex := regexp.MustCompile(`^(?:https?:\/\/)?(?:[^@\/\n]+@)?(?:www\.)?([^:\/?\n]+)(:[0-9]{4,4}){0,1}`)
		urlsParts := urlDomainRegex.FindAllString(config.URLparsed, -1)
		url := strings.Join(urlsParts, "") + resourceId

		// If the URL contains "azure", auto complete with additional (temporary) information
		switch {
		case strings.Contains(config.Name, "azure"):
			url += "/providers/microsoft.insights/metrics?api-version=2023-10-01&metricnames=Percentage%20CPU&timespan=2024-01-01T00:00:00Z/2024-03-01T00:00:00Z"
		}

		// Check if the ExporterScraperConfig already exists
		jsonData, _ := clientset.RESTClient().Get().
			AbsPath("/apis/finops.krateo.io/v1").
			Namespace(namespace).
			Resource("exporterscraperconfigs").
			Name(name).
			DoRaw(context.TODO())

		var crdResponse ExporterScraperConfig
		_ = json.Unmarshal(jsonData, &crdResponse)

		// If it does not exist, build it
		if crdResponse.Status == "Failure" {
			exporterScraperConfig := ExporterScraperConfig{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ExporterScraperConfig",
					APIVersion: "finops.krateo.io/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "finops.krateo.io/v1",
							Kind:       "ExporterScraperConfig",
							Name:       originalExporterScraperConfig.ObjectMeta.Name,
							UID:        originalExporterScraperConfig.ObjectMeta.UID,
						},
					},
				},
				Spec: ExporterScraperConfigSpec{
					ExporterConfig: ExporterConfig{
						Name:                  name,
						URL:                   "@RES:" + url,
						RequireAuthentication: config.RequireAuthentication,
						AuthenticationMethod:  config.AuthenticationMethod,
						PollingIntervalHours:  config.PollingIntervalHours,
						AdditionalVariables:   config.AdditionalVariables,
					},
					ScraperConfig: ScraperConfig{
						TableName:            originalExporterScraperConfig.Spec.ScraperConfig.TableName + "_res",
						PollingIntervalHours: originalExporterScraperConfig.Spec.ScraperConfig.PollingIntervalHours,
						ScraperDatabaseConfigRef: ScraperDatabaseConfigRef{
							Name:      originalExporterScraperConfig.Spec.ScraperConfig.ScraperDatabaseConfigRef.Name,
							Namespace: originalExporterScraperConfig.Spec.ScraperConfig.ScraperDatabaseConfigRef.Namespace,
						},
					},
				},
			}

			jsonData, err = json.Marshal(exporterScraperConfig)
			if err != nil {
				return err
			}
			// Create the object in the cluster
			_, err := clientset.RESTClient().Post().
				AbsPath("/apis/finops.krateo.io/v1").
				Namespace(namespace).
				Resource("exporterscraperconfigs").
				Name(name).
				Body(jsonData).
				DoRaw(context.TODO())

			if err != nil {
				return err
			}
		}
	}
	return nil
}

type ExporterScraperConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Status            string                    `json:"status"`
	Spec              ExporterScraperConfigSpec `json:"spec,omitempty"`
}

type ExporterScraperConfigSpec struct {
	ExporterConfig ExporterConfig `yaml:"exporterConfig" json:"exporterConfig"`
	ScraperConfig  ScraperConfig  `yaml:"scraperConfig" json:"scraperConfig"`
}

type ExporterConfig struct {
	Name                  string            `yaml:"name" json:"name"`
	URL                   string            `yaml:"url" json:"url"`
	RequireAuthentication bool              `yaml:"requireAuthentication" json:"requireAuthentication"`
	AuthenticationMethod  string            `yaml:"authenticationMethod" json:"authenticationMethod"`
	PollingIntervalHours  int               `yaml:"pollingIntervalHours" json:"pollingIntervalHours"`
	AdditionalVariables   map[string]string `yaml:"additionalVariables" json:"additionalVariables"`
}

type ScraperConfig struct {
	TableName            string `yaml:"tableName" json:"tableName"`
	PollingIntervalHours int    `yaml:"pollingIntervalHours" json:"pollingIntervalHours"`
	// +optional
	Url                      string                   `yaml:"url" json:"url"`
	ScraperDatabaseConfigRef ScraperDatabaseConfigRef `yaml:"scraperDatabaseConfigRef" json:"scraperDatabaseConfigRef"`
}

type ScraperDatabaseConfigRef struct {
	Name      string `yaml:"name" json:"name"`
	Namespace string `yaml:"namespace" json:"namespace"`
}
