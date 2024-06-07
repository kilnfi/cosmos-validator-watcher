package crypto

import (
	"bytes"
	"fmt"

	"github.com/cometbft/cometbft/crypto"
	"github.com/cometbft/cometbft/crypto/tmhash"
	cmtjson "github.com/cometbft/cometbft/libs/json"
)

const (
	PubKeyName = "cometbft/PubKeyBls12_381"
)

func init() {
	cmtjson.RegisterType(Bls12PubKey{}, PubKeyName)
}

var _ crypto.PubKey = Bls12PubKey{}

type Bls12PubKey []byte

// Address is the SHA256-20 of the raw pubkey bytes.
func (pubKey Bls12PubKey) Address() crypto.Address {
	// if len(pubKey) != PubKeySize {
	// 	panic("pubkey is incorrect size")
	// }
	return crypto.Address(tmhash.SumTruncated(pubKey))
}

// Bytes returns the PubKey byte format.
func (pubKey Bls12PubKey) Bytes() []byte {
	return []byte(pubKey)
}

func (pubKey Bls12PubKey) VerifySignature(msg []byte, sig []byte) bool {
	return false
}

func (pubKey Bls12PubKey) String() string {
	return fmt.Sprintf("PubKeyBls12_381{%X}", []byte(pubKey))
}

func (Bls12PubKey) Type() string {
	return "bls12_381"
}

func (pubKey Bls12PubKey) Equals(other crypto.PubKey) bool {
	if otherEd, ok := other.(Bls12PubKey); ok {
		return bytes.Equal(pubKey[:], otherEd[:])
	}

	return false
}
