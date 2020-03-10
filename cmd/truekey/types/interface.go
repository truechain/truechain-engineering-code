package types

import (
	"context"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/common/hexutil"
)

const (
	// ExternalAPIVersion
	ExternalAPIVersion = "1.0.0"
)

type UserIdentity struct {
	PubKey string      `json:"pubkey"`
	Prompt common.Hash `json:"id"`
}

// ExternalAPI defines the external API through which signing requests are made.
type ExternalAPI interface {
	//Register(ctx context.Context, pubkey string) (, error)
	// List available accounts
	List(ctx context.Context) ([]common.Address, error)
	// SignHash request to sign the specified transaction
	SignHash(ctx context.Context, addr common.Address, hash hexutil.Bytes) (hexutil.Bytes, error)
	// Version info about the APIs
	Version(ctx context.Context) (string, error)
}

// ServerAPI defines the admin API through which control hd  wallet.
type ServerAPI interface {
	// Register a admin
	RegisterAdmin(ctx context.Context, passphrase string) error
	// Derive account according to passphrase
	DeriveAccounts(ctx context.Context, count uint64, passphrase string) ([]Account, error)
	// Get private key
	//ExportKey(ctx context.Context, id uint64, addr common.Address) (hexutil.Bytes, error)
	// List available accounts
	ChildAddress(ctx context.Context, passphrase string) ([]ChildAccount, error)
	// update account setting
	UpdateAccount(ctx context.Context, passphrase string, id uint64, content AccountState) error
	// update account setting
	ChangeAdmin(ctx context.Context, passphrase string, newPassphrase string) error
}

// UIClientAPI specifies what method a UI needs to implement to be able to be used as a
// UI for the signer
