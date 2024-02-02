package watcher

import (
	"strings"

	"github.com/cosmos/cosmos-sdk/types/bech32"
)

type TrackedValidator struct {
	Address         string
	Name            string
	Moniker         string
	OperatorAddress string
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
	prefix, bytes, err := bech32.DecodeAndConvert(t.OperatorAddress)
	if err != nil {
		return err.Error()
	}

	for _, v := range []string{"valoper", "cncl"} {
		prefix = strings.TrimSuffix(prefix, v)
	}
	conv, err := bech32.ConvertAndEncode(prefix, bytes)
	if err != nil {
		return err.Error()
	}

	return conv
}
