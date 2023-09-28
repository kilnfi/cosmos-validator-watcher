package metrics

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	Registry *prometheus.Registry

	// Global metrics
	ActiveSet       *prometheus.GaugeVec
	BlockHeight     *prometheus.GaugeVec
	ProposalEndTime *prometheus.GaugeVec
	SeatPrice       *prometheus.GaugeVec
	SkippedBlocks   *prometheus.CounterVec
	TrackedBlocks   *prometheus.CounterVec
	UpgradePlan     *prometheus.GaugeVec

	// Validator metrics
	Rank             *prometheus.GaugeVec
	ValidatedBlocks  *prometheus.CounterVec
	MissedBlocks     *prometheus.CounterVec
	SoloMissedBlocks *prometheus.CounterVec
	Tokens           *prometheus.GaugeVec
	IsBonded         *prometheus.GaugeVec
	IsJailed         *prometheus.GaugeVec
	Vote             *prometheus.GaugeVec

	// Node metrics
	NodeBlockHeight *prometheus.GaugeVec
	NodeSynced      *prometheus.GaugeVec
}

func New(namespace string) *Metrics {
	metrics := &Metrics{
		Registry: prometheus.NewRegistry(),
		BlockHeight: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "block_height",
				Help:      "Latest known block height (all nodes mixed up)",
			},
			[]string{"chain_id"},
		),
		ActiveSet: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "active_set",
				Help:      "Number of validators in the active set",
			},
			[]string{"chain_id"},
		),
		SeatPrice: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "seat_price",
				Help:      "Min seat price to be in the active set (ie. bonded tokens of the latest validator)",
			},
			[]string{"chain_id"},
		),
		Rank: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "rank",
				Help:      "Rank of the validator",
			},
			[]string{"chain_id", "address", "name"},
		),
		ValidatedBlocks: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "validated_blocks",
				Help:      "Number of validated blocks per validator (for a bonded validator)",
			},
			[]string{"chain_id", "address", "name"},
		),
		MissedBlocks: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "missed_blocks",
				Help:      "Number of missed blocks per validator (for a bonded validator)",
			},
			[]string{"chain_id", "address", "name"},
		),
		SoloMissedBlocks: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "solo_missed_blocks",
				Help:      "Number of missed blocks per validator, unless block is missed by many other validators",
			},
			[]string{"chain_id", "address", "name"},
		),
		TrackedBlocks: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "tracked_blocks",
				Help:      "Number of blocks tracked since start",
			},
			[]string{"chain_id"},
		),
		SkippedBlocks: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "skipped_blocks",
				Help:      "Number of blocks skipped (ie. not tracked) since start",
			},
			[]string{"chain_id"},
		),
		Tokens: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "tokens",
				Help:      "Number of staked tokens per validator",
			},
			[]string{"chain_id", "address", "name"},
		),
		IsBonded: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "is_bonded",
				Help:      "Set to 1 if the validator is bonded",
			},
			[]string{"chain_id", "address", "name"},
		),
		IsJailed: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "is_jailed",
				Help:      "Set to 1 if the validator is jailed",
			},
			[]string{"chain_id", "address", "name"},
		),
		Vote: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "vote",
				Help:      "Set to 1 if the validator has voted on a proposal",
			},
			[]string{"chain_id", "address", "name", "proposal_id"},
		),
		NodeBlockHeight: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "node_block_height",
				Help:      "Latest fetched block height for each node",
			},
			[]string{"chain_id", "node"},
		),
		NodeSynced: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "node_synced",
				Help:      "Set to 1 is the node is synced (ie. not catching-up)",
			},
			[]string{"chain_id", "node"},
		),
		UpgradePlan: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "upgrade_plan",
				Help:      "Block height of the upcoming upgrade (hard fork)",
			},
			[]string{"chain_id", "version"},
		),
		ProposalEndTime: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "proposal_end_time",
				Help:      "Timestamp of the voting end time of a proposal",
			},
			[]string{"chain_id", "proposal_id"},
		),
	}

	return metrics
}

func (m *Metrics) Register() {
	m.Registry.MustRegister(m.BlockHeight)
	m.Registry.MustRegister(m.ActiveSet)
	m.Registry.MustRegister(m.SeatPrice)
	m.Registry.MustRegister(m.Rank)
	m.Registry.MustRegister(m.ValidatedBlocks)
	m.Registry.MustRegister(m.MissedBlocks)
	m.Registry.MustRegister(m.SoloMissedBlocks)
	m.Registry.MustRegister(m.TrackedBlocks)
	m.Registry.MustRegister(m.SkippedBlocks)
	m.Registry.MustRegister(m.Tokens)
	m.Registry.MustRegister(m.IsBonded)
	m.Registry.MustRegister(m.IsJailed)
	m.Registry.MustRegister(m.Vote)
	m.Registry.MustRegister(m.NodeBlockHeight)
	m.Registry.MustRegister(m.NodeSynced)
	m.Registry.MustRegister(m.UpgradePlan)
	m.Registry.MustRegister(m.ProposalEndTime)
}
