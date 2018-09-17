package truescan

import (
	"flag"
	"testing"
)

var (
	address string
)

func init() {
	flag.StringVar(&address, "address", "", "address")
	flag.Parse()
}
func TestRedisClient(t *testing.T) {
	rc, err := NewRedisClient(address, 1)
	if err != nil {
		t.Error(err.Error())
	}
	err = rc.Ping()
	if err != nil {
		t.Error(err.Error())
	}
	t.Log("The message is successfully sent. Please check the client subscription.")
}

// go test -v core/redis_test.go core/redis.go -args -address=<redis server address>
