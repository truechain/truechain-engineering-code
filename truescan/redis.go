package truescan

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/gomodule/redigo/redis"
)

var msgChanSize = 16
var writeChanSize = 16

// RedisClient is used only at specific nodes, which needs to push messages to redis services.
// Now mainly provide service for TrueScan.
type RedisClient struct {
	id         int
	serverAddr string
	c          redis.Conn
	msgCh      chan string
	writeCh    chan string
	basicData  *basicData
}

type basicData struct {
	FastHeight  uint64 `json:"fastHeight"`
	FruitHeight uint64 `json:"fruitHeight"`
	SnailHeight uint64 `json:"snailHeight"`
	TxsCount    uint64 `json:"txsCount"`
	ViewNumber  uint64 `json:"viewNumber"`
}

// NewRedisClient returns a redis client with scheduled message sending interface.
func NewRedisClient(config *Config) (*RedisClient, error) {
	redisServerAddr := config.RedisHost + ":" + strconv.Itoa(config.RedisPort)
	rc := &RedisClient{
		id:         config.ChannelID,
		serverAddr: redisServerAddr,
	}
	const healthCheckPeriod = time.Minute
	c, err := redis.Dial("tcp", redisServerAddr,
		// Read timeout on server should be greater than ping period.
		redis.DialReadTimeout(healthCheckPeriod+10*time.Second),
		redis.DialWriteTimeout(10*time.Second),
		redis.DialPassword(config.Password))
	if err != nil {
		return nil, err
	}
	v, _ := redis.String(c.Do("GET", "basicData"+":"+strconv.Itoa(rc.id)))
	basicData := &basicData{
		FastHeight:  0,
		FruitHeight: 0,
		SnailHeight: 0,
		TxsCount:    0,
		ViewNumber:  0,
	}
	json.Unmarshal([]byte(v), basicData)
	rc.basicData = basicData
	log.RedisLog("BasicData", "String", v)
	log.RedisLog("BasicData", "FastHeight", basicData.FastHeight, "TxsCount", basicData.TxsCount)
	log.RedisLog("Redis client start", "touch", redisServerAddr, "channel", config.ChannelID)
	rc.c = c
	rc.msgCh = make(chan string, msgChanSize)
	rc.writeCh = make(chan string, writeChanSize)
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

func (rc *RedisClient) updateBasicData() error {
	data, err := json.Marshal(rc.basicData)
	if err != nil {
		return err
	}
	_, err = rc.c.Do("SET", "basicData"+":"+strconv.Itoa(rc.id), data)
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
		case msg, ok := <-rc.writeCh:
			if !ok {
				return
			}
			if msg == "update" {
				rc.updateBasicData()
			}
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
	rc.writeCh <- "update"
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
	rc.writeCh <- "update"
	err = rc.publishMsg(start + string(msg) + end)
	log.RedisLog("emit RemoveTransaction")
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
	needUpdate := false
	if len(fbm.Txs) > 0 {
		rc.basicData.TxsCount += uint64(len(fbm.Txs))
		needUpdate = true
	}
	if fbm.Number > rc.basicData.FastHeight {
		rc.basicData.FastHeight = fbm.Number
		needUpdate = true
	}
	if needUpdate {
		rc.writeCh <- "update"
	}
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
	if sbm.Number > rc.basicData.SnailHeight {
		rc.basicData.SnailHeight = sbm.Number
		rc.writeCh <- "update"
	}
	if sbm.EndFruitNumber > rc.basicData.FruitHeight {
		rc.basicData.FruitHeight = sbm.EndFruitNumber
		rc.writeCh <- "update"
	}
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

// Rewards is triggered when rewards associated with snail block are calculated and distributed,
// while snail block should have been confirmed by 12 blocks.
func (rc *RedisClient) Rewards(rm *RewardsMsg) error {
	msg, err := json.Marshal(rm)
	if err != nil {
		fmt.Println(err)
		return err
	}
	start := `{"type":"rewards","data":`
	end := `}`
	err = rc.publishMsg(start + string(msg) + end)
	log.RedisLog("emit Rewards")
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
	if cvm.EndFastNumber == 0 && cvm.ViewNumber > rc.basicData.ViewNumber {
		rc.basicData.ViewNumber = cvm.ViewNumber
		rc.writeCh <- "update"
	}
	err = rc.publishMsg(start + string(msg) + end)
	log.RedisLog("emit ChangeView")
	return err
}
