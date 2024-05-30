package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
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
		fmt.Println("Parsing failed:", err)
		fmt.Println(string(jsonData))
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
