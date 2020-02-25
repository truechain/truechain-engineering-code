package consensus

import (
	// "math/big"
	"sync"
	"time"
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
		count:	200,
		stop: 	false,
		chanReward: make(chan *rewardInfo,10),
	}
	res.RewardCache = make(map[uint64]*types.TimedChainReward)
	go res.loop()
	return res
}
type rewardInfo struct {
	height 	uint64
	stime   uint64
	infos 	*types.ChainReward
}

type CacheChainReward struct {
	RewardCache		map[uint64]*types.TimedChainReward
	min 		uint64
	max 		uint64
	count 		int
	chanReward  chan *rewardInfo
	stop 		bool
	lock sync.RWMutex
}
func (c *CacheChainReward) minMax() (uint64,uint64,int) {
	min,max := uint64(0),uint64(0)
	c.lock.RLock()
	defer c.lock.RUnlock()
	pos := 0
	for k,_ := range c.RewardCache {
		if pos == 0 {
			min = k
		}
		if min > k {
			min = k
		}		
		if max < k {
			max = k
		}
		pos++ 
	}
	return min,max,pos
}
func (c *CacheChainReward) Stop() {
	c.stop = true
}
func (c *CacheChainReward) AddChainReward(snailBlock,stime uint64,infos *types.ChainReward) {
	item := &rewardInfo{
		height:		snailBlock,
		stime:		stime,
		infos:		infos,
	}
	select {
	case c.chanReward <- item:
	default:
	}
}
func (c *CacheChainReward) loop() {
	for {
		if c.stop {
			return 
		}
		select {
		case item := <- c.chanReward:
			c.insertChainReward(item.height,item.stime,item.infos)
		default:
		}
		time.Sleep(time.Millisecond * time.Duration(500))
	}
}
func (c *CacheChainReward) insertChainReward(snailBlock,st uint64,infos *types.ChainReward) {
	if infos == nil {
		log.Error("AddChainReward: infos is nil","height",snailBlock)
	}
	c.lock.Lock()
	sum := len(c.RewardCache)
	if sum > c.count {
		delete(c.RewardCache,c.min)
	}
	c.RewardCache[snailBlock] = &types.TimedChainReward{
		St:	st,
		Number: snailBlock,
		Reward:infos,
	}
	c.lock.Unlock()
	c.min,c.max,sum = c.minMax()	
	log.Info("AddChainReward","height",snailBlock,"min",c.min,"max",c.max,"count",sum)
}

func (c *CacheChainReward) GetChainReward(snailBlock uint64) *types.TimedChainReward {
	c.lock.RLock()
	defer c.lock.RUnlock()
	
	if int64(snailBlock) <= 0 {
		_,max,_ := c.Summay()
		snailBlock = max
	}
	infos,ok := c.RewardCache[snailBlock]
	if ok {
		return infos
	}
	
	//log.Warn("GetChainReward over the cache","request",snailBlock,"min",min,"max",max,"count",count)
	return nil
}
func (c *CacheChainReward) Summay() (uint64,uint64,int) {
	return c.min,c.max,len(c.RewardCache)
}