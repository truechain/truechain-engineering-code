
package whisperv6

import (
	"bytes"
	"crypto/ecdsa"
	"testing"
	"time"

	"github.com/truechain/truechain-engineering-code/common"
	set "gopkg.in/fatih/set.v0"
)

func TestMultipleTopicCopyInNewMessageFilter(t *testing.T) {
	w := &Whisper{
		privateKeys:   make(map[string]*ecdsa.PrivateKey),
		symKeys:       make(map[string][]byte),
		envelopes:     make(map[common.Hash]*Envelope),
		expirations:   make(map[uint32]*set.SetNonTS),
		peers:         make(map[*Peer]struct{}),
		messageQueue:  make(chan *Envelope, messageQueueLimit),
		p2pMsgQueue:   make(chan *Envelope, messageQueueLimit),
		quit:          make(chan struct{}),
		syncAllowance: DefaultSyncAllowance,
	}
	w.filters = NewFilters(w)

	keyID, err := w.GenerateSymKey()
	if err != nil {
		t.Fatalf("Error generating symmetric key: %v", err)
	}
	api := PublicWhisperAPI{
		w:        w,
		lastUsed: make(map[string]time.Time),
	}

	t1 := [4]byte{0xde, 0xea, 0xbe, 0xef}
	t2 := [4]byte{0xca, 0xfe, 0xde, 0xca}

	crit := Criteria{
		SymKeyID: keyID,
		Topics:   []TopicType{TopicType(t1), TopicType(t2)},
	}

	_, err = api.NewMessageFilter(crit)
	if err != nil {
		t.Fatalf("Error creating the filter: %v", err)
	}

	found := false
	candidates := w.filters.getWatchersByTopic(TopicType(t1))
	for _, f := range candidates {
		if len(f.Topics) == 2 {
			if bytes.Equal(f.Topics[0], t1[:]) && bytes.Equal(f.Topics[1], t2[:]) {
				found = true
			}
		}
	}

	if !found {
		t.Fatalf("Could not find filter with both topics")
	}
}
