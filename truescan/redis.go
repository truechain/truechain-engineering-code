package truescan

import (
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/gomodule/redigo/redis"
)

// RedisClient is used only at specific nodes, which needs to push messages to redis services.
// Now mainly provide service for TrueScan.
type RedisClient struct {
	id         int
	serverAddr string
	c          redis.Conn
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
	return rc, nil
}

func (rc *RedisClient) publish(channel string, message string) error {
	if rc.c == nil {
		return errors.New("Redis client is closed")
	}
	_, err := rc.c.Do("PUBLISH", channel, message)
	return err
}

func (rc *RedisClient) publishMsg(message string) error {
	channel := "truescan:ch:" + strconv.Itoa(rc.id)
	err := rc.publish(channel, message)
	return err
}

// PendingTransaction is triggered when the node receives the transaction
// and is verified by adding it to the transaction pool.
func (rc *RedisClient) PendingTransaction(ptm *TransactionMsg) error {
	msg, err := json.Marshal(ptm)
	if err != nil {
		return err
	}
	start := `{"name":"pendingTransaction","data":`
	end := `}`
	err = rc.publishMsg(start + string(msg) + end)
	return err
}

// RemoveTransaction is triggered when transaction in the trading pool is discarded before execution,
// which may be the result of synchronization or transaction coverage event.
func (rc *RedisClient) RemoveTransaction() error {
	exampleMsg := struct {
		Hash string `json:"hash"`
	}{
		Hash: "0x3ad653e4ac05237b39b1cb42f054e0c167fed354c838e9cae6fe3871f006a6fc",
	}
	msg, err := json.Marshal(exampleMsg)
	if err != nil {
		return err
	}
	start := `{"name":"removeTransaction","data":`
	end := `}`
	err = rc.publishMsg(start + string(msg) + end)
	return err
}

// ExecuteTransaction is triggered when the transaction is executed,
// and the result of the transaction may be success or failure.
// Transaction failure is different from transaction discarded.
func (rc *RedisClient) ExecuteTransaction() error {
	exampleMsg := struct {
		Status           bool   `json:"status"`
		FruitblockNumber int    `json:"fruitblockNumber"`
		TransactionHash  string `json:"transactionHash"`
		TransactionIndex int    `json:"transactionIndex"`
		ContractAddress  string `json:"contractAddress"`
		GasUsed          int    `json:"gasUsed"`
		Timestamp        int    `json:"timestamp"`
	}{
		Status:           true,
		FruitblockNumber: 1234,
		TransactionHash:  "0x3ad653e4ac05237b39b1cb42f054e0c167fed354c838e9cae6fe3871f006a6fc",
		TransactionIndex: 2,
		ContractAddress:  "",
		GasUsed:          21000,
		Timestamp:        1536633528,
	}
	msg, err := json.Marshal(exampleMsg)
	if err != nil {
		return err
	}
	start := `{"name":"executeTransaction","data":`
	end := `}`
	err = rc.publishMsg(start + string(msg) + end)
	return err
}

// NewFastBlockHeader is triggered when a fast block is packaged
// by the committee.
func (rc *RedisClient) NewFastBlockHeader(fbm *FastBlockHeaderMsg) error {
	msg, err := json.Marshal(fbm)
	if err != nil {
		return err
	}
	start := `{"name":"receiveFastBlockHeader","data":`
	end := `}`
	err = rc.publishMsg(start + string(msg) + end)
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
	start := `{"name":"receiveFruitBlockHeader","data":`
	end := `}`
	err = rc.publishMsg(start + string(msg) + end)
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
	start := `{"name":"newBlockHeader","data":`
	end := `}`
	err = rc.publishMsg(start + string(msg) + end)
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
	start := `{"name":"pruningShortBranch","data":`
	end := `}`
	err = rc.publishMsg(start + string(msg) + end)
	return err
}

// ChangeBalance is triggered when any account balance changed due to any reason.
// If a balance change event is rolled back (which in theory should not have happened),
// then there needs to be a "countervailing event" with the opposite amount of change.
func (rc *RedisClient) ChangeBalance() error {
	exampleMsg := struct {
		Address string `json:"address"`
		Change  string `json:"change"`
	}{
		Address: "0xF2bb016e8C9C8975654dcd62f318323A8A79D48E",
		Change:  "100000000000000000000",
	}
	msg, err := json.Marshal(exampleMsg)
	if err != nil {
		return err
	}
	start := `{"name":"changeBalance","data":`
	end := `}`
	err = rc.publishMsg(start + string(msg) + end)
	return err
}

// ChangeView is triggered when the committee changes.
func (rc *RedisClient) changeView() error {
	var exampleMembers = []string{
		"0x281055afc982d96fab65b3a49cac8b878184cb16",
		"0x6f46cf5569aefa1acc1009290c8e043747172d89",
		"0x90e63c3d53e0ea496845b7a03ec7548b70014a91",
		"0x53d284357ec70ce289d6d64134dfac8e511c8a3d",
		"0x742d35cc6634c0532925a3b844bc454e4438f44e",
		"0xfe9e8709d3215310075d67e3ed32a380ccf451c8",
	}
	exampleMsg := struct {
		ViewNumber int      `json:"viewNumber"`
		Members    []string `json:"members"`
		Timestamp  int      `json:"timestamp"`
	}{
		ViewNumber: 5,
		Members:    exampleMembers,
		Timestamp:  1536642158,
	}
	msg, err := json.Marshal(exampleMsg)
	if err != nil {
		return err
	}
	start := `{"name":"changeView","data":`
	end := `}`
	err = rc.publishMsg(start + string(msg) + end)
	return err
}
