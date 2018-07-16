package truechain

import (
	"testing"
	"fmt"
)

func Test1(t *testing.T) {
	member := &CommitteeMember{
		"111",
		"127.4.6.9",
		8087,
	}
	bytes,_ :=toByte(member)
	m :=&CommitteeMember{
		Nodeid:"444",
	}
	fromByte(bytes,m)
	fmt.Println(m)
}
