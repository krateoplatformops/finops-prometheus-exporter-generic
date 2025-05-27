package utils

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	finopsdatatypes "github.com/krateoplatformops/finops-data-types/api/v1"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

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
			if strings.EqualFold(value, toFind) {
				return i, nil
			}
		}
	}
	return -1, errors.New(toFind + " not found")
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

// replaceVariables replaces all variables in the format <variable> with their values
// from the additionalVariables map or from environment variables if the variable name is uppercase
func ReplaceVariables(text string, additionalVariables map[string]string) string {
	regex, _ := regexp.Compile("<.*?>")
	toReplaceRange := regex.FindStringIndex(text)

	for toReplaceRange != nil {
		// Extract variable name without the < > brackets
		varName := text[toReplaceRange[0]+1 : toReplaceRange[1]-1]

		// Get replacement value from additionalVariables
		varToReplace := additionalVariables[varName]

		// If the variable name is all uppercase, get value from environment
		if varToReplace == strings.ToUpper(varToReplace) {
			varToReplace = os.Getenv(varToReplace)
		}

		// Replace the variable in the text
		text = strings.Replace(text, text[toReplaceRange[0]:toReplaceRange[1]], varToReplace, -1)

		// Find next variable
		toReplaceRange = regex.FindStringIndex(text)
	}

	return text
}
