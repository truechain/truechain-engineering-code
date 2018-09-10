package core

import (
	"encoding/json"
	"errors"
	"fmt"
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
func (rc *RedisClient) PendingTransaction() error {
	exampleMsg := struct {
		Hash     string `json:"hash"`
		From     string `json:"from"`
		To       string `json:"to"`
		Value    string `json:"value"`
		Gas      int    `json:"gas"`
		GasPrice string `json:"gasPrice"`
		Input    string `json:"input"`
	}{
		Hash:     "0x3ad653e4ac05237b39b1cb42f054e0c167fed354c838e9cae6fe3871f006a6fc",
		From:     "0x7E5F4552091A69125d5DfCb7b8C2659029395Bdf",
		To:       "0xF2bb016e8C9C8975654dcd62f318323A8A79D48E",
		Value:    "100000000000000000000",
		Gas:      21000,
		GasPrice: "5000000000",
		Input:    "",
	}
	msg, err := json.Marshal(exampleMsg)
	if err != nil {
		return err
	}
	fmt.Println(exampleMsg)
	start := `{"name":"pendingTransaction","data":`
	end := `}`
	err = rc.publishMsg(start + string(msg) + end)
	return err
}

// RemoveTransaction is triggered when transaction in the trading pool is abandoned before execution,
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

// to be continued...
