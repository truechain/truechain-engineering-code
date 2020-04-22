package main

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/etruedb"
	"math/big"
	"testing"
	"time"
)

func TestStateTime(t *testing.T) {
	// Create an empty state0 database
	db := etruedb.NewMemDatabase()
	state0, _ := state.New(common.Hash{}, state.NewDatabase(db))
	mAccount := common.HexToAddress("0xC02f50f4F41f46b6a2f08036ae65039b2F9aCd69")
	key, _ := crypto.GenerateKey()
	coinbase := crypto.PubkeyToAddress(key.PublicKey)
	sendNumber := 10000
	delegateAddr := make([]common.Address, sendNumber)
	for i := 0; i < sendNumber; i++ {
		key, _ := crypto.GenerateKey()
		delegateAddr[i] = crypto.PubkeyToAddress(key.PublicKey)
	}
	toAddr := make([]common.Address, sendNumber)
	for i := 0; i < sendNumber; i++ {
		key, _ := crypto.GenerateKey()
		toAddr[i] = crypto.PubkeyToAddress(key.PublicKey)
	}
	i, _ := new(big.Int).SetString("90000000000000000000000", 10)
	state0.AddBalance(mAccount, i)
	state0.SetNonce(mAccount, 1)

	nonce := state0.GetNonce(mAccount)
	for _, v := range delegateAddr {
		state0.SubBalance(mAccount, trueToWei(2))
		state0.AddBalance(v, trueToWei(2))
		nonce = nonce + 1
		state0.SetNonce(mAccount, nonce)
	}
	root := state0.IntermediateRoot(true)
	// Write state0 changes to db
	root, err := state0.Commit(true)
	if err != nil {
		panic(fmt.Sprintf("state0 write error: %v", err))
	}
	if err := state0.Database().TrieDB().Commit(root, false); err != nil {
		panic(fmt.Sprintf("trie write error: %v", err))
	}

	start := time.Now()
	state1, _ := state.New(root, state.NewDatabase(db))
	nonce = state1.GetNonce(mAccount)
	for i := 0; i < sendNumber; i++ {
		state1.SubBalance(mAccount, trueToWei(2))
		state1.AddBalance(coinbase, trueToWei(2))
		nonce = nonce + 1
		state1.SetNonce(mAccount, nonce)
	}
	t1 := time.Now()
	root1 := state1.IntermediateRoot(true)
	// Write state0 changes to db
	t2 := time.Now()
	root1, err = state1.Commit(true)
	if err != nil {
		panic(fmt.Sprintf("state0 write error: %v", err))
	}
	t3 := time.Now()
	if err := state1.Database().TrieDB().Commit(root1, false); err != nil {
		panic(fmt.Sprintf("trie write error: %v", err))
	}
	fmt.Println("balance", weiToTrue(state1.GetBalance(mAccount)), "apply", common.PrettyDuration(t1.Sub(start)),
		"IntermediateRoot", common.PrettyDuration(t2.Sub(t1)),
		"Commit", common.PrettyDuration(t3.Sub(t2)),
		"DBCommit", common.PrettyDuration(time.Since(t3)),
		"all time", common.PrettyDuration(time.Since(start)),
	)

	start = time.Now()
	state1, _ = state.New(root, state.NewDatabase(db))
	nonce = state1.GetNonce(mAccount)
	for k, v := range delegateAddr {
		state1.SubBalance(v, trueToWei(1))
		state1.AddBalance(toAddr[k], trueToWei(1))
		state1.SetNonce(v, state1.GetNonce(v)+1)
	}

	t1 = time.Now()
	root1 = state1.IntermediateRoot(true)
	// Write state0 changes to db
	t2 = time.Now()
	root1, err = state1.Commit(true)
	if err != nil {
		panic(fmt.Sprintf("state0 write error: %v", err))
	}
	t3 = time.Now()
	if err := state1.Database().TrieDB().Commit(root1, false); err != nil {
		panic(fmt.Sprintf("trie write error: %v", err))
	}
	fmt.Println("balance", weiToTrue(state1.GetBalance(mAccount)), "apply", common.PrettyDuration(t1.Sub(start)),
		"IntermediateRoot", common.PrettyDuration(t2.Sub(t1)),
		"Commit", common.PrettyDuration(t3.Sub(t2)),
		"DBCommit", common.PrettyDuration(time.Since(t3)),
		"all time", common.PrettyDuration(time.Since(start)),
	)

	start = time.Now()
	type task struct {
		state *state.StateDB
		from  []common.Address
		to    []common.Address
		index int
	}
	taskdone := make(chan task, 16)
	maxActiveDialTasks := 32
	var runningTasks []task
	var queuedTasks []task // tasks that can't run yet

	stateNum := 20
	txsNumber := sendNumber / stateNum

	// removes t from runningTasks
	delTask := func(t task) {
		for i := range runningTasks {
			if runningTasks[i].index == t.index {
				runningTasks = append(runningTasks[:i], runningTasks[i+1:]...)
				break
			}
		}
	}
	// starts until max number of active tasks is satisfied
	startTasks := func(ts []task) (rest []task) {
		i := 0
		for ; len(runningTasks) < maxActiveDialTasks && i < len(ts); i++ {
			t := ts[i]
			go func() {
				stateC := t.state
				for k, v := range t.from {
					stateC.SubBalance(v, trueToWei(1))
					stateC.AddBalance(t.to[k], trueToWei(1))
					stateC.SetNonce(v, stateC.GetNonce(v)+1)
				}
				taskdone <- t
			}()
			runningTasks = append(runningTasks, t)
		}
		return ts[i:]
	}

	scheduleTasks := func(state *state.StateDB, from []common.Address, to []common.Address, index int) int {
		// Start from queue first.
		queuedTasks = startTasks(queuedTasks)
		// Query dialer for new tasks and start as many as possible now.
		if len(runningTasks) < maxActiveDialTasks {
			var tasks []task // tasks that can't run yet
			for i := 0; i < maxActiveDialTasks; i++ {
				if index+i > stateNum-1 {
					break
				}
				tasks = append(tasks, task{state.Copy(), from[(index+i)*txsNumber : (index+i+1)*txsNumber], to[(index+i)*txsNumber : (index+i+1)*txsNumber], index + i})
			}
			queuedTasks = append(queuedTasks, startTasks(tasks)...)
			return index + len(tasks)
		}
		return index
	}
	var stateArr []*state.StateDB
	state1, _ = state.New(root, state.NewDatabase(db))
	index := 0

running:
	for {
		if index < stateNum && len(queuedTasks) < 2 {
			index = scheduleTasks(state1, delegateAddr, toAddr, index)
		} else if len(queuedTasks) > 0 {
			queuedTasks = startTasks(queuedTasks)
		}

		select {
		case t := <-taskdone:
			stateArr = append(stateArr, t.state)
			delTask(t)
			if len(stateArr) == stateNum {
				break running
			}
		}
	}

	fmt.Println("time ", common.PrettyDuration(time.Since(start)))
}
