package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/krateoplatformops/finops-prometheus-exporter-generic/internal/utils"
	"k8s.io/client-go/rest"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"

	finopsdatatypes "github.com/krateoplatformops/finops-data-types/api/v1"
	"github.com/krateoplatformops/finops-prometheus-exporter-generic/internal/helpers/kube/endpoints"
	"github.com/krateoplatformops/finops-prometheus-exporter-generic/internal/helpers/kube/httpcall"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type recordGaugeCombo struct {
	record        []string
	gauge         prometheus.Gauge
	thisIteration bool
}

func ParseConfigFile(file string) (finopsdatatypes.ExporterScraperConfig, *httpcall.Endpoint, error) {
	fileReader, err := os.OpenFile(file, os.O_RDONLY, 0600)
	if err != nil {
		return finopsdatatypes.ExporterScraperConfig{}, &httpcall.Endpoint{}, err
	}
	defer fileReader.Close()
	data, err := io.ReadAll(fileReader)
	if err != nil {
		return finopsdatatypes.ExporterScraperConfig{}, &httpcall.Endpoint{}, err
	}

	parse := finopsdatatypes.ExporterScraperConfig{}

	err = yaml.Unmarshal(data, &parse)
	if err != nil {
		return finopsdatatypes.ExporterScraperConfig{}, &httpcall.Endpoint{}, err
	}

	rc, _ := rest.InClusterConfig()
	endpoint, err := endpoints.Resolve(context.Background(), endpoints.ResolveOptions{
		RESTConfig: rc,
		API:        &parse.Spec.ExporterConfig.API,
	})
	if err != nil {
		return finopsdatatypes.ExporterScraperConfig{}, &httpcall.Endpoint{}, err
	}

	// Replace variables in server URL
	endpoint.ServerURL = utils.ReplaceVariables(endpoint.ServerURL, parse.Spec.ExporterConfig.AdditionalVariables)

	// Replace variables in API path
	parse.Spec.ExporterConfig.API.Path = utils.ReplaceVariables(parse.Spec.ExporterConfig.API.Path, parse.Spec.ExporterConfig.AdditionalVariables)

	return parse, endpoint, nil
}

func makeAPIRequest(config finopsdatatypes.ExporterScraperConfig, endpoint *httpcall.Endpoint, fileName string) {
	log.Logger.Info().Msgf("Request URL: %s", endpoint.ServerURL)

	res := &http.Response{StatusCode: 500}
	err_call := fmt.Errorf("")

	for ok := true; ok; ok = (err_call != nil || res.StatusCode != 200) {
		httpClient, err := httpcall.HTTPClientForEndpoint(endpoint)
		if err != nil {
			fatal(err)
		}

		res, err_call = httpcall.Do(context.TODO(), httpClient, httpcall.Options{
			API:      &config.Spec.ExporterConfig.API,
			Endpoint: endpoint,
		})

		if err == nil && res.StatusCode != 200 {
			log.Warn().Msgf("Received status code %d", res.StatusCode)
		} else {
			fatal(err)
		}
		log.Logger.Warn().Msgf("Retrying connection in 5s...")
		time.Sleep(5 * time.Second)

		log.Logger.Info().Msgf("Parsing Endpoint again...")
		rc, _ := rest.InClusterConfig()
		endpoint, err = endpoints.Resolve(context.Background(), endpoints.ResolveOptions{
			RESTConfig: rc,
			API:        &config.Spec.ExporterConfig.API,
		})
		if err != nil {
			continue
		}
		endpoint.ServerURL = utils.ReplaceVariables(endpoint.ServerURL, config.Spec.ExporterConfig.AdditionalVariables)
	}

	defer res.Body.Close()

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

func getRecordsFromFile(fileName string) [][]string {
	file, err := os.Open(fmt.Sprintf("/temp/%s.dat", fileName))
	fatal(err)

	defer file.Close()

	reader := csv.NewReader(file)
	reader.LazyQuotes = true

	records, err := reader.ReadAll()
	fatal(err)

	return records
}

func updatedMetrics(config finopsdatatypes.ExporterScraperConfig, endpoint *httpcall.Endpoint, useConfig bool, registry *prometheus.Registry, prometheusMetrics map[string]recordGaugeCombo) {
	for {
		attemptResourceExportersInThisIteration := true
		fileName := ""
		if config.Spec.ExporterConfig.Provider.Name != "" {
			fileName = config.Spec.ExporterConfig.Provider.Name
		} else {
			fileName = "download"
		}
		if useConfig {
			makeAPIRequest(config, endpoint, fileName)
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
		log.Info().Msgf("Analyzing %d records...", len(records))
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
				gaugeObj := prometheusMetrics[strings.Join(record, " ")]
				gaugeObj.gauge.Set(metricValue)
				gaugeObj.thisIteration = true
				prometheusMetrics[strings.Join(record, " ")] = gaugeObj
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
					Name:        "billed_cost",
					ConstLabels: labels,
				})
				metricValue, err := strconv.ParseFloat(records[i][billedCostIndex], 64)
				fatal(err)
				newMetricsRow.Set(metricValue)
				prometheusMetrics[strings.Join(record, " ")] = recordGaugeCombo{record: record, gauge: newMetricsRow, thisIteration: true}
				registry.MustRegister(newMetricsRow)
			}
		}

		if attemptResourceExportersInThisIteration {
			err = utils.StartNewExporters(config, endpoint)
			if err != nil {
				log.Logger.Warn().Err(err).Msg("error while starting resource exporters, continuing...")
			}
		}

		for key, gaugeObj := range prometheusMetrics {
			if !gaugeObj.thisIteration {
				registry.Unregister(gaugeObj.gauge)
				delete(prometheusMetrics, key)
			} else {
				gaugeObj.thisIteration = false
				prometheusMetrics[key] = gaugeObj
			}
		}
		time.Sleep(config.Spec.ExporterConfig.PollingInterval.Duration)
	}
}

func main() {
	var err error
	config := finopsdatatypes.ExporterScraperConfig{}
	endpoint := &httpcall.Endpoint{}
	useConfig := true
	if len(os.Args) <= 1 {
		config, endpoint, err = ParseConfigFile("/config/config.yaml")
		if err != nil {
			log.Logger.Error().Err(err).Msg("error while parsing configuration, exiting")
			return
		}
	} else {
		useConfig = false
		config.Spec.ExporterConfig.Provider.Name = os.Args[1]
		config.Spec.ExporterConfig.PollingInterval = metav1.Duration{Duration: 1 * time.Hour}
	}

	log.Debug().Msgf("Polling interval set to %s", config.Spec.ExporterConfig.PollingInterval.Duration.String())

	registry := prometheus.NewRegistry()

	go updatedMetrics(config, endpoint, useConfig, registry, map[string]recordGaugeCombo{})

	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})

	http.Handle("/metrics", handler)
	http.ListenAndServe(":2112", nil)
}

func fatal(err error) {
	if err != nil {
		log.Logger.Warn().Err(err).Msg("an error has occured, continuing...")
	}
}
