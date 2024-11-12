package crypto

import (
	"strings"

	types1 "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/types/bech32"
)

func PubKeyAddress(consensusPubkey *types1.Any) string {
	switch consensusPubkey.TypeUrl {
	case "/cosmos.crypto.ed25519.PubKey":
		key := ed25519.PubKey{Key: consensusPubkey.Value[2:]}
		return key.Address().String()

	case "/cosmos.crypto.secp256k1.PubKey":
		key := secp256k1.PubKey{Key: consensusPubkey.Value[2:]}
		return key.Address().String()
	}

	panic("unknown pubkey type: " + consensusPubkey.TypeUrl)
}

func PubKeyBech32Address(consensusPubkey *types1.Any, prefix string) string {
	key := PubKeyAddress(consensusPubkey)
	address, _ := bech32.ConvertAndEncode(prefix, key.Address())
	
	return address
}

// GetHrpPrefix returns the human-readable prefix for a given address.
// Examples of valid address HRPs are "cosmosvalcons", "cosmosvaloper".
// So this will return "cosmos" as the prefix
func GetHrpPrefix(a string) string {

	hrp, _, err := bech32.DecodeAndConvert(a)
	if err != nil {
		return err.Error()
	}

	for _, v := range []string{"valoper", "cncl", "valcons"} {
		hrp = strings.TrimSuffix(hrp, v)
	}
	return hrp
}
