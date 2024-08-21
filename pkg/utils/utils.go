package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	urlPackage "net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	configPackage "github.com/krateoplatformops/finops-prometheus-exporter-generic/pkg/config"

	operatorPackage "github.com/krateoplatformops/finops-operator-exporter/api/v1"
	operatorPackageFocus "github.com/krateoplatformops/finops-operator-focus/api/v1"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func StartNewExporters(config operatorPackage.ExporterScraperConfig) error {
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
					// Compute the URL starting from this exporter's URL
					urlDomainRegex := regexp.MustCompile(`^(?:https?:\/\/)?(?:[^@\/\n]+@)?(?:www\.)?([^:\/?\n]+)(:[0-9]{4,4}){0,1}`)
					urlsParts := urlDomainRegex.FindAllString(config.Spec.ExporterConfig.UrlParsed, -1)
					url := strings.Join(urlsParts, "") + resourceId

					additionalVariables := config.Spec.ExporterConfig.AdditionalVariables
					additionalVariables["ResourceId"] = resourceId
					fmt.Println("new additional variables\n", additionalVariables)

					url += fmt.Sprintf(metric.Endpoint.ResourceSuffix, urlPackage.QueryEscape(metric.MetricName), computeTimespan(metric.Timespan), metric.Interval)

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
									Provider: operatorPackage.ObjectRef{
										Name:      config.Spec.ExporterConfig.Provider.Name,
										Namespace: config.Spec.ExporterConfig.Provider.Namespace,
									},
									Url:                   url,
									MetricType:            "resource",
									RequireAuthentication: config.Spec.ExporterConfig.RequireAuthentication,
									AuthenticationMethod:  config.Spec.ExporterConfig.AuthenticationMethod,
									PollingIntervalHours:  config.Spec.ExporterConfig.PollingIntervalHours,
									AdditionalVariables:   additionalVariables,
								},
								ScraperConfig: operatorPackage.ScraperConfig{
									TableName:            config.Spec.ScraperConfig.TableName + "_res",
									PollingIntervalHours: config.Spec.ScraperConfig.PollingIntervalHours,
									ScraperDatabaseConfigRef: operatorPackage.ObjectRef{
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
		return time.Now().Format(dateFormat) + "/" + time.Now().AddDate(0, 0, -1).Format(dateFormat)
	case "month":
		return time.Now().Format(dateFormat) + "/" + time.Now().AddDate(0, -1, 0).Format(dateFormat)
	case "year":
		return time.Now().Format(dateFormat) + "/" + time.Now().AddDate(-1, 0, 0).Format(dateFormat)
	}
	return time.Now().Format(dateFormat) + "/" + time.Now().AddDate(0, -1, 0).Format(dateFormat)
}

/*
* Function to remove the encoding bytes from a file.
* @param file The file to remove the encoding from.
 */
func TrapBOM(file []byte) []byte {
	return bytes.Trim(file, "\xef\xbb\xbf")
}

func TryParseResponseAsFocusJSON(jsonData []byte) ([]byte, error) {
	var focusConfigList operatorPackageFocus.FocusConfigList
	err := json.Unmarshal(jsonData, &focusConfigList)
	if err != nil {
		fmt.Println("\t", "Parsing failed:", err)
		//fmt.Println(string(jsonData))
		return []byte{}, err
	}

	outputStr := GetOutputStr(focusConfigList)

	return []byte(outputStr), nil
}

func GetOutputStr(configList operatorPackageFocus.FocusConfigList) string {
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
	fmt.Println(outputStr)
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

	tags, ok := value.([]operatorPackageFocus.TagsType)
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
	for i, value := range records[0] {
		if value == toFind {
			return i, nil
		}
	}
	return -1, errors.New(toFind + " not found")
}

func InitializeResourcesWithProvider(config operatorPackage.ExporterScraperConfig) ([]string, error) {
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

	fmt.Println("Found provider", config.Spec.ExporterConfig.Provider.Name)
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

		fmt.Println("Found resource", resourceConfigSpec.ResourceFocusName)
	}

	return resourceStrings, nil
}

func GetClientSet() (*kubernetes.Clientset, error) {
	inClusterConfig, err := rest.InClusterConfig()
	if err != nil {
		return &kubernetes.Clientset{}, err
	}

	inClusterConfig.APIPath = "/apis"
	inClusterConfig.GroupVersion = &operatorPackage.GroupVersion

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
		fmt.Println("\t", "Found metric", metricConfigSpec.MetricName, metricConfigSpec.Interval, metricConfigSpec.Timespan)
	}

	return result, nil
}

func GetBearerTokenSecret(config operatorPackage.ExporterScraperConfig) (string, error) {
	clientset, err := GetClientSet()
	if err != nil {
		return "", err
	}

	secret, err := clientset.CoreV1().Secrets(config.Spec.ExporterConfig.BearerToken.Namespace).Get(context.TODO(), config.Spec.ExporterConfig.BearerToken.Name, v1.GetOptions{})
	if err != nil {
		return "", err
	}
	return string(secret.Data["bearer-token"]), nil
}
