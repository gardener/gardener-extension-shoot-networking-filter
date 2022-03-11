// SPDX-FileCopyrightText: 2021 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0
package metrics

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

func init() {
	metrics.Registry.MustRegister(FilterListSize)
	metrics.Registry.MustRegister(FilterListDownloads)
}

var (
	FilterListDownloads = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "shoot_networking_filter_list_downloads",
			Help: "Total number of download requests executed",
		},
		[]string{"success"},
	)

	FilterListSize = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "shoot_networking_filter_list_size",
			Help: "Number of entries in filter list",
		},
		[]string{"list"},
	)
)

// ReportDownload reports a filter list download.
func ReportDownload(success bool) {
	FilterListDownloads.WithLabelValues(strconv.FormatBool(success)).Inc()
}

// ReportFilterListSize reports the size of a filter list.
func ReportFilterListSize(name string, size int) {
	FilterListSize.WithLabelValues(name).Set(float64(size))
}
