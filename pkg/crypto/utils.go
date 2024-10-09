package crypto

import (
	types1 "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
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
