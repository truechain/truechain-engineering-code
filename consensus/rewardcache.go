package consensus

import (
	"math/big"
	"sync"
	"github.com/truechain/truechain-engineering-code/common"
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
	res.RewardCache = make(map[uint64]*ChainReward)
	return res
}

type ChainReward struct {
	Foundation		*types.RewardInfo
	CoinBase 		*types.RewardInfo
	FruitBase 		[]*types.RewardInfo
	CommitteeBase 	[]*types.SARewardInfos
}
func NewChainReward(found,coin *types.RewardInfo,fruits []*types.RewardInfo,committee []*types.SARewardInfos) *ChainReward{
	return &ChainReward{
		Foundation: found,
		CoinBase: 	coin,
		FruitBase:fruits,
		CommitteeBase:committee,
	}
}
func ToRewardInfos1(items map[common.Address]*big.Int) []*types.RewardInfo {
	infos := make([]*types.RewardInfo,0,0)
	for k,v := range items {
		infos = append(infos,&types.RewardInfo{
			Address:	k,
			Amount: 	new(big.Int).Set(v),
		})
	}
	return infos
}
func ToRewardInfos2(items map[common.Address]*big.Int) []*types.SARewardInfos {
	infos := make([]*types.SARewardInfos,0,0)
	for k,v := range items {
		items := []*types.RewardInfo{&types.RewardInfo{
			Address:	k,
			Amount: 	new(big.Int).Set(v),
		}}
		
		infos = append(infos,&types.SARewardInfos{
			Items:	items,
		})
	}
	return infos
}
func MergeReward(map1,map2 map[common.Address]*big.Int) map[common.Address]*big.Int {
	for k,v := range map2 {
		if vv,ok := map1[k];ok {
			map1[k] = new(big.Int).Add(vv,v)
		} else {
			map1[k] = v
		}
	}
	return map1
}

type CacheChainReward struct {
	RewardCache		map[uint64]*ChainReward
	min 		uint64
	max 		uint64
	count 		int
	lock sync.Mutex
}
func (c *CacheChainReward) minMax() (uint64,uint64,int) {
	min,max := uint64(0),uint64(0)
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
func (c *CacheChainReward) AddChainReward(snailBlock uint64,infos *ChainReward) {
	if infos == nil {
		log.Error("AddChainReward: infos is nil","height",snailBlock)
	}
	sum := len(c.RewardCache)
	if sum > c.count {
		delete(c.RewardCache,c.min)
	}
	c.RewardCache[snailBlock] = infos
	c.min,c.max,sum = c.minMax()	
	log.Info("AddChainReward","height",snailBlock,"min",c.min,"max",c.max,"count",sum)
}

func (c *CacheChainReward) GetChainReward(snailBlock uint64) *ChainReward {
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