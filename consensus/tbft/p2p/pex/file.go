package pex

import (
	"encoding/json"
	"fmt"
	"os"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/help"
)

/* Loading & Saving */

type addrBookJSON struct {
	Key   string          `json:"key"`
	Addrs []*knownAddress `json:"addrs"`
}

func (a *addrBook) saveToFile(filePath string) {
	log.Info("Saving AddrBook to file", "size", a.Size())

	a.mtx.Lock()
	defer a.mtx.Unlock()
	// Compile Addrs
	addrs := []*knownAddress{}
	for _, ka := range a.addrLookup {
		addrs = append(addrs, ka)
	}

	aJSON := &addrBookJSON{
		Key:   a.key,
		Addrs: addrs,
	}

	jsonBytes, err := json.MarshalIndent(aJSON, "", "\t")
	if err != nil {
		log.Error("Failed to save AddrBook to file", "err", err)
		return
	}
	err = help.WriteFileAtomic(filePath, jsonBytes, 0644)
	if err != nil {
		log.Error("Failed to save AddrBook to file", "file", filePath, "err", err)
	}
}

// Returns false if file does not exist.
// help.Panics if file is corrupt.
func (a *addrBook) loadFromFile(filePath string) bool {
	// If doesn't exist, do nothing.
	_, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return false
	}

	// Load addrBookJSON{}
	r, err := os.Open(filePath)
	if err != nil {
		help.PanicSanity(fmt.Sprintf("Error opening file %s: %v", filePath, err))
	}
	defer r.Close() // nolint: errcheck
	aJSON := &addrBookJSON{}
	dec := json.NewDecoder(r)
	err = dec.Decode(aJSON)
	if err != nil {
		help.PanicSanity(fmt.Sprintf("Error reading file %s: %v", filePath, err))
	}

	// Restore all the fields...
	// Restore the key
	a.key = aJSON.Key
	// Restore .bucketsNew & .bucketsOld
	for _, ka := range aJSON.Addrs {
		for _, bucketIndex := range ka.Buckets {
			bucket := a.getBucket(ka.BucketType, bucketIndex)
			bucket[ka.Addr.String()] = ka
		}
		a.addrLookup[ka.ID()] = ka
		if ka.BucketType == bucketTypeNew {
			a.nNew++
		} else {
			a.nOld++
		}
	}
	return true
}
