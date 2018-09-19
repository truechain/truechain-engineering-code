package etrue

import (
	"testing"

	"github.com/truechain/truechain-engineering-code/crypto"
	"fmt"
	"github.com/truechain/truechain-engineering-code/common"
	)

func TestStart(t *testing.T) {

	prvKey,_:=crypto.GenerateKey();

	fmt.Println(common.Bytes2Hex(crypto.FromECDSA(prvKey)))

	fmt.Println(common.Bytes2Hex(crypto.FromECDSAPub(&prvKey.PublicKey)))

	fmt.Println(common.Bytes2Hex(crypto.PubkeyToAddress(prvKey.PublicKey).Bytes()))



}