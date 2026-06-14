package server

import (
	"github.com/feedme/aws-eks-ipamd-attribution-exporter/internal/attribution"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	ipCounts    *prometheus.GaugeVec
	duration    prometheus.Gauge
	errorsTotal *prometheus.CounterVec
	mismatches  *prometheus.GaugeVec
	lastSuccess prometheus.Gauge
	buildInfo   *prometheus.GaugeVec
}

func newMetrics(cfg Config, registry *prometheus.Registry) *metrics {
	m := &metrics{
		ipCounts: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "ipamd_attribution_ips",
			Help: "Number of AWS-observed private IPv4 addresses by Kubernetes attribution state.",
		}, []string{"cluster", "subnet_id", "subnet_name", "az", "nodepool", "instance_type", "state"}),
		duration: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "ipamd_attribution_reconcile_duration_seconds",
			Help: "Duration of the last reconcile in seconds.",
		}),
		errorsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ipamd_attribution_reconcile_errors_total",
			Help: "Total reconcile errors by bounded reason.",
		}, []string{"reason"}),
		mismatches: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "ipamd_attribution_pod_ip_mismatches",
			Help: "Active Kubernetes pod IPv4 addresses not observed on an in-scope AWS ENI by bounded reason.",
		}, []string{"reason"}),
		lastSuccess: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "ipamd_attribution_last_success_timestamp_seconds",
			Help: "Unix timestamp of the last successful reconcile.",
		}),
		buildInfo: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "ipamd_attribution_build_info",
			Help: "Build information for the exporter.",
		}, []string{"version", "commit", "cluster"}),
	}

	registry.MustRegister(m.ipCounts, m.duration, m.errorsTotal, m.mismatches, m.lastSuccess, m.buildInfo)
	m.buildInfo.WithLabelValues(cfg.BuildVersion, cfg.BuildCommit, cfg.ClusterName).Set(1)
	return m
}

func (m *metrics) recordSuccess(result attribution.Result) {
	m.ipCounts.Reset()
	for key, value := range result.Metrics {
		m.ipCounts.WithLabelValues(
			key.Cluster,
			key.SubnetID,
			key.SubnetName,
			key.AZ,
			key.Nodepool,
			key.InstanceType,
			key.State,
		).Set(value)
	}

	m.mismatches.Reset()
	for reason, value := range result.Mismatches {
		m.mismatches.WithLabelValues(reason).Set(value)
	}

	m.duration.Set(result.Duration.Seconds())
	m.lastSuccess.Set(float64(result.FinishedAt.Unix()))
	m.incrementErrors(result.Errors)
}

func (m *metrics) recordFailure(result attribution.Result) {
	m.duration.Set(result.Duration.Seconds())
	m.incrementErrors(result.Errors)
}

func (m *metrics) incrementErrors(current map[string]float64) {
	for reason, value := range current {
		m.errorsTotal.WithLabelValues(reason).Add(value)
	}
}
