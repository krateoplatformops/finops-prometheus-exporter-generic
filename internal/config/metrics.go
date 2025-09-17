package config

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Metrics struct {
	Value []Value `json:"value"`
}

type Value struct {
	Id         string       `json:"id"`
	Name       Name         `json:"name"`
	Unit       string       `json:"unit"`
	Timeseries []Timeseries `json:"timeseries"`
}

type Name struct {
	Value          string `json:"value"`
	LocalizedValue string `json:"localizedValue"`
}

type Timeseries struct {
	Data []Data `json:"data"`
}

type Data struct {
	Timestamp metav1.Time       `json:"timeStamp"`
	Average   resource.Quantity `json:"average"`
}
