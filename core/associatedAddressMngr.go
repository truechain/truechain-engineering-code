package core

import (
	"github.com/ethereum/go-ethereum/common"
	lru "github.com/hashicorp/golang-lru"
	"github.com/truechain/truechain-engineering-code/core/state"
	"google.golang.org/genproto/googleapis/spanner/admin/database/v1"
)

const (
	associatedAddressCacheLimit = 10240
)

type AssociatedAddressMngr struct {
	lruCache *lru.Cache
	db       database.Database
}

func NewAssociatedAddressMngr() *AssociatedAddressMngr {
	lruCache, _ := lru.New(associatedAddressCacheLimit)

	return &AssociatedAddressMngr{
		lruCache: lruCache,
		//db: db,
	}
}

func (aam *AssociatedAddressMngr) LoadAssociatedAddresses(addrs []common.Address) map[common.Address]*state.TouchedAddressObject {
	result := make(map[common.Address]*state.TouchedAddressObject)
	var addrsToLoad []common.Address

	for _, addr := range addrs {
		if obj, exist := aam.lruCache.Get(addr); !exist {
			addrsToLoad = append(addrsToLoad, addr)
		} else {
			result[addr] = obj.(*state.TouchedAddressObject)
		}
	}

	loadedResult := aam.loadFromDatabase(addrsToLoad)
	if loadedResult != nil {
		for k, v := range loadedResult {
			result[k] = v
			aam.lruCache.Add(k, v)
		}
	}

	return result
}

func (aam *AssociatedAddressMngr) loadFromDatabase(addrs []common.Address) map[common.Address]*state.TouchedAddressObject {
	// TODO
	return nil
}

func (aam *AssociatedAddressMngr) UpdateAssociatedAddresses(associatedAddrs map[common.Address]*state.TouchedAddressObject) {
	updatedTouchedAddrs := make(map[common.Address]*state.TouchedAddressObject)
	for addr, associatedAddr := range associatedAddrs {
		// remove accounts which come from args
		associatedAddr.RemoveAccountsInArgs()

		if obj, exist := aam.lruCache.Get(addr); !exist {
			aam.lruCache.Add(addr, associatedAddr)
			updatedTouchedAddrs[addr] = associatedAddr
		} else {
			touchedAddressObj := obj.(*state.TouchedAddressObject)
			if changed := touchedAddressObj.Merge(associatedAddr); changed {
				updatedTouchedAddrs[addr] = touchedAddressObj
			}
		}
	}
	go aam.saveToDatabase(updatedTouchedAddrs)
}

func (aam *AssociatedAddressMngr) saveToDatabase(associatedAddrs map[common.Address]*state.TouchedAddressObject) {
	// TODO
}
