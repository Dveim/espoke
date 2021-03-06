// Copyright © 2018 Barthelemy Vessemont
// GNU General Public License version 3

package cmd

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

var (
	errorsCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "es_probe_errors_count",
		Help: "Reports Espoke internal errors absolute counter since start",
	})

	nodeCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "es_node_count",
		Help: "Reports current discovered nodes amount",
	})

	shardsSuccessfulGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "es_node_shards_successful",
			Help: "Reports current successful shards",
		},
		[]string{"cluster", "nodename"},
	)

	docsHitGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "es_node_documents_hit",
			Help: "Reports current successful shards",
		},
		[]string{"cluster", "nodename"},
	)

	elasticNodeAvailabilityGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "es_node_availability",
			Help: "Reflects node availabity : 1 is OK, 0 means node unavailable ",
		},
		[]string{"cluster", "nodename"},
	)

	kibanaNodeAvailabilityGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "kibana_node_availability",
			Help: "Reflects node availabity : 1 is OK, 0 means node unavailable ",
		},
		[]string{"cluster", "nodename"},
	)

	nodeSearchLatencySummary = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "es_node_search_latency",
			Help:       "Measure latency for every node (quantiles - in ns)",
			MaxAge:     20 * time.Minute, // default value * 2
			AgeBuckets: 20,               // default value * 4
			BufCap:     2000,             // default value * 4
		},
		[]string{"cluster", "nodename"},
	)

	consulDiscoveryDurationSummary = promauto.NewSummary(prometheus.SummaryOpts{
		Name:       "es_probe_consul_discovery_duration",
		Help:       "Time spent for discovering nodes using Consul API (in ns)",
		MaxAge:     20 * time.Minute, // default value * 2
		AgeBuckets: 20,               // default value * 4
		BufCap:     2000,             // default value * 4
	})

	cleaningMetricsDurationSummary = promauto.NewSummary(prometheus.SummaryOpts{
		Name:       "es_probe_metrics_cleaning_duration",
		Help:       "Time spent for cleaning vanished nodes metrics (in ns)",
		MaxAge:     120 * time.Minute, // default value * 6
		AgeBuckets: 20,                // default value * 4
		BufCap:     2000,              // default value * 4
	})
)

func startMetricsEndpoint() {
	log.Info("Starting Prometheus /metrics endpoint on port ", metricsPort)
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%v", metricsPort), nil))
	}()
}

func cleanMetrics(nodes []esnode, allEverKnownNodes []string) error {
	start := time.Now()

	for _, nodeSerializedString := range allEverKnownNodes {
		n := strings.SplitN(nodeSerializedString, "|", 2) // [0]: name , [1] cluster

		deleteThisNodeMetrics := true
		for _, node := range nodes {
			if (node.name == n[0]) && (node.cluster == n[1]) {
				log.Debug("Metrics are live for node ", n[0], " from cluster ", n[1], " - keeping them")
				deleteThisNodeMetrics = false
				continue
			}
		}
		if deleteThisNodeMetrics {
			log.Info("Metrics removed for vanished node ", n[0], " from cluster ", n[1])
			elasticNodeAvailabilityGauge.DeleteLabelValues(n[1], n[0])
			nodeSearchLatencySummary.DeleteLabelValues(n[1], n[0])
			shardsSuccessfulGauge.DeleteLabelValues(n[1], n[0])
			docsHitGauge.DeleteLabelValues(n[1], n[0])
			kibanaNodeAvailabilityGauge.DeleteLabelValues(n[1], n[0])
		}
	}

	durationNanosec := float64(time.Since(start).Nanoseconds())
	cleaningMetricsDurationSummary.Observe(durationNanosec)
	return nil
}
