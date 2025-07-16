package crypto

import (
	"bytes"
	"fmt"

	"github.com/cometbft/cometbft/crypto"
	"github.com/cometbft/cometbft/crypto/tmhash"
	cmtjson "github.com/cometbft/cometbft/libs/json"
)

const (
	Secp256k1ethPubKeyName = "cometbft/PubKeySecp256k1eth"
)

func init() {
	cmtjson.RegisterType(Secp256k1ethPubKey{}, Secp256k1ethPubKeyName)
}

var _ crypto.PubKey = Secp256k1ethPubKey{}

type Secp256k1ethPubKey []byte

// Address is the SHA256-20 of the raw pubkey bytes.
func (pubKey Secp256k1ethPubKey) Address() crypto.Address {
	// if len(pubKey) != PubKeySize {
	// 	panic("pubkey is incorrect size")
	// }
	return crypto.Address(tmhash.SumTruncated(pubKey))
}

// Bytes returns the PubKey byte format.
func (pubKey Secp256k1ethPubKey) Bytes() []byte {
	return []byte(pubKey)
}

func (pubKey Secp256k1ethPubKey) VerifySignature(msg []byte, sig []byte) bool {
	return false
}

func (pubKey Secp256k1ethPubKey) String() string {
	return fmt.Sprintf("PubKeySecp256k1eth_381{%X}", []byte(pubKey))
}

func (Secp256k1ethPubKey) Type() string {
	return "Secp256k1eth_381"
}

func (pubKey Secp256k1ethPubKey) Equals(other crypto.PubKey) bool {
	if otherEd, ok := other.(Secp256k1ethPubKey); ok {
		return bytes.Equal(pubKey[:], otherEd[:])
	}

	return false
}
