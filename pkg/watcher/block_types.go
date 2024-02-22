package watcher

import (
	"github.com/cometbft/cometbft/types"
	"github.com/shopspring/decimal"
)

type BlockInfo struct {
	ChainID          string
	Height           int64
	Transactions     int
	TotalValidators  int
	SignedValidators int
	ValidatorStatus  []ValidatorStatus
}

func NewBlockInfo(block *types.Block, validatorStatus []ValidatorStatus) *BlockInfo {
	// Compute total signed validators
	signedValidators := 0
	for _, sig := range block.LastCommit.Signatures {
		if !sig.Absent() {
			signedValidators++
		}
	}

	return &BlockInfo{
		ChainID:          block.Header.ChainID,
		Height:           block.Header.Height,
		Transactions:     block.Txs.Len(),
		TotalValidators:  len(block.LastCommit.Signatures),
		SignedValidators: signedValidators,
		ValidatorStatus:  validatorStatus,
	}
}

func (b *BlockInfo) SignedRatio() decimal.Decimal {
	if b.TotalValidators == 0 {
		return decimal.Zero
	}

	return decimal.NewFromInt(int64(b.SignedValidators)).
		Div(decimal.NewFromInt(int64(b.TotalValidators)))
}

type ValidatorStatus struct {
	Address string
	Label   string
	Bonded  bool
	Signed  bool
	Rank    int
}
