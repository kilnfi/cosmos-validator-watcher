package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

type Metrics struct {
	Registry *prometheus.Registry

	// Global metrics
	ActiveSet                *prometheus.GaugeVec
	BlockHeight              *prometheus.GaugeVec
	ProposalEndTime          *prometheus.GaugeVec
	SeatPrice                *prometheus.GaugeVec
	SkippedBlocks            *prometheus.CounterVec
	TrackedBlocks            *prometheus.CounterVec
	Transactions             *prometheus.CounterVec
	UpgradePlan              *prometheus.GaugeVec
	SignedBlocksWindow       *prometheus.GaugeVec
	MinSignedBlocksPerWindow *prometheus.GaugeVec
	DowntimeJailDuration     *prometheus.GaugeVec
	SlashFractionDoubleSign  *prometheus.GaugeVec
	SlashFractionDowntime    *prometheus.GaugeVec

	// Validator metrics
	Rank                    *prometheus.GaugeVec
	ProposedBlocks          *prometheus.CounterVec
	ValidatedBlocks         *prometheus.CounterVec
	MissedBlocks            *prometheus.CounterVec
	SoloMissedBlocks        *prometheus.CounterVec
	ConsecutiveMissedBlocks *prometheus.GaugeVec
	MissedBlocksWindow      *prometheus.GaugeVec
	EmptyBlocks             *prometheus.CounterVec
	Tokens                  *prometheus.GaugeVec
	IsBonded                *prometheus.GaugeVec
	IsJailed                *prometheus.GaugeVec
	Commission              *prometheus.GaugeVec
	Vote                    *prometheus.GaugeVec

	// Babylon metrics
	BabylonEpoch                           *prometheus.GaugeVec
	BabylonCheckpointVote                  *prometheus.CounterVec
	BabylonCommittedCheckpointVote         *prometheus.CounterVec
	BabylonMissedCheckpointVote            *prometheus.CounterVec
	BabylonConsecutiveMissedCheckpointVote *prometheus.GaugeVec
	BabylonFinalityVotes                   *prometheus.CounterVec
	BabylonCommittedFinalityVotes          *prometheus.CounterVec
	BabylonMissedFinalityVotes             *prometheus.CounterVec
	BabylonConsecutiveMissedFinalityVotes  *prometheus.GaugeVec

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
			[]string{"chain_id", "denom"},
		),
		Rank: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "rank",
				Help:      "Rank of the validator",
			},
			[]string{"chain_id", "address", "name"},
		),
		ProposedBlocks: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "proposed_blocks",
				Help:      "Number of proposed blocks per validator (for a bonded validator)",
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
		ConsecutiveMissedBlocks: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "consecutive_missed_blocks",
				Help:      "Number of consecutive missed blocks per validator (for a bonded validator)",
			},
			[]string{"chain_id", "address", "name"},
		),
		MissedBlocksWindow: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "missed_blocks_window",
				Help:      "Number of missed blocks per validator for the current signing window (for a bonded validator)",
			},
			[]string{"chain_id", "address", "name"},
		),
		EmptyBlocks: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "empty_blocks",
				Help:      "Number of empty blocks proposed by validator",
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
		Transactions: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "transactions_total",
				Help:      "Number of transactions since start",
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
			[]string{"chain_id", "address", "name", "denom"},
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
		Commission: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "commission",
				Help:      "Earned validator commission",
			},
			[]string{"chain_id", "address", "name", "denom"},
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
		SignedBlocksWindow: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "signed_blocks_window",
				Help:      "Number of blocks per signing window",
			},
			[]string{"chain_id"},
		),
		MinSignedBlocksPerWindow: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "min_signed_blocks_per_window",
				Help:      "Minimum number of blocks required to be signed per signing window",
			},
			[]string{"chain_id"},
		),
		DowntimeJailDuration: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "downtime_jail_duration",
				Help:      "Duration of the jail period for a validator in seconds",
			},
			[]string{"chain_id"},
		),
		SlashFractionDoubleSign: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "slash_fraction_double_sign",
				Help:      "Slash penaltiy for double-signing",
			},
			[]string{"chain_id"},
		),
		SlashFractionDowntime: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "slash_fraction_downtime",
				Help:      "Slash penaltiy for downtime",
			},
			[]string{"chain_id"},
		),
		BabylonEpoch: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "babylon_epoch",
				Help:      "Babylon epoch",
			},
			[]string{"chain_id"},
		),
		BabylonCheckpointVote: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "babylon_checkpoint_vote",
				Help:      "Count of checkpoint votes since start (equal to number of epochs)",
			},
			[]string{"chain_id"},
		),
		BabylonCommittedCheckpointVote: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "babylon_committed_checkpoint_vote",
				Help:      "Number of committed checkpoint votes for a validator",
			},
			[]string{"chain_id", "address", "name"},
		),
		BabylonMissedCheckpointVote: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "babylon_missed_checkpoint_vote",
				Help:      "Number of missed checkpoint votes for a validator",
			},
			[]string{"chain_id", "address", "name"},
		),
		BabylonConsecutiveMissedCheckpointVote: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "babylon_consecutive_missed_checkpoint_vote",
				Help:      "Number of consecutive missed checkpoint votes for a validator",
			},
			[]string{"chain_id", "address", "name"},
		),
		BabylonFinalityVotes: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "babylon_finality_votes",
				Help:      "Count of total finality provider slots since start",
			},
			[]string{"chain_id"},
		),
		BabylonCommittedFinalityVotes: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "babylon_committed_finality_votes",
				Help:      "Number of votes for a finality provider",
			},
			[]string{"chain_id", "address", "name"},
		),
		BabylonMissedFinalityVotes: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "babylon_missed_finality_votes",
				Help:      "Number of missed votes for a finality provider",
			},
			[]string{"chain_id", "address", "name"},
		),
		BabylonConsecutiveMissedFinalityVotes: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "babylon_consecutive_missed_finality_votes",
				Help:      "Number of consecutive missed votes for a finality provider",
			},
			[]string{"chain_id", "address", "name"},
		),
	}

	return metrics
}

func (m *Metrics) Register() {
	m.Registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	m.Registry.MustRegister(collectors.NewGoCollector())

	m.Registry.MustRegister(m.BlockHeight)
	m.Registry.MustRegister(m.ActiveSet)
	m.Registry.MustRegister(m.SeatPrice)
	m.Registry.MustRegister(m.Rank)
	m.Registry.MustRegister(m.ProposedBlocks)
	m.Registry.MustRegister(m.ValidatedBlocks)
	m.Registry.MustRegister(m.MissedBlocks)
	m.Registry.MustRegister(m.SoloMissedBlocks)
	m.Registry.MustRegister(m.ConsecutiveMissedBlocks)
	m.Registry.MustRegister(m.MissedBlocksWindow)
	m.Registry.MustRegister(m.EmptyBlocks)
	m.Registry.MustRegister(m.TrackedBlocks)
	m.Registry.MustRegister(m.Transactions)
	m.Registry.MustRegister(m.SkippedBlocks)
	m.Registry.MustRegister(m.Tokens)
	m.Registry.MustRegister(m.IsBonded)
	m.Registry.MustRegister(m.Commission)
	m.Registry.MustRegister(m.IsJailed)
	m.Registry.MustRegister(m.Vote)
	m.Registry.MustRegister(m.NodeBlockHeight)
	m.Registry.MustRegister(m.NodeSynced)
	m.Registry.MustRegister(m.UpgradePlan)
	m.Registry.MustRegister(m.ProposalEndTime)
	m.Registry.MustRegister(m.SignedBlocksWindow)
	m.Registry.MustRegister(m.MinSignedBlocksPerWindow)
	m.Registry.MustRegister(m.DowntimeJailDuration)
	m.Registry.MustRegister(m.SlashFractionDoubleSign)
	m.Registry.MustRegister(m.SlashFractionDowntime)
	m.Registry.MustRegister(m.BabylonEpoch)
	m.Registry.MustRegister(m.BabylonCheckpointVote)
	m.Registry.MustRegister(m.BabylonCommittedCheckpointVote)
	m.Registry.MustRegister(m.BabylonMissedCheckpointVote)
	m.Registry.MustRegister(m.BabylonConsecutiveMissedCheckpointVote)
	m.Registry.MustRegister(m.BabylonFinalityVotes)
	m.Registry.MustRegister(m.BabylonCommittedFinalityVotes)
	m.Registry.MustRegister(m.BabylonMissedFinalityVotes)
	m.Registry.MustRegister(m.BabylonConsecutiveMissedFinalityVotes)
}
