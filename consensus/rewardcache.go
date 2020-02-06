package consensus

import (
	// "math/big"
	"sync"
	// "github.com/truechain/truechain-engineering-code/common"
	// "github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/log"
	// "github.com/truechain/truechain-engineering-code/params"
	// "github.com/truechain/truechain-engineering-code/rpc"
	// "github.com/truechain/truechain-engineering-code/core/vm"
)
var CR *CacheChainReward
func init() {
	CR = newCacheChainReward()
}
func newCacheChainReward() *CacheChainReward{
	res := &CacheChainReward{
		min:	0,
		max:	0,
		count:	100,
	}
	res.RewardCache = make(map[uint64]*types.ChainReward)
	return res
}



type CacheChainReward struct {
	RewardCache		map[uint64]*types.ChainReward
	min 		uint64
	max 		uint64
	count 		int
	lock sync.RWMutex
}
func (c *CacheChainReward) minMax() (uint64,uint64,int) {
	min,max := uint64(0),uint64(0)
	c.lock.RLock()
	defer c.lock.RUnlock()
	for k,_ := range c.RewardCache {
		if min > k {
			min = k
		}		
		if max < k {
			max = k
		}
	}
	return min,max,len(c.RewardCache)
}
func (c *CacheChainReward) AddChainReward(snailBlock uint64,infos *types.ChainReward) {
	if infos == nil {
		log.Error("AddChainReward: infos is nil","height",snailBlock)
	}
	c.lock.Lock()
	sum := len(c.RewardCache)
	if sum > c.count {
		delete(c.RewardCache,c.min)
	}
	c.RewardCache[snailBlock] = infos
	c.lock.Unlock()
	c.min,c.max,sum = c.minMax()	
	log.Info("AddChainReward","height",snailBlock,"min",c.min,"max",c.max,"count",sum)
}

func (c *CacheChainReward) GetChainReward(snailBlock uint64) *types.ChainReward {
	c.lock.RLock()
	defer c.lock.RUnlock()
	infos,ok := c.RewardCache[snailBlock]
	if ok {
		return infos
	}
	min,max,count := c.Summay()
	log.Warn("GetChainReward over the cache","request",snailBlock,"min",min,"max",max,"count",count)
	return nil
}
func (c *CacheChainReward) Summay() (uint64,uint64,int) {
	return c.min,c.max,len(c.RewardCache)
}