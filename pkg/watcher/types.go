package watcher

import (
	"strings"

	"github.com/cosmos/cosmos-sdk/types/bech32"
	utils "github.com/kilnfi/cosmos-validator-watcher/pkg/crypto"
)

type TrackedValidator struct {
	Address          string
	Name             string
	Moniker          string
	OperatorAddress  string
	ConsensusAddress string
}

func ParseValidator(val string) TrackedValidator {
	parts := strings.Split(val, ":")
	if len(parts) > 1 {
		return TrackedValidator{
			Address: parts[0],
			Name:    parts[1],
		}
	}

	return TrackedValidator{
		Address: parts[0],
		Name:    parts[0],
	}
}

func (t TrackedValidator) AccountAddress() string {
	_, bytes, err := bech32.DecodeAndConvert(t.OperatorAddress)
	if err != nil {
		return err.Error()
	}

	prefix := utils.GetHrpPrefix(t.OperatorAddress)

	conv, err := bech32.ConvertAndEncode(prefix, bytes)
	if err != nil {
		return err.Error()
	}

	return conv
}
