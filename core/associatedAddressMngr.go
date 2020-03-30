package core

import (
	"github.com/ethereum/go-ethereum/common"
	lru "github.com/hashicorp/golang-lru"
	"github.com/truechain/truechain-engineering-code/core/state"
)

const (
	associatedAddressCacheLimit = 10240
)

type AssociatedAddressMngr struct {
	lruCache *lru.Cache
}

func NewAssociatedAddressMngr() *AssociatedAddressMngr {
	lruCache, _ := lru.New(associatedAddressCacheLimit)

	return &AssociatedAddressMngr{
		lruCache: lruCache,
	}
}

func (aam *AssociatedAddressMngr) LoadAssociatedAddresses(addrs []common.Address) map[common.Address]*state.TouchedAddressObject {
	result := make(map[common.Address]*state.TouchedAddressObject)

	for _, addr := range addrs {
		if obj, exist := aam.lruCache.Get(addr); exist {
			result[addr] = obj.(*state.TouchedAddressObject)
		}
	}

	return result
}

func (aam *AssociatedAddressMngr) UpdateAssociatedAddresses(associatedAddrs map[common.Address]*state.TouchedAddressObject) {
	for addr, associatedAddr := range associatedAddrs {
		if obj, exist := aam.lruCache.Get(addr); exist {
			touchedAddressObj := obj.(*state.TouchedAddressObject)
			associatedAddr.Merge(touchedAddressObj)
		}
		aam.lruCache.Add(addr, associatedAddrs)
	}
}
