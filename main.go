package main

import (
	"crypto/tls"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/krateoplatformops/finops-prometheus-exporter-generic/pkg/utils"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"

	finopsDataTypes "github.com/krateoplatformops/finops-data-types/api/v1"
)

type recordGaugeCombo struct {
	record []string
	gauge  prometheus.Gauge
}

/*
* Parse the given configuration file and unmarhsal it into the "config.Config" data type.
* The configuration struct is an array of TargetAPI structs to allow the user to define multiple end-points for exporting.
* @param file The path to the configuration file
 */
func ParseConfigFile(file string) (finopsDataTypes.ExporterScraperConfig, error) {
	fileReader, err := os.OpenFile(file, os.O_RDONLY, 0600)
	if err != nil {
		return finopsDataTypes.ExporterScraperConfig{}, err
	}
	defer fileReader.Close()
	data, err := io.ReadAll(fileReader)
	if err != nil {
		return finopsDataTypes.ExporterScraperConfig{}, err
	}

	parse := finopsDataTypes.ExporterScraperConfig{}

	err = yaml.Unmarshal(data, &parse)
	if err != nil {
		return finopsDataTypes.ExporterScraperConfig{}, err
	}

	regex, _ := regexp.Compile("<.*?>")
	newURL := parse.Spec.ExporterConfig.Url
	toReplaceRange := regex.FindStringIndex(newURL)
	for toReplaceRange != nil {
		// Use the indexes of the match of the regex to replace the URL with the value of the additional variable from the config file
		// The replacement has +1/-1 on the indexes to remove the < and > from the string to use as key in the config map
		// If the replacement contains ONLY uppercase letters, it is taken from environment variables
		varToReplace := parse.Spec.ExporterConfig.AdditionalVariables[newURL[toReplaceRange[0]+1:toReplaceRange[1]-1]]
		if varToReplace == strings.ToUpper(varToReplace) {
			varToReplace = os.Getenv(varToReplace)
		}
		newURL = strings.Replace(newURL, newURL[toReplaceRange[0]:toReplaceRange[1]], varToReplace, -1)
		toReplaceRange = regex.FindStringIndex(newURL)
	}
	parse.Spec.ExporterConfig.UrlParsed = newURL
	return parse, nil
}

/*
* This function makes the API request to download the FOCUS csv file according to the given configuration.
* @param targetAPI the configuration for the API request
* @return the name of the saved file
 */
func makeAPIRequest(config finopsDataTypes.ExporterScraperConfig, fileName string) {
	requestURL := fmt.Sprintf(config.Spec.ExporterConfig.UrlParsed)
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	request, err := http.NewRequest(http.MethodGet, requestURL, nil)
	fatal(err)

	log.Logger.Info().Msg(requestURL)

	if config.Spec.ExporterConfig.RequireAuthentication {
		switch config.Spec.ExporterConfig.AuthenticationMethod {
		case "bearer-token":
			token, err := utils.GetBearerTokenSecret(config)
			if err != nil {
				fatal(err)
			}
			request.Header.Set("Authorization", "Bearer "+token)
		case "cert-file":
			data, err := os.ReadFile(config.Spec.ExporterConfig.AdditionalVariables["certFilePath"])
			if err != nil {
				log.Logger.Info().Msg("There has been an error reading the cert-file")
				return
			}
			request.Header.Set("Authorization", "Bearer "+string(data))
		}
	}

	res, err := http.DefaultClient.Do(request)
	fatal(err)

	defer res.Body.Close()

	if res.StatusCode == 400 {
		res, err = http.DefaultClient.Do(request)
		fatal(err)
	}

	if res.StatusCode == 202 {
		res.Body.Close()
		secondsToSleep, _ := strconv.ParseInt(res.Header.Get("Retry-after"), 10, 64)
		time.Sleep(time.Duration(secondsToSleep) * time.Second)
		res, err = http.Get(res.Header.Get("Location"))
		fatal(err)

		var data map[string]string
		err = json.NewDecoder(res.Body).Decode(&data)
		fatal(err)
		res.Body.Close()
		res, err = http.Get(data["downloadUrl"])
		fatal(err)
	}
	data, err := io.ReadAll(res.Body)
	fatal(err)

	log.Logger.Info().Msg("Trying to parse data as JSON")
	jsonDataParsed, err := utils.TryParseResponseAsFocusJSON(utils.TrapBOM(data))
	if err != nil {
		err = os.WriteFile(fmt.Sprintf("/temp/%s.dat", fileName), utils.TrapBOM(data), 0644)
		fatal(err)
	} else {
		err = os.WriteFile(fmt.Sprintf("/temp/%s.dat", fileName), jsonDataParsed, 0644)
		if err != nil {
			fatal(err)
		}

	}
}

/*
* This function reads the given csv file and returns the record list.
* @param fileName the name of the FOCUS csv file
* @return csv file as a 2D array of strings
 */
func getRecordsFromFile(fileName string) [][]string {
	file, err := os.Open(fmt.Sprintf("/temp/%s.dat", fileName))
	fatal(err)

	defer file.Close()

	reader := csv.NewReader(file)

	records, err := reader.ReadAll()
	fatal(err)

	return records
}

/*
* This function creates and maintains the prometheus gauges. Periodically, it updates the records csv file and checks if there are new rows to add to the registry.
* @param targetAPI the configuration for the API request
* @param registry the prometheus registry to add the gauges to
* @param prometheusMetrics the array of structs that contain gauges and the record the gauge was created from (to check when there are new records if it has already been created)
 */
func updatedMetrics(config finopsDataTypes.ExporterScraperConfig, useConfig bool, registry *prometheus.Registry, prometheusMetrics map[string]recordGaugeCombo) {
	for {
		attemptResourceExportersInThisIteration := true
		fileName := ""
		if config.Spec.ExporterConfig.Provider.Name != "" {
			fileName = config.Spec.ExporterConfig.Provider.Name
		} else {
			fileName = "download"
		}
		if useConfig {
			makeAPIRequest(config, fileName)
		}
		records := getRecordsFromFile(fileName)

		resourceTypes := []string{}
		if config.Spec.ExporterConfig.Provider.Name != "" {
			var err error
			resourceTypes, err = utils.InitializeResourcesWithProvider(config)
			if err != nil {
				log.Logger.Warn().Err(err).Msg("error while retrieving provider name, continuing...")
				attemptResourceExportersInThisIteration = false
			}
		}

		// Obtain various indexes
		// BilledCost for value of metric
		// ResourceType to check if additional exporters need to be started
		billedCostIndex, err := utils.GetIndexOf(records, "BilledCost")
		if err != nil {
			log.Logger.Warn().Err(err).Msg("error while selecting column BilledCost, retrying...")
			continue
		}
		resourceTypeIndex := -1
		if attemptResourceExportersInThisIteration {
			resourceTypeIndex, err = utils.GetIndexOf(records, "ResourceType")
			if err != nil {
				log.Logger.Warn().Err(err).Msg("error while selecting column ResourceType, retrying...")
				continue
			}
		}

		notFound := true
		for i, record := range records {
			// Skip header line
			if i == 0 {
				continue
			}
			if attemptResourceExportersInThisIteration {
				for _, resourceName := range resourceTypes {
					if record[resourceTypeIndex] == resourceName {
						resourceIdIndex, err := utils.GetIndexOf(records, "ResourceId")
						if err != nil {
							log.Logger.Warn().Err(err).Msg("error while selecting column ResourceId, retrying...")
							continue
						}
						found := false
						for _, elem := range utils.ResourceIdTypeComboList {
							if strings.EqualFold(record[resourceIdIndex], elem.ResourceId) {
								found = true
							}
						}
						if !found {
							utils.ResourceIdTypeComboList = append(utils.ResourceIdTypeComboList, utils.ResourceIdTypeCombo{ResourceId: record[resourceIdIndex], ResourceType: resourceName})
						}
					}
				}
			}

			notFound = true
			if _, ok := prometheusMetrics[strings.Join(record, " ")]; ok {
				metricValue, err := strconv.ParseFloat(record[billedCostIndex], 64)
				fatal(err)
				prometheusMetrics[strings.Join(record, " ")].gauge.Set(metricValue)
				notFound = false

			}
			if notFound {
				labels := prometheus.Labels{}
				for j, value := range record {
					if strings.Contains(records[0][j], "x_") {
						continue
					}
					if !strings.Contains(records[0][j], "Tags") {
						labels[records[0][j]] = value
					} else {
						replacer := strings.NewReplacer("{", "", "}", "", "=", ":", ",", ";", "\"", "")
						labels[records[0][j]] = replacer.Replace(value)
					}
				}

				newMetricsRow := promauto.NewGauge(prometheus.GaugeOpts{
					Name:        fmt.Sprintf("billed_cost_%s_%d", strings.ReplaceAll(config.Spec.ExporterConfig.Provider.Name, "-", "_"), i),
					ConstLabels: labels,
				})
				metricValue, err := strconv.ParseFloat(records[i][billedCostIndex], 64)
				fatal(err)
				newMetricsRow.Set(metricValue)
				prometheusMetrics[strings.Join(record, " ")] = recordGaugeCombo{record: record, gauge: newMetricsRow}
				registry.MustRegister(newMetricsRow)
			}
		}

		if attemptResourceExportersInThisIteration {
			err = utils.StartNewExporters(config)
			if err != nil {
				log.Logger.Warn().Err(err).Msg("error while starting resource exporters, continuing...")
			}
		}
		time.Sleep(time.Duration(config.Spec.ExporterConfig.PollingIntervalHours) * time.Hour)
	}
}

func main() {
	var err error
	config := finopsDataTypes.ExporterScraperConfig{}
	useConfig := true
	if len(os.Args) <= 1 {
		config, err = ParseConfigFile("/config/config.yaml")
		fatal(err)
		log.Logger.Error().Msg("error while parsing configuration, exiting")
		return
	} else {
		useConfig = false
		config.Spec.ExporterConfig.Provider.Name = os.Args[1]
		config.Spec.ExporterConfig.PollingIntervalHours = 1
	}

	registry := prometheus.NewRegistry()

	go updatedMetrics(config, useConfig, registry, map[string]recordGaugeCombo{})

	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})

	http.Handle("/metrics", handler)
	http.ListenAndServe(":2112", nil)
}

func fatal(err error) {
	if err != nil {
		log.Logger.Warn().Err(err).Msg("an error has occured, continuing...")
	}
}
