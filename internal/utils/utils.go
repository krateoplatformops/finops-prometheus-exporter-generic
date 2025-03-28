package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	urlPackage "net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	configPackage "github.com/krateoplatformops/finops-prometheus-exporter-generic/internal/config"
	"github.com/krateoplatformops/finops-prometheus-exporter-generic/internal/helpers/kube/httpcall"

	finopsdatatypes "github.com/krateoplatformops/finops-data-types/api/v1"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type ResourceIdTypeCombo struct {
	ResourceId   string
	ResourceType string
}

var (
	ResourceList            []configPackage.ResourceConfigSpec
	ResourceIdTypeComboList []ResourceIdTypeCombo
)

func StartNewExporters(config finopsdatatypes.ExporterScraperConfig, endpoint *httpcall.Endpoint) error {
	clientset, err := GetClientSet()
	if err != nil {
		return err
	}

	// For each resourceId found in the FOCUS report
	for i, resourceIdTypeCombo := range ResourceIdTypeComboList {
		resourceId := resourceIdTypeCombo.ResourceId
		for _, resource := range ResourceList {
			if resource.ResourceFocusName == resourceIdTypeCombo.ResourceType {
				metricsList, err := getMetricsList(clientset, resource)
				if err != nil {
					return err
				}

				for _, metric := range metricsList {
					// The name for the ExporterScraperConfig to be created now
					name := "exporterscraperconfig-" + strings.TrimSuffix(config.Spec.ExporterConfig.Provider.Name, "-exporter") + "-res" + strconv.FormatInt(int64(i), 10)
					additionalVariables := config.Spec.ExporterConfig.AdditionalVariables
					additionalVariables["ResourceId"] = resourceId

					var api finopsdatatypes.API
					if metric.Endpoint.ResourcePrefixAPI == nil {
						api = config.Spec.ExporterConfig.API
					} else {
						api = *metric.Endpoint.ResourcePrefixAPI
					}
					api.Path = resourceId + fmt.Sprintf(metric.Endpoint.ResourceSuffix, urlPackage.QueryEscape(metric.MetricName), computeTimespan(metric.Timespan), metric.Interval)
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
						exporterScraperConfig := finopsdatatypes.ExporterScraperConfig{
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
							Spec: finopsdatatypes.ExporterScraperConfigSpec{
								ExporterConfig: finopsdatatypes.ExporterConfigSpec{
									Provider: finopsdatatypes.ObjectRef{
										Name:      config.Spec.ExporterConfig.Provider.Name,
										Namespace: config.Spec.ExporterConfig.Provider.Namespace,
									},
									API:                 api,
									MetricType:          "resource",
									PollingInterval:     config.Spec.ExporterConfig.PollingInterval,
									AdditionalVariables: additionalVariables,
								},
								ScraperConfig: finopsdatatypes.ScraperConfigSpec{
									MetricType:      "resource",
									TableName:       config.Spec.ScraperConfig.TableName + "_res",
									PollingInterval: config.Spec.ScraperConfig.PollingInterval,
									ScraperDatabaseConfigRef: finopsdatatypes.ObjectRef{
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

					} else {
						continue
					}
				}
			}
		}
	}
	return nil
}

func computeTimespan(timespanName string) string {
	// Format: yyyy-mm-dd
	dateFormat := "2006-01-02"
	switch timespanName {
	case "day":
		return time.Now().AddDate(0, 0, -1).Format(dateFormat) + "/" + time.Now().Format(dateFormat)
	case "month":
		return time.Now().AddDate(0, -1, 0).Format(dateFormat) + "/" + time.Now().Format(dateFormat)
	case "year":
		return time.Now().AddDate(-1, 0, 0).Format(dateFormat) + "/" + time.Now().Format(dateFormat)
	}
	return time.Now().AddDate(0, -1, 0).Format(dateFormat) + "/" + time.Now().Format(dateFormat)
}

/*
* Function to remove the encoding bytes from a file.
* @param file The file to remove the encoding from.
 */
func TrapBOM(file []byte) []byte {
	return bytes.Trim(file, "\xef\xbb\xbf")
}

func TryParseResponseAsFocusJSON(jsonData []byte) ([]byte, error) {
	var focusConfigList finopsdatatypes.FocusConfigList
	err := json.Unmarshal(jsonData, &focusConfigList)
	if err != nil {
		log.Logger.Warn().Err(err).Msg("Parsing failed")
		//log.Logger.Info().Msg(string(jsonData))
		return []byte{}, err
	}

	outputStr := GetOutputStr(focusConfigList)

	return []byte(outputStr), nil
}

func GetOutputStr(configList finopsdatatypes.FocusConfigList) string {
	outputStr := ""
	for i, config := range configList.Items {
		v := reflect.ValueOf(config.Spec.FocusSpec)

		if i == 0 {
			for i := 0; i < v.NumField(); i++ {
				outputStr += v.Type().Field(i).Name + ","
			}
			outputStr = strings.TrimSuffix(outputStr, ",") + "\n"
		}

		for i := 0; i < v.NumField(); i++ {
			outputStr += GetStringValue(v.Field(i).Interface()) + ","
		}
		outputStr = strings.TrimSuffix(outputStr, ",") + "\n"
	}
	outputStr = strings.TrimSuffix(outputStr, "\n")
	log.Logger.Info().Msg(outputStr)
	return outputStr
}

func GetStringValue(value any) string {
	str, ok := value.(string)
	if ok {
		return str
	}

	integer, ok := value.(int)
	if ok {
		return strconv.FormatInt(int64(integer), 10)
	}

	integer64, ok := value.(int64)
	if ok {
		return strconv.FormatInt(integer64, 10)
	}

	resourceQuantity, ok := value.(resource.Quantity)
	if ok {
		return resourceQuantity.AsDec().String()
	}

	metav1Time, ok := value.(metav1.Time)
	if ok {
		return metav1Time.Format(time.RFC3339)
	}

	tags, ok := value.([]finopsdatatypes.TagsType)
	if ok {
		res := ""
		for _, tag := range tags {
			res += tag.Key + "=" + tag.Value + ";"
		}
		return strings.TrimSuffix(res, ";")
	}

	return ""
}

/*
* Given the records from the csv file, it returns the index of the "toFind" column.
* @param records The csv file as a 2D array of strings
* @param toFind the column to find
* @return the index of the "toFind" column
 */
func GetIndexOf(records [][]string, toFind string) (int, error) {
	log.Debug().Msgf("Looking for %s", toFind)
	if len(records) > 0 {
		for i, value := range records[0] {
			log.Debug().Msgf("Analyzing %s", value)
			if strings.ToLower(value) == strings.ToLower(toFind) {
				return i, nil
			}
		}
	}
	return -1, errors.New(toFind + " not found")
}

func InitializeResourcesWithProvider(config finopsdatatypes.ExporterScraperConfig) ([]string, error) {
	clientset, err := GetClientSet()
	if err != nil {
		return []string{}, err
	}

	jsonData, err := clientset.RESTClient().
		Get().
		AbsPath("/apis/finops.krateo.io/v1").
		Namespace(config.Spec.ExporterConfig.Provider.Namespace).
		Resource("providerconfigs").
		Name(config.Spec.ExporterConfig.Provider.Name).
		DoRaw(context.TODO())
	if err != nil {
		return []string{}, err
	}

	var providerConfig configPackage.ProviderConfig
	err = json.Unmarshal(jsonData, &providerConfig)
	if err != nil {
		return []string{}, err
	}

	providerConfigSpec := providerConfig.Spec

	log.Logger.Info().Msgf("Found provider %s", config.Spec.ExporterConfig.Provider.Name)
	resourceStrings := []string{}

	for _, resource := range providerConfigSpec.ResourcesRef {
		jsonData, err := clientset.RESTClient().
			Get().
			AbsPath("/apis/finops.krateo.io/v1").
			Namespace(resource.Namespace).
			Resource("resourceconfigs").
			Name(resource.Name).
			DoRaw(context.TODO())
		if err != nil {
			return []string{}, err
		}
		var resourceConfig configPackage.ResourceConfig
		err = json.Unmarshal(jsonData, &resourceConfig)
		if err != nil {
			return []string{}, err
		}
		resourceConfigSpec := resourceConfig.Spec
		resourceStrings = append(resourceStrings, resourceConfigSpec.ResourceFocusName)
		ResourceList = append(ResourceList, resourceConfigSpec)

		log.Logger.Info().Msgf("Found resource %s", resourceConfigSpec.ResourceFocusName)
	}

	return resourceStrings, nil
}

func GetClientSet() (*kubernetes.Clientset, error) {
	inClusterConfig, err := rest.InClusterConfig()
	if err != nil {
		return &kubernetes.Clientset{}, err
	}

	inClusterConfig.APIPath = "/apis"
	inClusterConfig.GroupVersion = &schema.GroupVersion{Group: "finops.krateo.io", Version: "v1"}

	clientset, err := kubernetes.NewForConfig(inClusterConfig)
	if err != nil {
		return &kubernetes.Clientset{}, err
	}
	return clientset, nil
}

func getMetricsList(clientset *kubernetes.Clientset, resource configPackage.ResourceConfigSpec) ([]configPackage.MetricConfigSpec, error) {
	result := []configPackage.MetricConfigSpec{}
	for _, metric := range resource.MetricsRef {
		jsonData, err := clientset.RESTClient().
			Get().
			AbsPath("/apis/finops.krateo.io/v1").
			Namespace(metric.Namespace).
			Resource("metricconfigs").
			Name(metric.Name).
			DoRaw(context.TODO())
		if err != nil {
			return result, err
		}
		var metricConfig configPackage.MetricConfig
		err = json.Unmarshal(jsonData, &metricConfig)
		if err != nil {
			return result, err
		}

		metricConfigSpec := metricConfig.Spec

		result = append(result, metricConfigSpec)
		log.Logger.Info().Msgf("\tFound metric %s, %s, %s", metricConfigSpec.MetricName, metricConfigSpec.Interval, metricConfigSpec.Timespan)
	}

	return result, nil
}
