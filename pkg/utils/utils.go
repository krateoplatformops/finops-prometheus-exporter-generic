package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	configPackage "github.com/krateoplatformops/finops-prometheus-exporter-generic/pkg/config"

	operatorPackage "github.com/krateoplatformops/finops-operator-exporter/api/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	ResourceIds []string
	metricName  = url.QueryEscape("Percentage CPU")
	timespan    = "month"
	interval    = "PT15M"
)

func StartNewExporters(config operatorPackage.ExporterScraperConfig) error {
	inClusterConfig, err := rest.InClusterConfig()
	if err != nil {
		return err
	}

	clientset, err := kubernetes.NewForConfig(inClusterConfig)
	if err != nil {
		return err
	}

	// For each resourceId found in the FOCUS report
	for i, resourceId := range ResourceIds {
		// The name for the ExporterScraperConfig to be created now
		name := "exporterscraperconfig-" + strings.TrimSuffix(config.Spec.ExporterConfig.Name, "-exporter") + "-res" + strconv.FormatInt(int64(i), 10)
		// Compute the URL starting from this exporter's URL
		urlDomainRegex := regexp.MustCompile(`^(?:https?:\/\/)?(?:[^@\/\n]+@)?(?:www\.)?([^:\/?\n]+)(:[0-9]{4,4}){0,1}`)
		urlsParts := urlDomainRegex.FindAllString(config.Spec.ExporterConfig.UrlParsed, -1)
		url := strings.Join(urlsParts, "") + resourceId

		// If the URL contains "azure", auto complete with additional (temporary) information
		switch {
		case strings.Contains(config.Spec.ExporterConfig.Name, "azure"):
			url += fmt.Sprintf("/providers/microsoft.insights/metrics?api-version=2023-10-01&metricnames=%s&timespan=%s&interval=%s", metricName, computeTimespan(timespan), interval)
		}

		// Check if the ExporterScraperConfig already exists
		jsonData, _ := clientset.RESTClient().Get().
			AbsPath("/apis/finops.krateo.io/v1").
			Namespace(config.ObjectMeta.Namespace).
			Resource("exporterscraperconfigs").
			Name(name).
			DoRaw(context.TODO())

		// Check if the CR already exists
		var crdResponse configPackage.Kind
		_ = json.Unmarshal(jsonData, &crdResponse)
		// If it does not exist, build it
		if crdResponse.Kind == "Status" && crdResponse.Status == "Failure" {
			exporterScraperConfig := operatorPackage.ExporterScraperConfig{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ExporterScraperConfig",
					APIVersion: "finops.krateo.io/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: config.ObjectMeta.Namespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "finops.krateo.io/v1",
							Kind:       "ExporterScraperConfig",
							Name:       config.ObjectMeta.Name,
							UID:        config.ObjectMeta.UID,
						},
					},
				},
				Spec: operatorPackage.ExporterScraperConfigSpec{
					ExporterConfig: operatorPackage.ExporterConfig{
						Name:                  name,
						Url:                   "@RES:" + url,
						RequireAuthentication: config.Spec.ExporterConfig.RequireAuthentication,
						AuthenticationMethod:  config.Spec.ExporterConfig.AuthenticationMethod,
						PollingIntervalHours:  config.Spec.ExporterConfig.PollingIntervalHours,
						AdditionalVariables:   config.Spec.ExporterConfig.AdditionalVariables,
					},
					ScraperConfig: operatorPackage.ScraperConfig{
						TableName:            config.Spec.ScraperConfig.TableName + "_res",
						PollingIntervalHours: config.Spec.ScraperConfig.PollingIntervalHours,
						ScraperDatabaseConfigRef: operatorPackage.ScraperDatabaseConfigRef{
							Name:      config.Spec.ScraperConfig.ScraperDatabaseConfigRef.Name,
							Namespace: config.Spec.ScraperConfig.ScraperDatabaseConfigRef.Namespace,
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
				Namespace(config.ObjectMeta.Namespace).
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

func computeTimespan(timespanName string) string {
	dateFormat := "yyyy-mm-dd"
	switch timespanName {
	case "day":
		interval = "PT5M"
		return time.Now().Format(dateFormat) + "/" + time.Now().AddDate(0, 0, -1).Format(dateFormat)
	case "month":
		interval = "PT15M"
		return time.Now().Format(dateFormat) + "/" + time.Now().AddDate(0, -1, 0).Format(dateFormat)
	case "year":
		interval = "PT6H"
		return time.Now().Format(dateFormat) + "/" + time.Now().AddDate(-1, 0, 0).Format(dateFormat)
	}
	interval = "PT15M"
	return time.Now().Format(dateFormat) + "/" + time.Now().AddDate(0, -1, 0).Format(dateFormat)
}
