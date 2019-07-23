package public

import "github.com/truechain/truechain-engineering-code/params"

const (
	Normal = iota
	DiscTooManyPeers
	DownloaderCall
	ServerSimpleCall
	ServerSignedCall
	FetcherSyncCall
	FetcherHeadCall
	FetcherKnownCall
	FetcherTimerCall
	FetcherDeliverCall
	FetcherAnnounceCall
	FetcherFastTimerCall

	FruitHead = iota + 1
	Fruit
)

// IndexerConfig includes a set of configs for chain indexers.
type IndexerConfig struct {
	// The block frequency for creating CHTs.
	ChtSize uint64

	// A special auxiliary field represents client's chtsize for server config, otherwise represents server's chtsize.
	PairChtSize uint64

	// The number of confirmations needed to generate/accept a canonical hash help trie.
	ChtConfirms uint64

	// The block frequency for creating new bloom bits.
	BloomSize uint64

	// The number of confirmation needed before a bloom section is considered probably final and its rotated bits
	// are calculated.
	BloomConfirms uint64

	// The block frequency for creating BloomTrie.
	BloomTrieSize uint64

	// The number of confirmations needed to generate/accept a bloom trie.
	BloomTrieConfirms uint64
}

var (
	// DefaultServerIndexerConfig wraps a set of configs as a default indexer config for server side.
	DefaultServerIndexerConfig = &IndexerConfig{
		ChtSize:           params.CHTFrequencyServer,
		PairChtSize:       params.CHTFrequencyClient,
		ChtConfirms:       params.HelperTrieProcessConfirmations,
		BloomSize:         params.BloomBitsBlocks,
		BloomConfirms:     params.BloomConfirms,
		BloomTrieSize:     params.BloomTrieFrequency,
		BloomTrieConfirms: params.HelperTrieProcessConfirmations,
	}
	// DefaultClientIndexerConfig wraps a set of configs as a default indexer config for client side.
	DefaultClientIndexerConfig = &IndexerConfig{
		ChtSize:           params.CHTFrequencyClient,
		PairChtSize:       params.CHTFrequencyServer,
		ChtConfirms:       params.HelperTrieConfirmations,
		BloomSize:         params.BloomBitsBlocksClient,
		BloomConfirms:     params.HelperTrieConfirmations,
		BloomTrieSize:     params.BloomTrieFrequency,
		BloomTrieConfirms: params.HelperTrieConfirmations,
	}
	// TestServerIndexerConfig wraps a set of configs as a test indexer config for server side.
	TestServerIndexerConfig = &IndexerConfig{
		ChtSize:           64,
		PairChtSize:       512,
		ChtConfirms:       4,
		BloomSize:         64,
		BloomConfirms:     4,
		BloomTrieSize:     512,
		BloomTrieConfirms: 4,
	}
	// TestClientIndexerConfig wraps a set of configs as a test indexer config for client side.
	TestClientIndexerConfig = &IndexerConfig{
		ChtSize:           512,
		PairChtSize:       64,
		ChtConfirms:       32,
		BloomSize:         512,
		BloomConfirms:     32,
		BloomTrieSize:     512,
		BloomTrieConfirms: 32,
	}
)
