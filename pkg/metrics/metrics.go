package metrics

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	// Exporter metrics
	BlockHeight     prometheus.Gauge
	ValidatedBlocks *prometheus.CounterVec
	MissedBlocks    *prometheus.CounterVec
	TrackedBlocks   prometheus.Counter
	SkippedBlocks   prometheus.Counter
	BondedTokens    *prometheus.GaugeVec
	IsBonded        *prometheus.GaugeVec
	IsJailed        *prometheus.GaugeVec

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
		BondedTokens: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "bonded_tokens",
				Help:      "Number of bonded tokens per validator",
			},
			[]string{"address", "name"},
		),
		IsBonded: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "is_bonded",
				Help:      "Set to 1 if the validator is bonded",
			},
			[]string{"address", "name"},
		),
		IsJailed: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "is_jailed",
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
	prometheus.MustRegister(metrics.BondedTokens)
	prometheus.MustRegister(metrics.IsBonded)
	prometheus.MustRegister(metrics.IsJailed)
	prometheus.MustRegister(metrics.NodeBlockHeight)
	prometheus.MustRegister(metrics.NodeSynced)

	return metrics
}
