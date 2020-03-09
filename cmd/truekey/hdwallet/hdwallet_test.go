package hdwallet

import (
	"fmt"
	"log"
	"math/big"
	"strings"
	"testing"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/common/hexutil"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/crypto"
)

func TestSeedWallet(t *testing.T) {
	seed, _ := NewSeed()
	wallet, err := NewFromSeed(seed)
	if err != nil {
		log.Fatal(err)
	}
	deriveAddress(wallet)
}

func TestMnemonicWallet(t *testing.T) {
	mnemonic := "tag volcano eight thank tide danger coast health above argue embrace heavy"
	wallet, err := NewFromMnemonic(mnemonic)
	if err != nil {
		log.Fatal(err)
	}
	deriveAddress(wallet)
}

func deriveAddress(wallet *Wallet) {
	path := MustParseDerivationPath("m/44'/60'/0'/0/0")
	account, err := wallet.Derive(path, true)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Account address: %s\n", account.Address.Hex())

	privateKey, err := wallet.PrivateKeyHex(account)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Private key in hex: %s\n", privateKey)

	publicKey, _ := wallet.PublicKeyHex(account)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Public key in hex: %s\n", publicKey)

	path = MustParseDerivationPath("m/44'/60'/0'/0/1")
	account, err = wallet.Derive(path, true)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(account.Address.Hex()) // 0x8230645aC28A4EdD1b0B53E7Cd8019744E9dD559
}

func TestWallet(t *testing.T) {
	mnemonic := "tag volcano eight thank tide danger coast health above argue embrace heavy"
	wallet, err := NewFromMnemonic(mnemonic)
	if err != nil {
		t.Error(err)
	}

	path, err := ParseDerivationPath("m/44'/60'/0'/0/0")
	if err != nil {
		t.Error(err)
	}

	account, err := wallet.Derive(path, true)
	if err != nil {
		t.Error(err)
	}

	if len(wallet.Accounts()) != 1 {
		t.Error("expected 1")
	}

	if !wallet.Contains(account) {
		t.Error("expected to contain account")
	}

	url := wallet.URL()
	if url.String() != "" {
		t.Error("expected empty url")
	}

	accountPath, err := wallet.Path(account)
	if err != nil {
		t.Error(err)
	}

	if accountPath != `m/44'/60'/0'/0/0` {
		t.Error("wrong hdpath")
	}

	privateKeyHex, err := wallet.PrivateKeyHex(account)
	if err != nil {
		t.Error(err)
	}

	if privateKeyHex != "63e21d10fd50155dbba0e7d3f7431a400b84b4c2ac1ee38872f82448fe3ecfb9" {
		t.Error("wrong private key")
	}

	publicKeyHex, err := wallet.PublicKeyHex(account)
	if err != nil {
		t.Error(err)
	}

	if publicKeyHex != "6005c86a6718f66221713a77073c41291cc3abbfcd03aa4955e9b2b50dbf7f9b6672dad0d46ade61e382f79888a73ea7899d9419becf1d6c9ec2087c1188fa18" {
		t.Error("wrong public key")
	}

	addressHex, err := wallet.AddressHex(account)
	if err != nil {
		t.Error(err)
	}

	if addressHex != "0xC49926C4124cEe1cbA0Ea94Ea31a6c12318df947" {
		t.Error("wrong address")
	}

	nonce := uint64(0)
	value := big.NewInt(1000000000000000000)
	toAddress := common.HexToAddress("0x0")
	gasLimit := uint64(21000)
	gasPrice := big.NewInt(21000000000)
	data := []byte{}

	tx := types.NewTransaction(nonce, toAddress, value, gasLimit, gasPrice, data)

	signedTx, err := wallet.SignTx(account, tx, big.NewInt(18))
	if err != nil {
		t.Error(err)
	}

	v, r, s := signedTx.RawSignatureValues()
	if v.Cmp(big.NewInt(0)) != 1 {
		t.Error("expected v value")
	}
	if r.Cmp(big.NewInt(0)) != 1 {
		t.Error("expected r value")
	}
	if s.Cmp(big.NewInt(0)) != 1 {
		t.Error("expected s value")
	}

	data = []byte("hello")
	hash := crypto.Keccak256Hash(data)
	sig, err := wallet.SignHash(account, hash.Bytes())
	if err != nil {
		t.Error(err)
	}
	if len(sig) == 0 {
		t.Error("expected signature")
	}

	sig2, err := wallet.SignHashWithPassphrase(account, "", hash.Bytes())
	if err != nil {
		t.Error(err)
	}
	if len(sig2) == 0 {
		t.Error("expected signature")
	}
	if hexutil.Encode(sig) != hexutil.Encode(sig2) {
		t.Error("expected match")
	}

	err = wallet.Unpin(account)
	if err != nil {
		t.Error(err)
	}

	if wallet.Contains(account) {
		t.Error("expected to not contain account")
	}

	seed, err := NewSeedFromMnemonic(mnemonic)
	if err != nil {
		t.Error(err)
	}

	wallet, err = NewFromSeed(seed)
	if err != nil {
		t.Error(err)
	}

	path = MustParseDerivationPath("m/44'/60'/0'/0/0")
	account, err = wallet.Derive(path, false)
	if err != nil {
		t.Error(err)
	}

	if account.Address.Hex() != "0xC49926C4124cEe1cbA0Ea94Ea31a6c12318df947" {
		t.Error("wrong address")
	}

	seed, err = NewSeed()
	if err != nil {
		t.Error(err)
	}

	if len(seed) != 64 {
		t.Error("expected size of 64")
	}

	seed, err = NewSeedFromMnemonic(mnemonic)
	if err != nil {
		t.Error(err)
	}

	if len(seed) != 64 {
		t.Error("expected size of 64")
	}

	mnemonic, err = NewMnemonic(128)
	if err != nil {
		t.Error(err)
	}

	words := strings.Split(mnemonic, " ")
	if len(words) != 12 {
		t.Error("expected 12 words")
	}

	entropy, err := NewEntropy(128)
	if err != nil {
		t.Error(err)
	}

	mnemonic, err = NewMnemonicFromEntropy(entropy)
	if err != nil {
		t.Error(err)
	}

	if len(words) != 12 {
		t.Error("expected 12 words")
	}
}
