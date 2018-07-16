package truechain

import (
	"testing"
	"encoding/hex"
	"fmt"
	"github.com/ethereum/go-ethereum/crypto"
	"math/big"
	"crypto/ecdsa"
	"math/rand"
	"time"
)
var msg *CdEncryptionMsg


func TestMemberToMsg(t *testing.T) {
	priv := privkeys[0]
	member :=GenerateMember(priv,0)
	var err error
	msg,err = ConvertCdMemberToMsg(member,priv)
	if err != nil {
		fmt.Println("makeCryptoMsg failed,err=",err)
	}
	fmt.Println("cmsg:",msg)
}

func TestMsgToMember(t *testing.T) {
	fmt.Println("msg:",msg)
	member := msg.convertMsgToCdMember()
	fmt.Println("member:",member)
}

//////////////////////////////////////////////
func TestEncryptionMsgInTrueChain(t *testing.T) {
	tmp := struct {
		msg1	[]string
		msg2	[]byte
		msg3	int
		msg4 	float64
	}{
		msg3:	5,
		msg4:	4.5,
	}
	priv := privkeys[0]

	pub :=hex.EncodeToString(crypto.FromECDSAPub(GetPub(priv)))
	hash := rlpHash(tmp)

	// sig message use priv
	sig,err := crypto.Sign(hash[:],priv)
	// verify message
	if err == nil {
		verifyPub, err := crypto.SigToPub(hash[:], sig)
		if err == nil {
			strverifyPub := hex.EncodeToString(crypto.FromECDSAPub(verifyPub))

			if pub == strverifyPub {
				fmt.Println("sign test seccess,pub = ", pub, " strverifyPub = ", strverifyPub)

			}
		}
	}
}