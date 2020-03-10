package types

import (
	"github.com/truechain/truechain-engineering-code/accounts/keystore"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/rlp"
	"io"
	"net"
	"time"
)

type AdminWallet struct {
	Info      string
	Accounts  map[common.Address]*ChildAccount
	keystore  *keystore.KeyStore
	Hash      common.Hash
	whitelist []common.Hash
}

// "external" AdminWallet encoding. used for pos hd.
type extAdminWallet struct {
	Info     string
	Accounts []*ChildAccount
	Array    []common.Address
	Hash     common.Hash
}

func (i *AdminWallet) DecodeRLP(s *rlp.Stream) error {
	var ei extAdminWallet
	if err := s.Decode(&ei); err != nil {
		return err
	}
	aAccounts := make(map[common.Address]*ChildAccount)
	for i, account := range ei.Accounts {
		aAccounts[ei.Array[i]] = account
	}

	i.Info, i.Accounts, i.Hash = ei.Info, aAccounts, ei.Hash
	return nil
}

// EncodeRLP serializes b into the truechain RLP AdminWallet format.
func (i *AdminWallet) EncodeRLP(w io.Writer) error {
	var aAccounts []*ChildAccount
	var order []common.Address
	for i, _ := range i.Accounts {
		order = append(order, i)
	}
	for m := 0; m < len(order)-1; m++ {
		for n := 0; n < len(order)-1-m; n++ {
			if order[n].Big().Cmp(order[n+1].Big()) > 0 {
				order[n], order[n+1] = order[n+1], order[n]
			}
		}
	}
	for _, epoch := range order {
		aAccounts = append(aAccounts, i.Accounts[epoch])
	}
	return rlp.Encode(w, extAdminWallet{
		Info:     i.Info,
		Accounts: aAccounts,
		Array:    order,
		Hash:     i.Hash,
	})
}

func NewAdminWallet(info string, keystore *keystore.KeyStore, hash common.Hash) *AdminWallet {
	return &AdminWallet{
		Info:     info,
		Accounts: make(map[common.Address]*ChildAccount),
		keystore: keystore,
		Hash:     hash,
	}
}

func (aw *AdminWallet) Keystore() *keystore.KeyStore {
	return aw.keystore
}

func (aw *AdminWallet) AccountArray() []*ChildAccount {
	var accs []*ChildAccount
	for _, child := range aw.Accounts {
		accs = append(accs, child)
	}
	return accs
}

type ChildAccount struct {
	ID        uint64         `json:"id"`
	Address   common.Address `json:"address"` // Ethereum account address derived from the key
	IPs       []net.IP       `json:"ips"`
	Lock      bool           `json:"lock"`
	Timestamp time.Time      `json:"timestamp"`
	Note      string         `json:"note"`
}

type AccountState struct {
	IPs  []net.IP `json:"ips"`
	Lock bool     `json:"lock"`
	Note string   `json:"note"`
}
