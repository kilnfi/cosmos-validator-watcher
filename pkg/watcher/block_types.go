package watcher

import (
	"github.com/cometbft/cometbft/types"
	"github.com/shopspring/decimal"
)

type BlockInfo struct {
	ChainID          string
	Height           int64
	Transactions     int
	TransactionsData [][]byte
	TotalValidators  int
	SignedValidators int
	ProposerAddress  string
	ValidatorStatus  []ValidatorStatus
}

func NewBlockInfo(block *types.Block, validatorStatus []ValidatorStatus) *BlockInfo {
	// Compute total signed validators
	signedValidators := 0
	for _, sig := range block.LastCommit.Signatures {
		if sig.BlockIDFlag == types.BlockIDFlagCommit {
			signedValidators++
		}
	}

	txs := make([][]byte, len(block.Data.Txs))
    for i, tx := range block.Data.Txs {
        txs[i] = tx
    }

	return &BlockInfo{
		ChainID:          block.Header.ChainID,
		Height:           block.Header.Height,
		Transactions:     len(block.Data.Txs),
		TransactionsData: txs,
		TotalValidators:  len(block.LastCommit.Signatures),
		SignedValidators: signedValidators,
		ValidatorStatus:  validatorStatus,
		ProposerAddress:  block.Header.ProposerAddress.String(),
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
