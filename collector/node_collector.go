package collector

import (
	"strings"
	"time"

	"github.com/pr8kerl/f5er/f5"
	"github.com/prometheus/client_golang/prometheus"
)

// A NodeCollector implements the prometheus.Collector.
type NodeCollector struct {
	metrics                   map[string]nodeMetric
	bigip                     *f5.Device
	partitionsList           []string
	collectorScrapeStatus   *prometheus.GaugeVec
	collectorScrapeDuration *prometheus.SummaryVec
}

type nodeMetric struct {
	desc      *prometheus.Desc
	extract   func(f5.LBNodeStatsInnerEntries) float64
	valueType prometheus.ValueType
}

// NewNodeCollector returns a collector that collecting node statistics
func NewNodeCollector(bigip *f5.Device, namespace string, partitionsList []string) (*NodeCollector, error) {
	var (
		subsystem  = "node"
		labelNames = []string{"partition", "node"}
	)
	return &NodeCollector{
		metrics: map[string]nodeMetric{
			"serverside_bytesOut": {
				desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, subsystem, "serverside_bytes_transmitted_total"),
					"serverside_bytes_transmitted_total",
					labelNames,
					nil,
				),
				extract: func(entries f5.LBNodeStatsInnerEntries) float64 {
					return float64(entries.Serverside_bitsOut.Value / 8)
				},
				valueType: prometheus.CounterValue,
			},
			"serverside_maxConns": {
				desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, subsystem, "serverside_connections_concurrent_max"),
					"serverside_connections_concurrent_max",
					labelNames,
					nil,
				),
				extract: func(entries f5.LBNodeStatsInnerEntries) float64 {
					return float64(entries.Serverside_maxConns.Value)
				},
				valueType: prometheus.CounterValue,
			},
			"serverside_curConns": {
				desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, subsystem, "serverside_connections"),
					"serverside_connections",
					labelNames,
					nil,
				),
				extract: func(entries f5.LBNodeStatsInnerEntries) float64 {
					return float64(entries.Serverside_curConns.Value)
				},
				valueType: prometheus.GaugeValue,
			},
			"serverside_pktsOut": {
				desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, subsystem, "serverside_packets_transmitted_total"),
					"serverside_packets_transmitted_total",
					labelNames,
					nil,
				),
				extract: func(entries f5.LBNodeStatsInnerEntries) float64 {
					return float64(entries.Serverside_pktsOut.Value)
				},
				valueType: prometheus.CounterValue,
			},
			"totRequests": {
				desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, subsystem, "requests_total"),
					"requests_total",
					labelNames,
					nil,
				),
				extract: func(entries f5.LBNodeStatsInnerEntries) float64 {
					return float64(entries.TotRequests.Value)
				},
				valueType: prometheus.CounterValue,
			},
			"serverside_pktsIn": {
				desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, subsystem, "serverside_packets_received_total"),
					"serverside_packets_received_total",
					labelNames,
					nil,
				),
				extract: func(entries f5.LBNodeStatsInnerEntries) float64 {
					return float64(entries.Serverside_pktsIn.Value)
				},
				valueType: prometheus.CounterValue,
			},
			"serverside_totConns": {
				desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, subsystem, "serverside_connections_total"),
					"serverside_connections_total",
					labelNames,
					nil,
				),
				extract: func(entries f5.LBNodeStatsInnerEntries) float64 {
					return float64(entries.Serverside_totConns.Value)
				},
				valueType: prometheus.CounterValue,
			},
			"serverside_bytesIn": {
				desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, subsystem, "serverside_bytes_received_total"),
					"serverside_bytes_received_total",
					labelNames,
					nil,
				),
				extract: func(entries f5.LBNodeStatsInnerEntries) float64 {
					return float64(entries.Serverside_bitsIn.Value / 8)
				},
				valueType: prometheus.CounterValue,
			},
			"curSessions": {
				desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, subsystem, "sessions"),
					"sessions",
					labelNames,
					nil,
				),
				extract: func(entries f5.LBNodeStatsInnerEntries) float64 {
					return float64(entries.CurSessions.Value)
				},
				valueType: prometheus.GaugeValue,
			},
			"status_availabilityState": {
				desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, subsystem, "status_availability_state"),
					"status_availability_state",
					labelNames,
					nil,
				),
				extract: func(entries f5.LBNodeStatsInnerEntries) float64 {
					if entries.Status_availabilityState.Description == "available" {
						return 1
					}
					return 0
				},
				valueType: prometheus.GaugeValue,
			},
		},
		collectorScrapeStatus: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "collector_scrape_status",
				Help:      "collector_scrape_status",
			},
			[]string{"collector"},
		),
		collectorScrapeDuration: prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Namespace: namespace,
				Name:      "collector_scrape_duration",
				Help:      "collector_scrape_duration",
			},
			[]string{"collector"},
		),
		bigip:           bigip,
		partitionsList: partitionsList,
	}, nil
}

// Collect collects metrics for BIG-IP nodes.
func (c *NodeCollector) Collect(ch chan<- prometheus.Metric) {
	start := time.Now()
	err, allNodeStats := c.bigip.ShowAllNodeStats()
	if err != nil {
		c.collectorScrapeStatus.WithLabelValues("node").Set(float64(0))
		logger.Warningf("Failed to get statistics for nodes (%s)", err)
	} else {
		for key, nodeStats := range allNodeStats.Entries {
			keyParts := strings.Split(key, "/")
			path := keyParts[len(keyParts)-2]
			pathParts := strings.Split(path, "~")
			partition := pathParts[1]
			nodeName := pathParts[len(pathParts)-1]

			if c.partitionsList != nil && !stringInSlice(partition, c.partitionsList) {
				continue
			}

			labels := []string{partition, nodeName}
			for _, metric := range c.metrics {
				ch <- prometheus.MustNewConstMetric(metric.desc, metric.valueType, metric.extract(nodeStats.NestedStats.Entries), labels...)
			}
		}
		c.collectorScrapeStatus.WithLabelValues("node").Set(float64(1))
		logger.Debugf("Successfully fetched statistics for nodes")
	}

	elapsed := time.Since(start)
	c.collectorScrapeDuration.WithLabelValues("node").Observe(float64(elapsed.Seconds()))
	c.collectorScrapeStatus.Collect(ch)
	c.collectorScrapeDuration.Collect(ch)
	logger.Debugf("Getting node statistics took %s", elapsed)
}

// Describe describes the metrics exported from this collector.
func (c *NodeCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, metric := range c.metrics {
		ch <- metric.desc
	}
	c.collectorScrapeStatus.Describe(ch)
	c.collectorScrapeDuration.Describe(ch)
}
