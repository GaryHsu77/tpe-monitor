package main

import (
	"errors"
	"strings"

	"github.com/iancoleman/strcase"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

type Counter struct {
	Desc  *prometheus.Desc
	value float64
	label []string
	// ... many more fields
}

func NewNumber(desc *prometheus.Desc) *Counter {
	return &Counter{
		Desc: desc,
	}
}

func (c *Counter) Set(v float64, label []string) {
	if v < 0 {
		logrus.Errorln(errors.New("counter cannot decrease in value"))
		return
	}
	c.value = v
	c.label = label
}

// Describe simply sends the two Descs in the struct to the channel.
func (c *Counter) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.Desc
}

// Collect already added counter values
func (c *Counter) Collect(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(
		c.Desc,
		prometheus.CounterValue,
		c.value,
		c.label...,
	)
}

func toKey(origin string, tag TagfResponse) string {
	if strings.Index(tag.PrvdName, "$connection_") == 0 {
		if strings.Index(tag.SrcName, "store") == 0 {
			return strcase.ToLowerCamel("store " + tag.TagName)
		}

		if strings.Index(tag.SrcName, "messageGroup") == 0 {
			return strcase.ToLowerCamel("messageGroup " + tag.TagName)
		}

		return strcase.ToLowerCamel("connection " + tag.TagName)
	}

	return strcase.ToLowerCamel(tag.TagName)
}

func toFloat32(input interface{}) float32 {
	switch input.(type) {
	case float32:
		return float32(input.(float32))
	case float64:
		return float32(input.(float64))
	default:
		return 0
	}
}

func toDouble(input interface{}) float64 {
	switch input.(type) {
	case float32:
		return float64(input.(float32))
	case float64:
		return input.(float64)
	default:
		return 0
	}
}

func toUint64(input interface{}) uint64 {
	switch input.(type) {
	case int64:
		return uint64(input.(int64))
	case uint64:
		return uint64(input.(uint64))
	case float32:
		return uint64(input.(float32))
	case float64:
		return uint64(input.(float64))
	default:
		return 0
	}
}

func parseValue(tag TagfResponse) float64 {
	switch tag.Type {
	case Boolean:
		if tag.Value.(bool) {
			return 1
		}
		return 0
	case Int8, Int16, Int32, Int64, Uint8, Uint16, Uint32, Uint64:
		return float64(toUint64(tag.Value))
	case Float:
		return float64(toFloat32(tag.Value))
	case Double:
		return toDouble(tag.Value)
	case String:
		fallthrough
	case Bytearray:
		fallthrough
	case Raw:
		fallthrough
	default:
		return 0
	}
}
