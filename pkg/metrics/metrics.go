package metrics

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	// Exporter metrics
	BlockHeight     prometheus.Gauge
	ValidatedBlocks *prometheus.CounterVec
	MissedBlocks    *prometheus.CounterVec
	TrackedBlocks   prometheus.Counter
	SkippedBlocks   prometheus.Counter
	ValidatorBonded *prometheus.GaugeVec
	ValidatorJail   *prometheus.GaugeVec

	// Node metrics
	NodeBlockHeight *prometheus.GaugeVec
	NodeSynced      *prometheus.GaugeVec
}

func New(namespace string) *Metrics {
	metrics := &Metrics{
		BlockHeight: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "block_height",
				Help:      "Latest known block height (all nodes mixed up)",
			},
		),
		ValidatedBlocks: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "validated_blocks",
				Help:      "Number of validated blocks per validator (for a bonded validator)",
			},
			[]string{"address", "name"},
		),
		MissedBlocks: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "missed_blocks",
				Help:      "Number of missed blocks per validator (for a bonded validator)",
			},
			[]string{"address", "name"},
		),
		TrackedBlocks: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "tracked_blocks",
				Help:      "Number of blocks tracked since start",
			},
		),
		SkippedBlocks: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "skipped_blocks",
				Help:      "Number of blocks skipped (ie. not tracked) since start",
			},
		),
		ValidatorBonded: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "validator_bonded",
				Help:      "Set to 1 if the validator is bonded",
			},
			[]string{"address", "name"},
		),
		ValidatorJail: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "validator_jail",
				Help:      "Set to 1 if the validator is jailed",
			},
			[]string{"address", "name"},
		),
		NodeBlockHeight: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "node_block_height",
				Help:      "Latest fetched block height for each node",
			},
			[]string{"node"},
		),
		NodeSynced: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "node_synced",
				Help:      "Set to 1 is the node is synced (ie. not catching-up)",
			},
			[]string{"node"},
		),
	}

	prometheus.MustRegister(metrics.BlockHeight)
	prometheus.MustRegister(metrics.ValidatedBlocks)
	prometheus.MustRegister(metrics.MissedBlocks)
	prometheus.MustRegister(metrics.TrackedBlocks)
	prometheus.MustRegister(metrics.SkippedBlocks)
	prometheus.MustRegister(metrics.ValidatorBonded)
	prometheus.MustRegister(metrics.ValidatorJail)
	prometheus.MustRegister(metrics.NodeBlockHeight)
	prometheus.MustRegister(metrics.NodeSynced)

	return metrics
}
