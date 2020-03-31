package vm

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"sort"
	// "sync"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/params"
	"github.com/truechain/truechain-engineering-code/rlp"
	lru "github.com/hashicorp/golang-lru"
)

/////////////////////////////////////////////////////////////////////////////////
var IC *ImpawnCache

func init() {
	IC = newImpawnCache()
}

type ImpawnCache struct {
	Cache 		*lru.Cache
	size 		int
}

func newImpawnCache() *ImpawnCache {
	cc := &ImpawnCache{
		size:	20,
	}
	cc.Cache,_ = lru.New(cc.size)
	return cc
}
/////////////////////////////////////////////////////////////////////////////////
type PairstakingValue struct {
	Amount *big.Int
	Height *big.Int
	State  uint8
}

func (v *PairstakingValue) isElection() bool {
	return (v.State&types.StateStakingOnce != 0 || v.State&types.StateStakingAuto != 0)
}

type RewardItem struct {
	Amount *big.Int
	Height *big.Int
}
type RedeemItem struct {
	Amount  *big.Int
	EpochID uint64
	State   uint8
}

func (r *RedeemItem) toHeight() *big.Int {
	e := types.GetEpochFromID(r.EpochID + 1)
	return new(big.Int).SetUint64(e.BeginHeight)
}
func (r *RedeemItem) fromHeight(hh *big.Int) {
	e := types.GetEpochFromHeight(hh.Uint64())
	if e != nil {
		r.EpochID = e.EpochID
	}
}
func (r *RedeemItem) update(o *RedeemItem) {
	if r.EpochID == o.EpochID {
		r.Amount = r.Amount.Add(r.Amount, o.Amount)
	}
}
func (r *RedeemItem) isRedeem(target uint64) bool {
	hh := r.toHeight().Uint64()
	return target > hh+params.MaxRedeemHeight
}
func newRedeemItem(eid uint64, amount *big.Int) *RedeemItem {
	return &RedeemItem{
		Amount:  new(big.Int).Set(amount),
		EpochID: eid,
	}
}

type impawnUnit struct {
	Address    common.Address
	Value      []*PairstakingValue // sort by height
	RedeemInof []*RedeemItem
}

func (s *impawnUnit) getAllStaking(hh uint64) *big.Int {
	all := big.NewInt(0)
	for _, v := range s.Value {
		if v.Height.Uint64() <= hh {
			all = all.Add(all, v.Amount)
		} else {
			break
		}
	}
	return all
}
func (s *impawnUnit) getValidStaking(hh uint64) *big.Int {
	all := big.NewInt(0)
	for _, v := range s.Value {
		if v.Height.Uint64() <= hh {
			if v.isElection() {
				all = all.Add(all, v.Amount)
			}
		} else {
			break
		}
	}
	e := types.GetEpochFromHeight(hh)
	r := s.getRedeemItem(e.EpochID)
	if r != nil {
		res := new(big.Int).Sub(all, r.Amount)
		if res.Sign() >= 0 {
			return res
		} else {
			log.Error("getValidStaking error", "all amount", all, "redeem amount", r.Amount, "height", hh)
			return big.NewInt(0)
		}
	}

	return all
}
func (s *impawnUnit) getValidRedeem(hh uint64) *big.Int {
	all := big.NewInt(0)
	for _, v := range s.RedeemInof {
		if v.isRedeem(hh) {
			all = all.Add(all, v.Amount)
		}
	}
	return all
}
func (s *impawnUnit) GetRewardAddress() common.Address {
	return s.Address
}
func (s *impawnUnit) getRedeemItem(epochID uint64) *RedeemItem {
	for _, v := range s.RedeemInof {
		if v.EpochID == epochID {
			return v
		}
	}
	return nil
}

// stopStakingInfo redeem delay in next epoch + MaxRedeemHeight
func (s *impawnUnit) stopStakingInfo(amount, lastHeight *big.Int) error {
	all := s.getValidStaking(lastHeight.Uint64())
	if all.Cmp(amount) < 0 {
		return types.ErrAmountOver
	}
	e := types.GetEpochFromHeight(lastHeight.Uint64())
	if e == nil {
		return types.ErrNotFoundEpoch
	}
	r := s.getRedeemItem(e.EpochID)
	tmp := &RedeemItem{
		Amount:  new(big.Int).Set(amount),
		EpochID: e.EpochID,
		State:   types.StateRedeem,
	}
	if r == nil {
		s.RedeemInof = append(s.RedeemInof, tmp)
	} else {
		r.update(tmp)
	}
	return nil
}
func (s *impawnUnit) redeeming(hh uint64, amount *big.Int) (common.Address, *big.Int, error) {
	if amount.Cmp(s.getValidRedeem(hh)) > 0 {
		return common.Address{}, nil, types.ErrAmountOver
	}
	allAmount := big.NewInt(0)
	s.sortRedeemItems()
	for _, v := range s.RedeemInof {
		if v.isRedeem(hh) {
			allAmount = allAmount.Add(allAmount, v.Amount)
			res := allAmount.Cmp(amount)
			if res <= 0 {
				v.Amount, v.State = v.Amount.Sub(v.Amount, v.Amount), types.StateRedeemed
				if res == 0 {
					break
				}
			} else {
				v.State = types.StateRedeemed
				v.Amount.Set(new(big.Int).Sub(allAmount, amount))
				break
			}
		}
	}
	res := allAmount.Cmp(amount)
	if res >= 0 {
		return s.Address, amount, nil
	} else {
		return s.Address, allAmount, nil
	}
}

// called by user input and it will be execute without wait for the staking be rewarded
func (s *impawnUnit) finishRedeemed() {
	pos := -1
	for i, v := range s.RedeemInof {
		if v.Amount.Sign() == 0 && v.State == types.StateRedeemed {
			pos = i
		}
	}
	if len(s.RedeemInof)-1 == pos {
		s.RedeemInof = s.RedeemInof[0:0]
	} else {
		s.RedeemInof = s.RedeemInof[pos+1:]
	}
}

// sort the redeemInof by asc with epochid
func (s *impawnUnit) sortRedeemItems() {
	sort.Sort(redeemByID(s.RedeemInof))
}

// merge for move from prev to next epoch,move the staking who was to be voted.
// merge all staking to one staking with the new height(the beginning of next epoch).
// it will remove the staking which was canceled in the prev epoch
// called by move function.
func (s *impawnUnit) merge(epochid, hh uint64) {

	all := big.NewInt(0)
	for _, v := range s.Value {
		all = all.Add(all, v.Amount)
	}
	tmp := &PairstakingValue{
		Amount: all,
		Height: new(big.Int).SetUint64(hh),
		State:  types.StateStakingAuto,
	}
	var val []*PairstakingValue
	redeem := s.getRedeemItem(epochid)
	if redeem != nil {
		left := all.Sub(all, redeem.Amount)
		if left.Sign() >= 0 {
			tmp.Amount = left
		} else {
			panic("big error" + fmt.Sprint("all:", all, "redeem:", redeem.Amount, "epoch:", epochid))
		}
	}
	val = append(val, tmp)
	s.Value = val
}
func (s *impawnUnit) update(unit *impawnUnit, move bool) {
	sorter := valuesByHeight(s.Value)
	sort.Sort(sorter)
	for _, v := range unit.Value {
		sorter = sorter.update(v)
	}
	s.Value = sorter

	if s.RedeemInof == nil {
		s.RedeemInof = make([]*RedeemItem, 0)
	}
	if move {
		var tmp []*RedeemItem
		s.RedeemInof = append(append(tmp, unit.RedeemInof...), s.RedeemInof...)
	}
}
func (s *impawnUnit) clone() *impawnUnit {
	tmp := &impawnUnit{
		Address:    s.Address,
		Value:      make([]*PairstakingValue, 0),
		RedeemInof: make([]*RedeemItem, 0),
	}
	for _, v := range s.Value {
		tmp.Value = append(tmp.Value, &PairstakingValue{
			Amount: new(big.Int).Set(v.Amount),
			Height: new(big.Int).Set(v.Height),
			State:  v.State,
		})
	}
	for _, v := range s.RedeemInof {
		tmp.RedeemInof = append(tmp.RedeemInof, &RedeemItem{
			Amount:  new(big.Int).Set(v.Amount),
			EpochID: v.EpochID,
			State:   v.State,
		})
	}
	return tmp
}
func (s *impawnUnit) sort() {
	sort.Sort(valuesByHeight(s.Value))
	s.sortRedeemItems()
}
func (s *impawnUnit) isValid() bool {
	for _, v := range s.Value {
		if v.Amount.Sign() > 0 {
			return true
		}
	}
	for _, v := range s.RedeemInof {
		if v.Amount.Sign() > 0 {
			return true
		}
	}
	return false
}
func (s *impawnUnit) valueToMap() map[uint64]*big.Int {
	res := make(map[uint64]*big.Int)
	for _, v := range s.Value {
		res[v.Height.Uint64()] = new(big.Int).Set(v.Amount)
	}
	return res
}
func (s *impawnUnit) redeemToMap() map[uint64]*big.Int {
	res := make(map[uint64]*big.Int)
	for _, v := range s.RedeemInof {
		res[v.EpochID] = new(big.Int).Set(v.Amount)
	}
	return res
}

/////////////////////////////////////////////////////////////////////////////////

type DelegationAccount struct {
	SaAddress common.Address
	Unit      *impawnUnit
}

func (d *DelegationAccount) update(da *DelegationAccount, move bool) {
	d.Unit.update(da.Unit, move)
}
func (s *DelegationAccount) getAllStaking(hh uint64) *big.Int {
	return s.Unit.getAllStaking(hh)
}
func (s *DelegationAccount) getValidStaking(hh uint64) *big.Int {
	return s.Unit.getValidStaking(hh)
}
func (s *DelegationAccount) stopStakingInfo(amount, lastHeight *big.Int) error {
	return s.Unit.stopStakingInfo(amount, lastHeight)
}
func (s *DelegationAccount) redeeming(hh uint64, amount *big.Int) (common.Address, *big.Int, error) {
	return s.Unit.redeeming(hh, amount)
}
func (s *DelegationAccount) finishRedeemed() {
	s.Unit.finishRedeemed()
}
func (s *DelegationAccount) merge(epochid, hh uint64) {
	s.Unit.merge(epochid, hh)
}
func (s *DelegationAccount) clone() *DelegationAccount {
	return &DelegationAccount{
		SaAddress: s.SaAddress,
		Unit:      s.Unit.clone(),
	}
}
func (s *DelegationAccount) isValid() bool {
	return s.Unit.isValid()
}

type StakingAccount struct {
	Unit       *impawnUnit
	Votepubkey []byte
	Fee        *big.Int
	Committee  bool
	Delegation []*DelegationAccount
	Modify     *AlterableInfo
}
type AlterableInfo struct {
	Fee        *big.Int
	VotePubkey []byte
}

func (s *StakingAccount) isInCommittee() bool {
	return s.Committee
}
func (s *StakingAccount) addAmount(height uint64, amount *big.Int) {
	unit := &impawnUnit{
		Address: s.Unit.Address,
		Value: []*PairstakingValue{&PairstakingValue{
			Amount: new(big.Int).Set(amount),
			Height: new(big.Int).SetUint64(height),
			State:  types.StateStakingAuto,
		}},
		RedeemInof: make([]*RedeemItem, 0),
	}
	s.Unit.update(unit, false)
}
func (s *StakingAccount) updateFee(height uint64, fee *big.Int) {
	if height > s.getMaxHeight() {
		s.Modify.Fee = new(big.Int).Set(fee)
	}
}
func (s *StakingAccount) updatePk(height uint64, pk []byte) {
	if height > s.getMaxHeight() {
		s.Modify.VotePubkey = types.CopyVotePk(pk)
	}
}
func (s *StakingAccount) update(sa *StakingAccount, hh uint64, next, move bool) {
	s.Unit.update(sa.Unit, move)
	dirty := false
	for _, v := range sa.Delegation {
		da := s.getDA(v.Unit.GetRewardAddress())
		if da == nil {
			s.Delegation = append(s.Delegation, v)
			dirty = true
		} else {
			da.update(v, move)
		}
	}
	// ignore the pk param
	if hh > s.getMaxHeight() && s.Modify != nil && sa.Modify != nil {
		if sa.Modify.Fee != nil {
			s.Modify.Fee = new(big.Int).Set(sa.Modify.Fee)
		}
		if sa.Modify.VotePubkey != nil {
			s.Modify.VotePubkey = types.CopyVotePk(sa.Modify.VotePubkey)
		}
	}
	if next {
		s.changeAlterableInfo()
	}
	if dirty && hh != 0 {
		tmp := toDelegationByAmount(hh, false, s.Delegation)
		sort.Sort(tmp)
		s.Delegation, _ = fromDelegationByAmount(tmp)
	}
}
func (s *StakingAccount) stopStakingInfo(amount, lastHeight *big.Int) error {
	return s.Unit.stopStakingInfo(amount, lastHeight)
}
func (s *StakingAccount) redeeming(hh uint64, amount *big.Int) (common.Address, *big.Int, error) {
	return s.Unit.redeeming(hh, amount)
}
func (s *StakingAccount) finishRedeemed() {
	s.Unit.finishRedeemed()
}
func (s *StakingAccount) getAllStaking(hh uint64) *big.Int {
	all := s.Unit.getAllStaking(hh)
	for _, v := range s.Delegation {
		all = all.Add(all, v.getAllStaking(hh))
	}
	return all
}
func (s *StakingAccount) getValidStaking(hh uint64) *big.Int {
	all := s.Unit.getValidStaking(hh)
	for _, v := range s.Delegation {
		all = all.Add(all, v.getValidStaking(hh))
	}
	return all
}
func (s *StakingAccount) getValidStakingOnly(hh uint64) *big.Int {
	return s.Unit.getValidStaking(hh)
}
func (s *StakingAccount) merge(epochid, hh uint64) {
	s.Unit.merge(epochid, hh)
	for _, v := range s.Delegation {
		v.merge(epochid, hh)
	}
}
func (s *StakingAccount) getDA(addr common.Address) *DelegationAccount {
	for _, v := range s.Delegation {
		if bytes.Equal(v.Unit.Address.Bytes(), addr.Bytes()) {
			return v
		}
	}
	return nil
}
func (s *StakingAccount) getMaxHeight() uint64 {
	l := len(s.Unit.Value)
	return s.Unit.Value[l-1].Height.Uint64()
}
func (s *StakingAccount) changeAlterableInfo() {
	if s.Modify != nil {
		if s.Modify.Fee != nil {
			// s.fee = new(big.Int).Set(s.modify.fee)
			s.Fee = s.Modify.Fee
		}
		if s.Modify.VotePubkey != nil {
			s.Votepubkey = s.Modify.VotePubkey
		}
	}
}
func (s *StakingAccount) clone() *StakingAccount {
	ss := &StakingAccount{
		Votepubkey: types.CopyVotePk(s.Votepubkey),
		Unit:       s.Unit.clone(),
		Fee:        new(big.Int).Set(s.Fee),
		Committee:  s.Committee,
		Delegation: make([]*DelegationAccount, 0),
		Modify:     &AlterableInfo{},
	}
	for _, v := range s.Delegation {
		ss.Delegation = append(ss.Delegation, v.clone())
	}
	if s.Modify != nil {
		if s.Modify.Fee != nil {
			ss.Modify.Fee = new(big.Int).Set(s.Modify.Fee)
		}
		if s.Modify.VotePubkey != nil {
			ss.Modify.VotePubkey = types.CopyVotePk(s.Modify.VotePubkey)
		}
	}
	return ss
}
func (s *StakingAccount) isvalid() bool {
	for _, v := range s.Delegation {
		if v.isValid() {
			return true
		}
	}
	return s.Unit.isValid()
}

type SAImpawns []*StakingAccount

func (s *SAImpawns) getAllStaking(hh uint64) *big.Int {
	all := big.NewInt(0)
	for _, val := range *s {
		all = all.Add(all, val.getAllStaking(hh))
	}
	return all
}
func (s *SAImpawns) getValidStaking(hh uint64) *big.Int {
	all := big.NewInt(0)
	for _, val := range *s {
		all = all.Add(all, val.getValidStaking(hh))
	}
	return all
}
func (s *SAImpawns) sort(hh uint64, valid bool) {
	for _, v := range *s {
		tmp := toDelegationByAmount(hh, valid, v.Delegation)
		sort.Sort(tmp)
		v.Delegation, _ = fromDelegationByAmount(tmp)
	}
	tmp := toStakingByAmount(hh, valid, *s)
	sort.Sort(tmp)
	*s, _ = fromStakingByAmount(tmp)
}
func (s *SAImpawns) getSA(addr common.Address) *StakingAccount {
	for _, val := range *s {
		if bytes.Equal(val.Unit.Address.Bytes(), addr.Bytes()) {
			return val
		}
	}
	return nil
}
func (s *SAImpawns) update(sa1 *StakingAccount, hh uint64, next, move bool,effectHeight uint64) {
	sa := s.getSA(sa1.Unit.Address)
	if sa == nil {
		if hh >= effectHeight {
			sa1.changeAlterableInfo()
		}
		*s = append(*s, sa1)
		s.sort(hh, false)
	} else {
		sa.update(sa1, hh, next, move)
	}
}

/////////////////////////////////////////////////////////////////////////////////
// be thread-safe for caller locked
type ImpawnImpl struct {
	accounts   map[uint64]SAImpawns // key is epoch id,value is SA set
	curEpochID uint64               // the new epochid of the current state
	lastReward uint64               // the curnent reward height block
}

func NewImpawnImpl() *ImpawnImpl {
	pre := types.GetPreFirstEpoch()
	return &ImpawnImpl{
		curEpochID: pre.EpochID,
		lastReward: 0,
		accounts:   make(map[uint64]SAImpawns),
	}
}
func CloneImpawnImpl(ori *ImpawnImpl) *ImpawnImpl {
	if ori == nil {
		return nil
	}
	tmp := &ImpawnImpl{
		curEpochID: ori.curEpochID,
		lastReward: ori.lastReward,
		accounts:   make(map[uint64]SAImpawns),
	}
	for k,val := range ori.accounts {
		items := SAImpawns{}
		for _, v := range val {
			vv := v.clone()
			items = append(items,vv)
		}
		tmp.accounts[k] = items
	}
	return tmp
}
/////////////////////////////////////////////////////////////////////////////////
///////////  auxiliary function ////////////////////////////////////////////
func (i *ImpawnImpl) getCurrentEpoch() uint64 {
	return i.curEpochID
}
func (i *ImpawnImpl) SetCurrentEpoch(eid uint64) {
	i.curEpochID = eid
}
func (i *ImpawnImpl) getMinEpochID() uint64 {
	eid := i.curEpochID
	for k, _ := range i.accounts {
		if eid > k {
			eid = k
		}
	}
	return eid
}
func (i *ImpawnImpl) isInCurrentEpoch(hh uint64) bool {
	return i.curEpochID == types.GetEpochFromHeight(hh).EpochID
}
func (i *ImpawnImpl) getCurrentEpochInfo() []*types.EpochIDInfo {
	var epochs []*types.EpochIDInfo
	var eids []float64
	for k, _ := range i.accounts {
		eids = append(eids, float64(k))
	}
	sort.Float64s(eids)
	for _, v := range eids {
		e := types.GetEpochFromID(uint64(v))
		if e != nil {
			epochs = append(epochs, e)
		}
	}
	return epochs
}
func (i *ImpawnImpl) repeatPK(addr common.Address,pk []byte) bool {
	for _,v := range i.accounts {
		for _,vv := range v {
			if !bytes.Equal(addr.Bytes(),vv.Unit.Address.Bytes()) && bytes.Equal(pk,vv.Votepubkey) {
				return true
			}
		}
	}
	return false
}
func (i *ImpawnImpl) GetStakingAccount(epochid uint64, addr common.Address) (*StakingAccount, error) {
	if v, ok := i.accounts[epochid]; !ok {
		return nil, types.ErrInvalidStaking
	} else {
		for _, val := range v {
			if bytes.Equal(val.Unit.Address.Bytes(), addr.Bytes()) {
				return val, nil
			}
		}
	}
	return nil, types.ErrInvalidStaking
}
func (i *ImpawnImpl) getDAfromSA(sa *StakingAccount, addr common.Address) (*DelegationAccount, error) {
	for _, ii := range sa.Delegation {
		if bytes.Equal(ii.Unit.Address.Bytes(), addr.Bytes()) {
			return ii, nil
		}
	}
	return nil, nil
}
func (i *ImpawnImpl) getElections(epochid uint64) []common.Address {
	if accounts, ok := i.accounts[epochid]; !ok {
		return nil
	} else {
		var addrs []common.Address
		for _, v := range accounts {
			if v.isInCommittee() {
				addrs = append(addrs, v.Unit.GetRewardAddress())
			}
		}
		return addrs
	}
}
func (i *ImpawnImpl) getElections2(epochid uint64) []*StakingAccount {
	if accounts, ok := i.accounts[epochid]; !ok {
		return nil
	} else {
		var sas []*StakingAccount
		for _, v := range accounts {
			if v.isInCommittee() {
				sas = append(sas, v)
			}
		}
		return sas
	}
}
func (i *ImpawnImpl) getElections3(epochid uint64) []*StakingAccount {
	eid := epochid
	if eid >= params.FirstNewEpochID {
		eid = eid - 1
	}
	return i.getElections2(eid)
}
func (i *ImpawnImpl) fetchAccountsInEpoch(epochid uint64, addrs []*StakingAccount) []*StakingAccount {
	if accounts, ok := i.accounts[epochid]; !ok {
		return addrs
	} else {
		find := func(addrs []*StakingAccount, addr common.Address) bool {
			for _, v := range addrs {
				if bytes.Equal(v.Unit.Address.Bytes(), addr.Bytes()) {
					return true
				}
			}
			return false
		}
		items := make([]*StakingAccount,0,0)
		for _, val := range accounts {
			if find(addrs, val.Unit.GetRewardAddress()) {
				items = append(items, val)
			}
		}
		return items
	}
}
func (i *ImpawnImpl) redeemBySa(sa *StakingAccount, height uint64, amount *big.Int) error {
	// can be redeem in the SA
	_, all, err1 := sa.redeeming(height, amount)
	if err1 != nil {
		return err1
	}
	if all.Cmp(amount) != 0 {
		return errors.New(fmt.Sprint(types.ErrRedeemAmount, "request amount", amount, "redeem amount", all))
	}
	sa.finishRedeemed()
	// fmt.Println("SA redeemed amount:[", all.String(), "],addr:[", addr.String())
	return nil
}
func (i *ImpawnImpl) redeemByDa(da *DelegationAccount, height uint64, amount *big.Int) error {
	// can be redeem in the DA
	_, all, err1 := da.redeeming(height, amount)
	if err1 != nil {
		return err1
	}
	if all.Cmp(amount) != 0 {
		return errors.New(fmt.Sprint(types.ErrRedeemAmount, "request amount", amount, "redeem amount", all))
	}
	da.finishRedeemed()
	// fmt.Println("DA redeemed amount:[", all.String(), "],addr:[", addr.String())
	return nil
}
func (i *ImpawnImpl) calcRewardInSa(target uint64, sa *StakingAccount, allReward, allStaking *big.Int, item *types.RewardInfo) ([]*types.RewardInfo, error) {
	if sa == nil || allReward == nil || item == nil || allStaking == nil {
		return nil, types.ErrInvalidParam
	}
	var items []*types.RewardInfo
	fee := new(big.Int).Quo(new(big.Int).Mul(allReward, sa.Fee), types.Base)
	all, left,left2 := new(big.Int).Sub(allReward, fee), big.NewInt(0),big.NewInt(0)
	for _, v := range sa.Delegation {
		daAll := v.getAllStaking(target)
		if daAll.Sign() <= 0 {
			continue
		}
		v1 := new(big.Int).Quo(new(big.Int).Mul(all, daAll), allStaking)
		left = left.Add(left, v1)
		left2 = left2.Add(left2,daAll)
		var ii types.RewardInfo
		ii.Address, ii.Amount,ii.Staking = v.Unit.GetRewardAddress(), new(big.Int).Set(v1),new(big.Int).Set(daAll)
		items = append(items, &ii)
	}
	item.Amount = new(big.Int).Add(new(big.Int).Sub(all, left), fee)
	item.Staking = new(big.Int).Sub(allStaking,left2)
	return items, nil
}
func (i *ImpawnImpl) calcReward(target,effectHeight uint64, allAmount *big.Int, einfo *types.EpochIDInfo) ([]*types.SARewardInfos, error) {
	if _, ok := i.accounts[einfo.EpochID]; !ok {
		return nil, types.ErrInvalidParam
	} else {
		sas := i.getElections3(einfo.EpochID)
		if sas == nil {
			return nil, errors.New(fmt.Sprint(types.ErrMatchEpochID, "epochid:", einfo.EpochID))
		}
		t_eid := types.GetEpochFromHeight(effectHeight)
		if einfo.EpochID >= t_eid.EpochID {
			sas = i.fetchAccountsInEpoch(einfo.EpochID,sas)
			if len(sas) == 0 {
				return nil, errors.New(fmt.Sprint(types.ErrMatchEpochID, "epochid:", einfo.EpochID,"sas=0"))
			}
		}
		impawns := SAImpawns(sas)
		impawns.sort(target, false)
		var res []*types.SARewardInfos
		allValidatorStaking := impawns.getAllStaking(target)
		sum := len(impawns)
		left := big.NewInt(0)

		for pos, v := range impawns {
			var info types.SARewardInfos
			var item types.RewardInfo
			item.Address = v.Unit.GetRewardAddress()
			allStaking := v.getAllStaking(target)
			if allStaking.Sign() <= 0 {
				continue
			}

			v2 := new(big.Int).Quo(new(big.Int).Mul(allStaking, allAmount), allValidatorStaking)
			if pos == sum-1 {
				v2 = new(big.Int).Sub(allAmount, left)
			}
			left = left.Add(left, v2)

			if ii, err := i.calcRewardInSa(target, v, v2, allStaking, &item); err != nil {
				return nil, err
			} else {
				info.Items = append(info.Items, &item)
				info.Items = append(info.Items, ii[:]...)
			}
			res = append(res, &info)
		}
		return res, nil
	}
}
func (i *ImpawnImpl) reward(begin, end,effectHeight uint64, allAmount *big.Int) ([]*types.SARewardInfos, error) {
	ids := types.GetEpochFromRange(begin, end)
	if ids == nil || len(ids) > 2 {
		return nil, errors.New(fmt.Sprint(types.ErrMatchEpochID, "more than 2 epochid:", begin, end))
	}

	if len(ids) == 2 {
		tmp := new(big.Int).Quo(new(big.Int).Mul(allAmount, new(big.Int).SetUint64(ids[0].EndHeight-begin+1)), new(big.Int).SetUint64(end-begin+1))
		amount1, amount2 := tmp, new(big.Int).Sub(allAmount, tmp)
		if items, err := i.calcReward(ids[0].EndHeight,effectHeight, amount1, ids[0]); err != nil {
			return nil, err
		} else {
			t_eid := types.GetEpochFromHeight(effectHeight)
			if ids[1].EpochID >= t_eid.EpochID {
				if items1, err2 := i.calcReward(end,effectHeight, amount2, ids[1]); err2 != nil {
					return nil, err2
				} else {
					items = append(items, items1[:]...)
				}
			} else {
				if items1, err2 := i.calcReward(ids[1].EndHeight,effectHeight, amount2, ids[1]); err2 != nil {
					return nil, err2
				} else {
					items = append(items, items1[:]...)
				}
			}
			return items, nil
		}
	} else {
		return i.calcReward(end,effectHeight, allAmount, ids[0])
	}
}

///////////auxiliary function ////////////////////////////////////////////

/////////////////////////////////////////////////////////////////////////////////
// move the accounts from prev to next epoch and keeps the prev account still here
func (i *ImpawnImpl) move(prev, next,effectHeight uint64) error {
	nextEpoch := types.GetEpochFromID(next)
	if nextEpoch == nil {
		return types.ErrOverEpochID
	}
	prevInfos, ok := i.accounts[prev]
	nextInfos, ok2 := i.accounts[next]
	if !ok {
		return errors.New(fmt.Sprintln("the epoch is nil", prev, "err:", types.ErrNotMatchEpochInfo))
	}
	if !ok2 {
		nextInfos = SAImpawns{}
	}
	for _, v := range prevInfos {
		vv := v.clone()
		vv.merge(prev, nextEpoch.BeginHeight)
		if vv.isvalid() {
			vv.Committee = false
			nextInfos.update(vv, nextEpoch.BeginHeight, true, true,effectHeight)
		}
	}
	i.accounts[next] = nextInfos
	return nil
}

/////////////////////////////////////////////////////////////////////////////////
////////////// external function //////////////////////////////////////////

// DoElections called by consensus while it closer the end of epoch,have 500~1000 fast block
func (i *ImpawnImpl) DoElections(epochid, height uint64) ([]*StakingAccount, error) {
	if epochid < params.FirstNewEpochID && epochid != i.getCurrentEpoch()+1 {
		return nil, types.ErrOverEpochID
	}
	cur := types.GetEpochFromID(i.curEpochID)
	if cur.EndHeight != height+params.ElectionPoint && i.curEpochID >= params.FirstNewEpochID {
		return nil, types.ErrNotElectionTime
	}
	// e := types.GetEpochFromID(epochid)
	eid := epochid
	if eid >= params.FirstNewEpochID {
		eid = eid - 1
	}
	if val, ok := i.accounts[eid]; ok {
		val.sort(height, true)
		var ee []*StakingAccount
		for _, v := range val {
			validStaking := v.getValidStakingOnly(height)
			if validStaking.Cmp(params.ElectionMinLimitForStaking) < 0 {
				continue
			}
			v.Committee = true
			ee = append(ee, v)
			if len(ee) >= params.CountInEpoch {
				break
			}
		}
		return ee, nil
	} else {
		return nil, types.ErrMatchEpochID
	}
}

// Shift will move the staking account which has election flag to the next epoch
// it will be save the whole state in the current epoch end block after it called by consensus
func (i *ImpawnImpl) Shift(epochid,effectHeight uint64) error {
	lastReward := i.lastReward
	minEpoch := types.GetEpochFromHeight(lastReward)
	min := i.getMinEpochID()
	// fmt.Println("*** move min:", min, "minEpoch:", minEpoch.EpochID, "lastReward:", i.lastReward)
	for ii := min; minEpoch.EpochID > 1 && ii < minEpoch.EpochID-1; ii++ {
		delete(i.accounts, ii)
		// fmt.Println("delete epoch:", ii)
	}

	if epochid != i.getCurrentEpoch()+1 {
		return types.ErrOverEpochID
	}
	i.SetCurrentEpoch(epochid)
	prev := epochid - 1
	return i.move(prev, epochid,effectHeight)
}

// CancelSAccount cancel amount of asset for staking account,it will be work in next epoch
func (i *ImpawnImpl) CancelSAccount(curHeight uint64, addr common.Address, amount *big.Int) error {
	if amount.Sign() <= 0 || curHeight <= 0 {
		return types.ErrInvalidParam
	}
	curEpoch := types.GetEpochFromHeight(curHeight)
	if curEpoch == nil || curEpoch.EpochID != i.curEpochID {
		return types.ErrInvalidParam
	}
	sa, err := i.GetStakingAccount(curEpoch.EpochID, addr)
	if err != nil {
		return err
	}
	err2 := sa.stopStakingInfo(amount, new(big.Int).SetUint64(curHeight))
	// fmt.Println("[SA]insert a redeem,address:[", addr.String(), "],amount:[", amount.String(), "],height:", curHeight, "]err:", err2)
	return err2
}

// CancelDAccount cancel amount of asset for delegation account,it will be work in next epoch
func (i *ImpawnImpl) CancelDAccount(curHeight uint64, addrSA, addrDA common.Address, amount *big.Int) error {
	if amount.Sign() <= 0 || curHeight <= 0 {
		return types.ErrInvalidParam
	}
	curEpoch := types.GetEpochFromHeight(curHeight)
	if curEpoch == nil || curEpoch.EpochID != i.curEpochID {
		return types.ErrInvalidParam
	}
	sa, err := i.GetStakingAccount(curEpoch.EpochID, addrSA)
	if err != nil {
		return err
	}
	da, err2 := i.getDAfromSA(sa, addrDA)
	if err2 != nil {
		return err
	}
	err3 := da.stopStakingInfo(amount, new(big.Int).SetUint64(curHeight))
	// fmt.Println("[DA]insert a redeem,address:[", addrSA.String(), "],DA address:[", addrDA.String(), "],amount:[", amount.String(), "],height:", curHeight, "]err:", err3)
	return err3
}

// RedeemSAccount redeem amount of asset for staking account,it will locked for a certain time
func (i *ImpawnImpl) RedeemSAccount(curHeight uint64, addr common.Address, amount *big.Int) error {
	if amount.Sign() <= 0 || curHeight <= 0 {
		return types.ErrInvalidParam
	}
	curEpoch := types.GetEpochFromHeight(curHeight)
	if curEpoch == nil || curEpoch.EpochID != i.curEpochID {
		return types.ErrInvalidParam
	}
	sa, err := i.GetStakingAccount(curEpoch.EpochID, addr)
	if err != nil {
		return err
	}
	return i.redeemBySa(sa, curHeight, amount)
}

// RedeemDAccount redeem amount of asset for delegation account,it will locked for a certain time
func (i *ImpawnImpl) RedeemDAccount(curHeight uint64, addrSA, addrDA common.Address, amount *big.Int) error {
	if amount.Sign() <= 0 || curHeight <= 0 {
		return types.ErrInvalidParam
	}
	curEpoch := types.GetEpochFromHeight(curHeight)
	if curEpoch == nil || curEpoch.EpochID != i.curEpochID {
		return types.ErrInvalidParam
	}
	sa, err := i.GetStakingAccount(curEpoch.EpochID, addrSA)
	if err != nil {
		return err
	}
	da, err2 := i.getDAfromSA(sa, addrDA)
	if err2 != nil {
		return err
	}
	return i.redeemByDa(da, curHeight, amount)
}

func (i *ImpawnImpl) insertDAccount(height uint64, da *DelegationAccount) error {
	if da == nil {
		return types.ErrInvalidParam
	}
	epochInfo := types.GetEpochFromHeight(height)
	if epochInfo == nil || epochInfo.EpochID > i.getCurrentEpoch() {
		return types.ErrOverEpochID
	}
	sa, err := i.GetStakingAccount(epochInfo.EpochID, da.SaAddress)
	if err != nil {
		return err
	}
	if ds, err := i.getDAfromSA(sa, da.Unit.Address); err != nil {
		return err
	} else {
		if ds == nil {
			sa.Delegation = append(sa.Delegation, da)
			log.Debug("Insert delegation account", "staking account", sa.Unit.GetRewardAddress(), "account", da.Unit.GetRewardAddress())
		} else {
			ds.update(da, false)
			log.Debug("Update delegation account", "staking account", sa.Unit.GetRewardAddress(), "account", da.Unit.GetRewardAddress())
		}
	}
	return nil
}
func (i *ImpawnImpl) InsertDAccount2(height uint64, addrSA, addrDA common.Address, val *big.Int) error {
	if val.Sign() <= 0 || height < 0 {
		return types.ErrInvalidParam
	}
	if bytes.Equal(addrSA.Bytes(), addrDA.Bytes()) {
		return types.ErrDelegationSelf
	}
	state := uint8(0)
	state |= types.StateStakingAuto
	da := &DelegationAccount{
		SaAddress: addrSA,
		Unit: &impawnUnit{
			Address: addrDA,
			Value: []*PairstakingValue{&PairstakingValue{
				Amount: new(big.Int).Set(val),
				Height: new(big.Int).SetUint64(height),
				State:  state,
			}},
			RedeemInof: make([]*RedeemItem, 0),
		},
	}
	return i.insertDAccount(height, da)
}
func (i *ImpawnImpl) insertSAccount(height uint64, sa *StakingAccount) error {
	if sa == nil {
		return types.ErrInvalidParam
	}
	epochInfo := types.GetEpochFromHeight(height)
	if epochInfo == nil || epochInfo.EpochID > i.getCurrentEpoch() {
		log.Error("insertSAccount", "eid", epochInfo.EpochID, "height", height, "eid2", i.getCurrentEpoch())
		return types.ErrOverEpochID
	}
	if val, ok := i.accounts[epochInfo.EpochID]; !ok {
		var accounts []*StakingAccount
		accounts = append(accounts, sa)
		i.accounts[epochInfo.EpochID] = SAImpawns(accounts)
		log.Debug("Insert staking account", "epoch", epochInfo, "account", sa.Unit.GetRewardAddress())
	} else {
		for _, ii := range val {
			if bytes.Equal(ii.Unit.Address.Bytes(), sa.Unit.Address.Bytes()) {
				ii.update(sa, height, false, false)
				log.Debug("Update staking account", "account", sa.Unit.GetRewardAddress())
				return nil
			}
		}
		i.accounts[epochInfo.EpochID] = append(val, sa)
		log.Debug("Insert staking account", "epoch", epochInfo, "account", sa.Unit.GetRewardAddress())
	}
	return nil
}
func (i *ImpawnImpl) InsertSAccount2(height uint64, addr common.Address, pk []byte, val *big.Int, fee *big.Int, auto bool) error {
	if val.Sign() <= 0 || height < 0 || fee.Sign() < 0 || fee.Cmp(types.Base) > 0 {
		return types.ErrInvalidParam
	}
	if err := types.ValidPk(pk); err != nil {
		return err
	}
	if i.repeatPK(addr,pk) {
		log.Error("Insert SA account repeat pk", "addr", addr, "pk", pk,)
		return types.ErrRepeatPk
	}
	state := uint8(0)
	if auto {
		state |= types.StateStakingAuto
	}
	sa := &StakingAccount{
		Votepubkey: append([]byte{}, pk...),
		Fee:        new(big.Int).Set(fee),
		Unit: &impawnUnit{
			Address: addr,
			Value: []*PairstakingValue{&PairstakingValue{
				Amount: new(big.Int).Set(val),
				Height: new(big.Int).SetUint64(height),
				State:  state,
			}},
			RedeemInof: make([]*RedeemItem, 0),
		},
		Modify: &AlterableInfo{},
	}
	return i.insertSAccount(height, sa)
}
func (i *ImpawnImpl) AppendSAAmount(height uint64, addr common.Address, val *big.Int) error {
	if val.Sign() <= 0 || height < 0 {
		return types.ErrInvalidParam
	}
	epochInfo := types.GetEpochFromHeight(height)
	if epochInfo.EpochID > i.getCurrentEpoch() {
		log.Debug("insertSAccount", "eid", epochInfo.EpochID, "height", height, "eid2", i.getCurrentEpoch())
		return types.ErrOverEpochID
	}
	sa, err := i.GetStakingAccount(epochInfo.EpochID, addr)
	if err != nil {
		return err
	}
	sa.addAmount(height, val)
	return nil
}
func (i *ImpawnImpl) UpdateSAFee(height uint64, addr common.Address, fee *big.Int) error {
	if height < 0 || fee.Sign() < 0 || fee.Cmp(types.Base) > 0 {
		return types.ErrInvalidParam
	}
	epochInfo := types.GetEpochFromHeight(height)
	if epochInfo.EpochID > i.getCurrentEpoch() {
		log.Info("UpdateSAFee", "eid", epochInfo.EpochID, "height", height, "eid2", i.getCurrentEpoch())
		return types.ErrOverEpochID
	}
	sa, err := i.GetStakingAccount(epochInfo.EpochID, addr)
	if err != nil {
		return err
	}
	sa.updateFee(height, fee)
	return nil
}
func (i *ImpawnImpl) UpdateSAPK(height uint64, addr common.Address, pk []byte) error {
	if height < 0 {
		return types.ErrInvalidParam
	}
	if err := types.ValidPk(pk); err != nil {
		return err
	}
	epochInfo := types.GetEpochFromHeight(height)
	if epochInfo.EpochID > i.getCurrentEpoch() {
		log.Info("UpdateSAPK", "eid", epochInfo.EpochID, "height", height, "eid2", i.getCurrentEpoch())
		return types.ErrOverEpochID
	}
	sa, err := i.GetStakingAccount(epochInfo.EpochID, addr)
	if err != nil {
		return err
	}
	sa.updatePk(height, pk)
	return nil
}
func (i *ImpawnImpl) Reward(block *types.SnailBlock, allAmount *big.Int,effectHeight uint64) ([]*types.SARewardInfos, error) {
	begin, end := types.FromBlock(block)
	res, err := i.reward(begin, end,effectHeight, allAmount)
	if err == nil {
		i.lastReward = end
	}
	return res, err
}
func (i *ImpawnImpl) Reward2(begin, end,effectHeight uint64, allAmount *big.Int) ([]*types.SARewardInfos, error) {

	res, err := i.reward(begin, end,effectHeight, allAmount)
	if err == nil {
		i.lastReward = end
	}
	return res, err
}

/////////////////////////////////////////////////////////////////////////////////
// GetStakings return all staking accounts of the current epoch
func (i *ImpawnImpl) GetAllStakingAccount() SAImpawns {
	if val, ok := i.accounts[i.curEpochID]; ok {
		return val
	} else {
		return nil
	}
}

// GetStakingAsset returns a map for all staking amount of the address, the key is the SA address
func (i *ImpawnImpl) GetStakingAsset(addr common.Address) map[common.Address]*types.StakingValue {
	epochid := i.curEpochID
	res, _ := i.getAsset(addr, epochid, types.OpQueryStaking)
	return res
}

// GetLockedAsset returns a group canceled asset from the state of the addr,it includes redemption on
// maturity and unmaturity asset
func (i *ImpawnImpl) GetLockedAsset(addr common.Address) map[common.Address]*types.StakingValue {
	epochid := i.curEpochID
	res, _ := i.getAsset(addr, epochid, types.OpQueryLocked)
	return res
}
func (i *ImpawnImpl) GetLockedAsset2(addr common.Address, height uint64) map[common.Address]*types.LockedValue {
	epochid := i.curEpochID
	items, _ := i.getAsset(addr, epochid, types.OpQueryLocked)
	res := make(map[common.Address]*types.LockedValue)
	for k, v := range items {
		res[k] = v.ToLockedValue(height)
	}
	return res
}

// GetAllCancelableAsset returns all asset on addr it can be canceled
func (i *ImpawnImpl) GetAllCancelableAsset(addr common.Address) map[common.Address]*big.Int {
	epochid := i.curEpochID
	_, res := i.getAsset(addr, epochid, types.OpQueryCancelable)
	return res
}
func (i *ImpawnImpl) getAsset(addr common.Address, epoch uint64, op uint8) (map[common.Address]*types.StakingValue, map[common.Address]*big.Int) {
	epochid := epoch
	end := types.GetEpochFromID(epochid).EndHeight
	if val, ok := i.accounts[epochid]; ok {
		res := make(map[common.Address]*types.StakingValue)
		res2 := make(map[common.Address]*big.Int)
		for _, v := range val {
			if bytes.Equal(v.Unit.Address.Bytes(), addr.Bytes()) {
				if op&types.OpQueryStaking != 0 || op&types.OpQueryLocked != 0 {
					if _, ok := res[addr]; !ok {
						if op&types.OpQueryLocked != 0 {
							res[addr] = &types.StakingValue{
								Value: v.Unit.redeemToMap(),
							}
						} else {
							res[addr] = &types.StakingValue{
								Value: v.Unit.valueToMap(),
							}
						}

					} else {
						log.Error("getAsset", "repeat staking account", addr, "epochid", epochid, "op", op)
					}
				}
				if op&types.OpQueryCancelable != 0 {
					all := v.Unit.getValidStaking(end)
					if all.Sign() >= 0 {
						res2[addr] = all
					}
				}

				continue
			} else {
				for _, vv := range v.Delegation {
					if bytes.Equal(vv.Unit.Address.Bytes(), addr.Bytes()) {
						if op&types.OpQueryStaking != 0 || op&types.OpQueryLocked != 0 {
							if _, ok := res[v.Unit.Address]; !ok {
								if op&types.OpQueryLocked != 0 {
									res[v.Unit.Address] = &types.StakingValue{
										Value: vv.Unit.redeemToMap(),
									}
								} else {
									res[v.Unit.Address] = &types.StakingValue{
										Value: vv.Unit.valueToMap(),
									}
								}
							} else {
								log.Error("getAsset", "repeat delegation account[sa,da]", v.Unit.Address, addr, "epochid", epochid, "op", op)
							}
						}

						if op&types.OpQueryCancelable != 0 {
							all := vv.Unit.getValidStaking(end)
							if all.Sign() >= 0 {
								res2[v.Unit.Address] = all
							}
						}
						break
					}
				}
			}
		}
		return res, res2
	} else {
		log.Error("getAsset", "wrong epoch in current", epochid)
	}
	return nil, nil
}

/////////////////////////////////////////////////////////////////////////////////
// storage layer
func (i *ImpawnImpl) GetRoot() common.Hash {
	return common.Hash{}
}
func (i *ImpawnImpl) Save(state StateDB, preAddress common.Address) error {
	key := common.BytesToHash(preAddress[:])
	data, err := rlp.EncodeToBytes(i)

	if err != nil {
		log.Crit("Failed to RLP encode ImpawnImpl", "err", err)
	}
	hash := types.RlpHash(data)
	state.SetPOSState(preAddress, key, data)
	tmp := CloneImpawnImpl(i)
	if tmp != nil {
		IC.Cache.Add(hash, tmp)
	}
	return err
}
func (i *ImpawnImpl) Load(state StateDB, preAddress common.Address) error {
	key := common.BytesToHash(preAddress[:])
	data := state.GetPOSState(preAddress, key)
	lenght := len(data)
	if lenght == 0 {
		return errors.New("Load data = 0")
	}
	// cache := true
	hash := types.RlpHash(data)
	var temp ImpawnImpl
	if cc, ok := IC.Cache.Get(hash); ok {
		impawn := cc.(*ImpawnImpl)
		temp = *(CloneImpawnImpl(impawn))
	} else {
		if err := rlp.DecodeBytes(data, &temp); err != nil {
			log.Error("Invalid ImpawnImpl entry RLP", "err", err)
			return errors.New(fmt.Sprintf("Invalid ImpawnImpl entry RLP %s", err.Error()))
		}	
		tmp := CloneImpawnImpl(&temp)	
		if tmp != nil {
			IC.Cache.Add(hash, tmp)
		}
		// cache = false
	}
	// log.Info("-----Load impawn---","len:",lenght,"count:",temp.Counts(),"cache",cache)
	i.curEpochID, i.accounts, i.lastReward = temp.curEpochID, temp.accounts, temp.lastReward
	return nil
}

func GetCurrentValidators(state StateDB) []*types.CommitteeMember {
	i := NewImpawnImpl()
	i.Load(state, types.StakingAddress)
	eid := i.getCurrentEpoch()
	accs := i.getElections3(eid)
	var vv []*types.CommitteeMember
	for _, v := range accs {
		pubkey, _ := crypto.UnmarshalPubkey(v.Votepubkey)
		vv = append(vv, &types.CommitteeMember{
			CommitteeBase: crypto.PubkeyToAddress(*pubkey),
			Coinbase:      v.Unit.GetRewardAddress(),
			Publickey:     types.CopyVotePk(v.Votepubkey),
			Flag:          types.StateUsedFlag,
			MType:         types.TypeWorked,
		})
	}
	return vv
}

func GetValidatorsByEpoch(state StateDB, eid, hh uint64) []*types.CommitteeMember {
	i := NewImpawnImpl()
	err := i.Load(state, types.StakingAddress)
	accs := i.getElections3(eid)
	first := types.GetFirstEpoch()
	if hh == first.EndHeight-params.ElectionPoint {
		fmt.Println("****** accounts len:", len(i.accounts), "election:", len(accs), " err ", err)
	}
	var vv []*types.CommitteeMember
	for _, v := range accs {
		pubkey, _ := crypto.UnmarshalPubkey(v.Votepubkey)
		vv = append(vv, &types.CommitteeMember{
			CommitteeBase: crypto.PubkeyToAddress(*pubkey),
			Coinbase:      v.Unit.GetRewardAddress(),
			Publickey:     types.CopyVotePk(v.Votepubkey),
			Flag:          types.StateUsedFlag,
			MType:         types.TypeWorked,
		})
	}
	return vv
}
func (i *ImpawnImpl) Counts() int {
	pos := 0
	for _,val := range i.accounts {
		for _, vv := range val {
			pos = pos + len(vv.Delegation)
		}
		pos = pos + len(val)
	}
	return pos
}
func (i *ImpawnImpl) Summay() *types.ImpawnSummay {
	summay := &types.ImpawnSummay{
		LastReward: i.lastReward,
		Infos:		make([]*types.SummayEpochInfo,0,0),
	}
	sumAccount := 0
	for k,val := range i.accounts {
		info := types.GetEpochFromID(k)
		item := &types.SummayEpochInfo{
			EpochID:		info.EpochID,
			BeginHeight:	info.BeginHeight,
			EndHeight:		info.EndHeight,
		}
		item.AllAmount = val.getValidStaking(info.EndHeight)
		daSum,saSum := 0, len(val)
		for _, vv := range val {
			daSum = daSum + len(vv.Delegation)
		}
		item.DaCount,item.SaCount = uint64(daSum),uint64(saSum)
		summay.Infos = append(summay.Infos,item)
		sumAccount = sumAccount + daSum + saSum
		if i.curEpochID == k {
			summay.AllAmount = new(big.Int).Set(item.AllAmount)
		}
	}
	summay.Accounts = uint64(sumAccount)
	return summay
}
/////////////////////////////////////////////////////////////////////////////////
type valuesByHeight []*PairstakingValue

func (vs valuesByHeight) Len() int {
	return len(vs)
}
func (vs valuesByHeight) Less(i, j int) bool {
	return vs[i].Height.Cmp(vs[j].Height) == -1
}
func (vs valuesByHeight) Swap(i, j int) {
	it := vs[i]
	vs[i] = vs[j]
	vs[j] = it
}
func (vs valuesByHeight) find(hh uint64) (*PairstakingValue, int) {
	low, height := 0, len(vs)-1
	mid := 0
	for low <= height {
		mid = (height + low) / 2
		if hh == vs[mid].Height.Uint64() {
			return vs[mid], mid
		} else if hh > vs[mid].Height.Uint64() {
			low = mid + 1
			if low > height {
				return nil, low
			}
		} else {
			height = mid - 1
		}
	}
	return nil, mid
}
func (vs valuesByHeight) update(val *PairstakingValue) valuesByHeight {
	item, pos := vs.find(val.Height.Uint64())
	if item != nil {
		item.Amount = item.Amount.Add(item.Amount, val.Amount)
		item.State |= val.State
	} else {
		rear := append([]*PairstakingValue{}, vs[pos:]...)
		vs = append(append(vs[:pos], val), rear...)
	}
	return vs
}

type redeemByID []*RedeemItem

func (vs redeemByID) Len() int {
	return len(vs)
}
func (vs redeemByID) Less(i, j int) bool {
	return vs[i].EpochID < vs[j].EpochID
}
func (vs redeemByID) Swap(i, j int) {
	it := vs[i]
	vs[i] = vs[j]
	vs[j] = it
}

type stakingItem struct {
	item   *StakingAccount
	height uint64
	valid  bool
}

func (s *stakingItem) getAll() *big.Int {
	if s.valid {
		return s.item.getValidStaking(s.height)
	} else {
		return s.item.getAllStaking(s.height)
	}
}

type stakingByAmount []*stakingItem

func toStakingByAmount(hh uint64, valid bool, items []*StakingAccount) stakingByAmount {
	var tmp []*stakingItem
	for _, v := range items {
		v.Unit.sort()
		tmp = append(tmp, &stakingItem{
			item:   v,
			height: hh,
			valid:  valid,
		})
	}
	return stakingByAmount(tmp)
}
func fromStakingByAmount(items stakingByAmount) ([]*StakingAccount, uint64) {
	var tmp []*StakingAccount
	var vv uint64
	for _, v := range items {
		tmp = append(tmp, v.item)
		vv = v.height
	}
	return tmp, vv
}
func (vs stakingByAmount) Len() int {
	return len(vs)
}
func (vs stakingByAmount) Less(i, j int) bool {
	return vs[i].getAll().Cmp(vs[j].getAll()) > 0
}
func (vs stakingByAmount) Swap(i, j int) {
	it := vs[i]
	vs[i] = vs[j]
	vs[j] = it
}

type delegationItem struct {
	item   *DelegationAccount
	height uint64
	valid  bool
}

func (d *delegationItem) getAll() *big.Int {
	if d.valid {
		return d.item.getValidStaking(d.height)
	} else {
		return d.item.getAllStaking(d.height)
	}
}

type delegationItemByAmount []*delegationItem

func toDelegationByAmount(hh uint64, valid bool, items []*DelegationAccount) delegationItemByAmount {
	var tmp []*delegationItem
	for _, v := range items {
		v.Unit.sort()
		tmp = append(tmp, &delegationItem{
			item:   v,
			height: hh,
			valid:  valid,
		})
	}
	return delegationItemByAmount(tmp)
}
func fromDelegationByAmount(items delegationItemByAmount) ([]*DelegationAccount, uint64) {
	var tmp []*DelegationAccount
	var vv uint64
	for _, v := range items {
		tmp = append(tmp, v.item)
		vv = v.height
	}
	return tmp, vv
}
func (vs delegationItemByAmount) Len() int {
	return len(vs)
}
func (vs delegationItemByAmount) Less(i, j int) bool {
	return vs[i].getAll().Cmp(vs[j].getAll()) > 0
}
func (vs delegationItemByAmount) Swap(i, j int) {
	it := vs[i]
	vs[i] = vs[j]
	vs[j] = it
}
