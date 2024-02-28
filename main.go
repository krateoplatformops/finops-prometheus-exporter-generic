package main

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Name                  string            `yaml:"name" json:"name"`
	URL                   string            `yaml:"url" json:"url"`
	RequireAuthentication bool              `yaml:"requireAuthentication" json:"requireAuthentication"`
	AuthenticationMethod  string            `yaml:"authenticationMethod" json:"authenticationMethod"`
	PollingIntervalHours  int               `yaml:"pollingIntervalHours" json:"pollingIntervalHours"`
	AdditionalVariables   map[string]string `yaml:"additionalVariables" json:"additionalVariables"`
}

type ScraperDatabaseConfigRef struct {
	Name      string `yaml:"name" json:"name"`
	Namespace string `yaml:"namespace" json:"namespace"`
}

type recordGaugeCombo struct {
	record []string
	gauge  prometheus.Gauge
}

/*
* Parse the given configuration file and unmarhsal it into the "Config" data type.
* The configuration struct is an array of TargetAPI structs to allow the user to define multiple end-points for exporting.
* @param file The path to the configuration file
 */
func ParseConfigFile(file string) (Config, error) {
	fileReader, err := os.OpenFile(file, os.O_RDONLY, 0600)
	if err != nil {
		return Config{}, err
	}
	defer fileReader.Close()
	data, err := io.ReadAll(fileReader)
	if err != nil {
		return Config{}, err
	}

	parse := Config{}

	err = yaml.Unmarshal(data, &parse)
	if err != nil {
		return Config{}, err
	}

	regex, _ := regexp.Compile("<.*?>")
	newURL := parse.URL
	toReplaceRange := regex.FindStringIndex(newURL)
	for toReplaceRange != nil {
		// Use the indexes of the match of the regex to replace the URL with the value of the additional variable from the config file
		// The replacement has +1/-1 on the indexes to remove the < and > from the string to use as key in the config map
		// If the replacement contains ONLY uppercase letters, it is taken from environment variables
		varToReplace := parse.AdditionalVariables[newURL[toReplaceRange[0]+1:toReplaceRange[1]-1]]
		if varToReplace == strings.ToUpper(varToReplace) {
			varToReplace = os.Getenv(varToReplace)
		}
		newURL = strings.Replace(newURL, newURL[toReplaceRange[0]:toReplaceRange[1]], varToReplace, -1)
		toReplaceRange = regex.FindStringIndex(newURL)
	}
	parse.URL = newURL

	return parse, nil
}

/*
* Function to remove the encoding bytes from a file.
* @param file The file to remove the encoding from.
 */
func trapBOM(file []byte) []byte {
	return bytes.Trim(file, "\xef\xbb\xbf")
}

/*
* This function makes the API request to read the FOCUS csv file according to the given configuration.
* @param targetAPI the configuration for the API request
* @return the csv file as a 2D array of strings
 */
func makeAPIRequest(config Config) [][]string {
	requestURL := fmt.Sprintf(config.URL)
	request, err := http.NewRequest(http.MethodGet, requestURL, nil)
	fatal(err)

	if config.RequireAuthentication {
		switch config.AuthenticationMethod {
		case "bearer-token":
			request.Header.Set("Authorization", config.AdditionalVariables["authenticationToken"])
		}
	}

	res, err := http.Get(requestURL)
	fatal(err)

	defer res.Body.Close()

	data, err := io.ReadAll(res.Body)
	fatal(err)

	err = os.WriteFile(fmt.Sprintf("/temp/%s.dat", config.Name), trapBOM(data), 0644)
	fatal(err)

	file, err := os.Open(fmt.Sprintf("/temp/%s.dat", config.Name))
	fatal(err)

	defer file.Close()

	reader := csv.NewReader(file)

	records, err := reader.ReadAll()
	fatal(err)

	return records
}

/*
* Given the records from the csv file, it returns the index of the "BilledCost" column.
* @param records The csv file as a 2D array of strings
* @return the index of the "BilledCost" column
 */
func getBilledCostIndex(records [][]string) (int, error) {
	for i, value := range records[0] {
		if value == "BilledCost" {
			return i, nil
		}
	}
	return -1, errors.New("BilledCost not found")
}

/*
* This function creates and maintains the prometheus gauges. Periodically, it updates the records csv file and checks if there are new rows to add to the registry.
* @param targetAPI the configuration for the API request
* @param registry the prometheus registry to add the gauges to
* @param prometheusMetrics the array of structs that contain gauges and the record the gauge was created from (to check when there are new records if it has already been created)
 */
func updatedMetrics(config Config, registry *prometheus.Registry, prometheusMetrics []recordGaugeCombo) {
	for {
		records := makeAPIRequest(config)
		billedCostIndex, err := getBilledCostIndex(records)
		if err != nil {
			fmt.Println(err)
			continue
		}
		notFound := true
		for i, record := range records {
			// Skip header line
			if i == 0 {
				continue
			}

			notFound = true
			for _, metric := range prometheusMetrics {

				if strings.Join(metric.record, " ") == strings.Join(record, " ") {
					metricValue, err := strconv.ParseFloat(record[billedCostIndex], 64)
					fatal(err)
					metric.gauge.Set(metricValue)
					notFound = false
				}
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
						labels[records[0][j]] = ""
					}
				}

				newMetricsRow := promauto.NewGauge(prometheus.GaugeOpts{
					Name:        fmt.Sprintf("billed_cost_%s_%d", config.Name, i),
					ConstLabels: labels,
				})
				metricValue, err := strconv.ParseFloat(records[i][billedCostIndex], 64)
				fatal(err)
				newMetricsRow.Set(metricValue)
				prometheusMetrics = append(prometheusMetrics, recordGaugeCombo{record: record, gauge: newMetricsRow})
				registry.MustRegister(newMetricsRow)
			}
		}
		time.Sleep(time.Duration(config.PollingIntervalHours) * time.Hour)
	}
}

func main() {
	config, err := ParseConfigFile("/config/config.yaml")
	fatal(err)

	registry := prometheus.NewRegistry()

	go updatedMetrics(config, registry, []recordGaugeCombo{})

	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})

	http.Handle("/metrics", handler)
	http.ListenAndServe(":2112", nil)
}

func fatal(err error) {
	if err != nil {
		log.Fatalln(err)
	}
}
