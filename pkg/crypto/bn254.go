package crypto

import (
	"bytes"
	"fmt"

	"github.com/cometbft/cometbft/crypto"
	"github.com/cometbft/cometbft/crypto/tmhash"
	cmtjson "github.com/cometbft/cometbft/libs/json"
)

const (
	Bn254PubKeyName = "cometbft/PubKeyBn254"
)

func init() {
	cmtjson.RegisterType(Bn254PubKey{}, Bn254PubKeyName)
}

var _ crypto.PubKey = Bn254PubKey{}

type Bn254PubKey []byte

// Address is the SHA256-20 of the raw pubkey bytes.
func (pubKey Bn254PubKey) Address() crypto.Address {
	// if len(pubKey) != PubKeySize {
	// 	panic("pubkey is incorrect size")
	// }
	return crypto.Address(tmhash.SumTruncated(pubKey))
}

// Bytes returns the PubKey byte format.
func (pubKey Bn254PubKey) Bytes() []byte {
	return []byte(pubKey)
}

func (pubKey Bn254PubKey) VerifySignature(msg []byte, sig []byte) bool {
	return false
}

func (pubKey Bn254PubKey) String() string {
	return fmt.Sprintf("PubKeyBn254{%X}", []byte(pubKey))
}

func (Bn254PubKey) Type() string {
	return "Bn254"
}

func (pubKey Bn254PubKey) Equals(other crypto.PubKey) bool {
	if otherEd, ok := other.(Bn254PubKey); ok {
		return bytes.Equal(pubKey[:], otherEd[:])
	}

	return false
}
