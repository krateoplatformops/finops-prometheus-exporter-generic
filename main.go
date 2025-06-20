package main

import (
	"bytes"
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

func makeAPIRequest(config finopsdatatypes.ExporterScraperConfig, endpoint *httpcall.Endpoint) []byte {
	res := &http.Response{StatusCode: 500}
	err_call := fmt.Errorf("")

	for ok := true; ok; ok = (err_call != nil || res.StatusCode != 200) {
		httpClient, err := httpcall.HTTPClientForEndpoint(endpoint)
		if err != nil {
			log.Logger.Warn().Err(err).Msg("error while creating HTTP client")
		}

		res, err_call = httpcall.Do(context.TODO(), httpClient, httpcall.Options{
			API:      &config.Spec.ExporterConfig.API,
			Endpoint: endpoint,
		})

		if err_call == nil && res.StatusCode != 200 {
			log.Warn().Msgf("Received status code %d", res.StatusCode)
			bodyData, _ := io.ReadAll(res.Body)
			log.Warn().Msgf("Body %s", string(bodyData))
		} else {
			log.Logger.Warn().Err(err_call).Msg("error occurred while making API call")
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
	if err != nil {
		log.Logger.Warn().Err(err).Msg("an error has occured while reading response body")
	}

	log.Logger.Info().Msg("Trying to parse data as JSON")
	jsonDataParsed, err := utils.TryParseResponseAsFocusJSON(utils.TrapBOM(data))
	if err != nil {
		return utils.TrapBOM(data)
	} else {
		return jsonDataParsed
	}
}

func getRecordsFromFile(data []byte) [][]string {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.LazyQuotes = true

	records, err := reader.ReadAll()
	if err != nil {
		log.Logger.Warn().Err(err).Msg("error while reading file")
		return nil
	}

	return records
}

func updatedMetrics(registry *prometheus.Registry, prometheusMetrics map[string]recordGaugeCombo) {
	for {
		config, endpoint, err := ParseConfigFile("/config/config.yaml")
		if err != nil {
			log.Logger.Error().Err(err).Msg("error while parsing configuration, trying again in 5s...")
			time.Sleep(5 * time.Second)
			continue
		}
		data := makeAPIRequest(config, endpoint)
		records := getRecordsFromFile(data)

		// Obtain various indexes
		// BilledCost for value of metric
		billedCostIndex, err := utils.GetIndexOf(records, "BilledCost")
		if err != nil {
			log.Logger.Warn().Err(err).Msg("error while selecting column BilledCost, retrying...")
			time.Sleep(5 * time.Second)
			continue
		}

		notFound := true
		log.Info().Msgf("Analyzing %d records...", len(records))
		for i, record := range records {
			// Skip header line
			if i == 0 {
				continue
			}

			notFound = true
			if _, ok := prometheusMetrics[strings.Join(record, " ")]; ok {
				metricValue, err := strconv.ParseFloat(record[billedCostIndex], 64)
				if err != nil {
					log.Logger.Warn().Err(err).Msgf("skipping this record for this iteration, error while parsing metric value: %s", record[billedCostIndex])
					continue
				}
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
				if err != nil {
					log.Logger.Warn().Err(err).Msgf("skipping this record for this iteration, error while parsing metric value: %s", records[i][billedCostIndex])
					continue
				}
				newMetricsRow.Set(metricValue)
				prometheusMetrics[strings.Join(record, " ")] = recordGaugeCombo{record: record, gauge: newMetricsRow, thisIteration: true}
				registry.MustRegister(newMetricsRow)
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
		log.Debug().Msgf("Polling interval set to %s, starting sleep...", config.Spec.ExporterConfig.PollingInterval.Duration.String())
		time.Sleep(config.Spec.ExporterConfig.PollingInterval.Duration)
	}
}

func main() {
	registry := prometheus.NewRegistry()
	go updatedMetrics(registry, map[string]recordGaugeCombo{})

	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})

	http.Handle("/metrics", handler)
	http.ListenAndServe(":2112", nil)
}
