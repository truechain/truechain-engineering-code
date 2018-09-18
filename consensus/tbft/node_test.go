package consensus

import (
	"fmt"
	"testing"
)

func TestNoteBuild(t *testing.T) {
	n, e := NewNode(nil, "", nil, nil)
	fmt.Println(n, e)
}
