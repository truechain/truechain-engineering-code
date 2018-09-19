package crypto

import (
	"bytes"
	"crypto/ecdsa"
	"fmt"
	tcrypyo "github.com/truechain/truechain-engineering-code/crypto"
	amino "github.com/truechain/truechain-engineering-code/consensus/tbft/go-amino"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/help"
)

//-------------------------------------
const (
	EcdsaPrivKeyAminoRoute = "true/PrivKeyTrue"
	EcdsaPubKeyAminoRoute  = "true/PubKeyTrue"
	// Size of an Edwards25519 signature. Namely the size of a compressed
	// Edwards25519 point, and a field element. Both of which are 32 bytes.
	SignatureEd25519Size = 64
)
var cdc = amino.NewCodec()

func init() {
	cdc.RegisterInterface((*PubKey)(nil), nil)
	cdc.RegisterConcrete(PubKeyTrue{},
		EcdsaPubKeyAminoRoute, nil)

	cdc.RegisterInterface((*PrivKey)(nil), nil)
	cdc.RegisterConcrete(PrivKeyTrue{},
		EcdsaPrivKeyAminoRoute, nil)
}
// PrivKeyTrue implements PrivKey.
type PrivKeyTrue ecdsa.PrivateKey

// Bytes marshals the privkey using amino encoding.
func (priv PrivKeyTrue) Bytes() []byte {
	return cdc.MustMarshalBinaryBare(priv)
}

// Sign produces a signature on the provided message.
func (priv PrivKeyTrue) Sign(msg []byte) ([]byte, error) {
	priv1 := ecdsa.PrivateKey(priv)
	return tcrypyo.Sign(msg,&priv1)
}

// PubKey gets the corresponding public key from the private key.
func (priv PrivKeyTrue) PubKey() PubKey {
	priv1 := ecdsa.PrivateKey(priv)
	pub0,ok := priv1.Public().(*ecdsa.PublicKey)
	if !ok {
		panic(0)
	}
	pub := PubKeyTrue(*pub0)
	return &pub
}

// Equals - you probably don't need to use this.
// Runs in constant time based on length of the keys.
func (priv PrivKeyTrue) Equals(other PrivKey) bool {
	if otherEd, ok := other.(PrivKeyTrue); ok {
		priv0 := ecdsa.PrivateKey(otherEd)
		data0 := tcrypyo.FromECDSA(&priv0)
		priv1 := ecdsa.PrivateKey(priv)
		data1 := tcrypyo.FromECDSA(&priv1)
		return bytes.Equal(data0[:], data1[:])
	} else {
		return false
	}
}

// GenPrivKey generates a new ed25519 private key.

func GenPrivKey() PrivKeyTrue {
	priv,err := tcrypyo.GenerateKey()
	if err != nil {
		panic(err)
	}
	privKey := PrivKeyTrue(*priv)
	return privKey
}

//-------------------------------------

// PubKeyTrue implements PubKey for the ecdsa.PublicKey signature scheme.
type PubKeyTrue ecdsa.PublicKey

// Address is the Keccak256 of the raw pubkey bytes.
func (pub PubKeyTrue) Address() help.Address {
	pub1 := ecdsa.PublicKey(pub)
	data := tcrypyo.FromECDSAPub(&pub1)
	return help.Address(help.HexBytes(data[:]))
}

// Bytes marshals the PubKey using amino encoding.
func (pub PubKeyTrue) Bytes() []byte {
	bz, err := cdc.MarshalBinaryBare(pub)
	if err != nil {
		panic(err)
	}
	return bz
}

func (pub PubKeyTrue) VerifyBytes(msg []byte, sig_ []byte) bool {
	// make sure we use the same algorithm to sign
	if pub0,err := tcrypyo.SigToPub(msg,sig_); err == nil {
		pub1 := PubKeyTrue(*pub0)
		return pub.Equals(pub1)
	}
	return false
}

func (pub PubKeyTrue) String() string {
	pub1 := ecdsa.PublicKey(pub)
	data := tcrypyo.FromECDSAPub(&pub1)
	if data == nil {
		return ""
	}
	return fmt.Sprintf("PubKeyTrue{%X}", data[:])
}

// nolint: golint
func (pub PubKeyTrue) Equals(other PubKey) bool {
	if otherEd, ok := other.(PubKeyTrue); ok {
		pub0 := ecdsa.PublicKey(otherEd)
		pub1 := ecdsa.PublicKey(pub)
		data0 := tcrypyo.FromECDSAPub(&pub0)
		data1 := tcrypyo.FromECDSAPub(&pub1)
		if data0 == nil || data1 == nil {
			return false
		}
		return bytes.Equal(data0[:], data1[:])
	} else {
		return false
	}
}
