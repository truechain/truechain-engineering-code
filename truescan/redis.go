package truescan

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/log"
)

var msgChanSize = 16

// RedisClient is used only at specific nodes, which needs to push messages to redis services.
// Now mainly provide service for TrueScan.
type RedisClient struct {
	id         int
	serverAddr string
	c          redis.Conn
	msgCh      chan string
}

// NewRedisClient returns a redis client with scheduled message sending interface.
func NewRedisClient(redisServerAddr string, id int) (*RedisClient, error) {
	rc := &RedisClient{
		id:         id,
		serverAddr: redisServerAddr,
	}
	const healthCheckPeriod = time.Minute
	c, err := redis.Dial("tcp", redisServerAddr,
		// Read timeout on server should be greater than ping period.
		redis.DialReadTimeout(healthCheckPeriod+10*time.Second),
		redis.DialWriteTimeout(10*time.Second))
	if err != nil {
		return nil, err
	}
	rc.c = c
	rc.msgCh = make(chan string, msgChanSize)
	return rc, nil
}

// Start redis server
func (rc *RedisClient) Start() {
	go rc.msgSendLoop()
}

func (rc *RedisClient) publish(channel string, message string) error {
	if rc.c == nil {
		return errors.New("Redis client is closed")
	}
	_, err := rc.c.Do("PUBLISH", channel, message)
	return err
}

func (rc *RedisClient) msgSendLoop() {
	channel := "truescan:ch:" + strconv.Itoa(rc.id)
	for {
		select {
		case msg, ok := <-rc.msgCh:
			if !ok {
				return
			}
			rc.publish(channel, msg)
		}
	}
}

func (rc *RedisClient) publishMsg(message string) error {
	rc.msgCh <- message
	return nil
}

// Ping sends a message without any payload.
func (rc *RedisClient) Ping() error {
	message := `{"type":"ping","data":"Hello TrueChain!"}`
	err := rc.publishMsg(message)
	log.RedisLog("emit Ping")
	return err
}

// PendingTransaction is triggered when the node receives the transaction
// and is verified by adding it to the transaction pool.
func (rc *RedisClient) PendingTransaction(ptm *TransactionMsg) error {
	msg, err := json.Marshal(ptm)
	if err != nil {
		return err
	}
	start := `{"type":"pendingTransaction","data":`
	end := `}`
	err = rc.publishMsg(start + string(msg) + end)
	log.RedisLog("emit PendingTransaction")
	return err
}

// RemoveTransaction is triggered when transaction in the trading pool is discarded before execution,
// which may be the result of synchronization or transaction coverage event.
func (rc *RedisClient) RemoveTransaction(rtm *RemoveTxMsg) error {
	msg, err := json.Marshal(rtm)
	if err != nil {
		return err
	}
	start := `{"type":"removeTransaction","data":`
	end := `}`
	err = rc.publishMsg(start + string(msg) + end)
	log.RedisLog("emit RemoveTransaction")
	return err
}

// TransactionReceipts is triggered when the transaction is executed,
// and the result of the transaction may be success or failure.
// Transaction failure is different from transaction discarded.
func (rc *RedisClient) TransactionReceipts(receipts types.Receipts) error {
	msg, err := json.Marshal(receipts)
	if err != nil {
		return err
	}
	start := `{"type":"transactionReceipts","data":`
	end := `}`
	err = rc.publishMsg(start + string(msg) + end)
	log.RedisLog("emit TransactionReceipts")
	return err
}

// NewFastBlockHeader is triggered when a fast block is packaged
// by the committee.
func (rc *RedisClient) NewFastBlockHeader(fbm *FastBlockHeaderMsg) error {
	msg, err := json.Marshal(fbm)
	if err != nil {
		return err
	}
	start := `{"type":"newFastBlockHeader","data":`
	end := `}`
	err = rc.publishMsg(start + string(msg) + end)
	log.RedisLog("emit NewFastBlockHeader")
	return err
}

// ReceiveFruitBlockHeader is triggered when a fruit block is mined,
// and there may be multiple fruit blocks at the same height,
// but they may not be packed finally.
func (rc *RedisClient) ReceiveFruitBlockHeader() error {
	exampleMsg := struct {
		FruitblockNumber int    `json:"fruitblockNumber"`
		FruitblockHash   string `json:"fruitblockHash"`
		Nonce            string `json:"nonce"`
		Difficulty       string `json:"difficulty"`
		Miner            string `json:"miner"`
		Timestamp        int    `json:"timestamp"`
	}{
		FruitblockNumber: 1234,
		FruitblockHash:   "0x0ffc66ce61855fc032175fbb9fd82e8ee973f6f0ad5ee26d6fd0715c4de40e3a",
		Nonce:            "0x75df61900a55e511",
		Difficulty:       "2117963098883076",
		Miner:            "0x7E5F4552091A69125d5DfCb7b8C2659029395Bdf",
		Timestamp:        1536633528,
	}
	msg, err := json.Marshal(exampleMsg)
	if err != nil {
		return err
	}
	start := `{"type":"receiveFruitBlockHeader","data":`
	end := `}`
	err = rc.publishMsg(start + string(msg) + end)
	log.RedisLog("emit ReceiveFruitBlockHeader")
	return err
}

// NewSnailBlockHeader is triggered when new blocks are mined.
// The blocks and the fruit blocks packaged in it may roll back,
// but this won't affect the outcome of the transaction.
func (rc *RedisClient) NewSnailBlockHeader(sbm *SnailBlockHeaderMsg) error {
	msg, err := json.Marshal(sbm)
	if err != nil {
		return err
	}
	start := `{"type":"newSnailBlockHeader","data":`
	end := `}`
	err = rc.publishMsg(start + string(msg) + end)
	log.RedisLog("emit NewSnailBlockHeader")
	return err
}

// PruningShortBranch is triggered when the soft fork occurs,
// and the rolled blocks and fruit blocks need to be discarded.
func (rc *RedisClient) PruningShortBranch() error {
	var exampleFruits = []string{
		"0x039bf6c9055a879ef9c4dca7dc52d1d153949c58592dc4d8d21f65681c67af4d",
		"0xb111473b8918b31908b73a1d184ba251a1573bcfb23f07d32550827e4dd8a640",
		"0xdcf771c58a2813f3ff95ee9e1155fbd478334f609810b63eff0ef80e491efa90",
	}
	exampleMsg := struct {
		Number int      `json:"number"`
		Hash   string   `json:"hash"`
		Fruits []string `json:"fruits"`
	}{
		Number: 67,
		Hash:   "0x0e85909ca8633754642b6790cd522e798af31b7258a6420faac041d4ca9886b6",
		Fruits: exampleFruits,
	}
	msg, err := json.Marshal(exampleMsg)
	if err != nil {
		return err
	}
	start := `{"type":"pruningShortBranch","data":`
	end := `}`
	err = rc.publishMsg(start + string(msg) + end)
	log.RedisLog("emit PruningShortBranch")
	return err
}

// StateChange is triggered when accounts balances changed due to any reason.
// If a balance change event is rolled back (which in theory should not have happened),
// then there needs to be a "countervailing event" with the opposite amount of change.
func (rc *RedisClient) StateChange(scm *StateChangeMsg) error {
	msg, err := json.Marshal(scm)
	if err != nil {
		fmt.Println(err)
		return err
	}
	start := `{"type":"stateChange","data":`
	end := `}`
	err = rc.publishMsg(start + string(msg) + end)
	log.RedisLog("emit StateChange")
	return err
}

// ChangeView is triggered when the committee changes.
func (rc *RedisClient) ChangeView(cvm *ChangeViewMsg) error {
	msg, err := json.Marshal(cvm)
	if err != nil {
		return err
	}
	start := `{"type":"changeView","data":`
	end := `}`
	err = rc.publishMsg(start + string(msg) + end)
	log.RedisLog("emit ChangeView")
	return err
}
