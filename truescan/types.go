package truescan

import "github.com/truechain/truechain-engineering-code/core/types"

// Config about redis service.
type Config struct {
	RedisHost string
	RedisPort int
	ChannelID int
	Password  string
}

// TransactionMsg used in corresponding message transmission
type TransactionMsg struct {
	Nonce    uint64 `json:"nonce"`
	Hash     string `json:"hash"`
	From     string `json:"from"`
	To       string `json:"to"`
	Value    string `json:"value"`
	Gas      uint64 `json:"gas"`
	GasPrice string `json:"gasPrice"`
	Input    string `json:"input"`
}

// FullTransactionMsg used in corresponding message transmission
type FullTransactionMsg struct {
	Nonce    uint64 `json:"nonce"`
	Hash     string `json:"hash"`
	From     string `json:"from"`
	To       string `json:"to"`
	Value    string `json:"value"`
	Gas      uint64 `json:"gas"`
	GasPrice string `json:"gasPrice"`
	Input    string `json:"input"`
	// Receipts
	PostState         string       `json:"root"`
	Status            bool         `json:"status"`
	CumulativeGasUsed uint64       `json:"cumulativeGasUsed"`
	Bloom             string       `json:"logsBloom"`
	Logs              []*types.Log `json:"logs"`
	ContractAddress   string       `json:"contractAddress"`
	GasUsed           uint64       `json:"gasUsed"`
}

// RemoveTxMsg used in corresponding message transmission
type RemoveTxMsg struct {
	Hash string `json:"hash"`
}

// FastBlockHeaderMsg used in corresponding message transmission
type FastBlockHeaderMsg struct {
	Number     uint64                `json:"number"`
	Hash       string                `json:"hash"`
	ParentHash string                `json:"parentHash"`
	ExtraData  string                `json:"extraData"`
	Size       uint64                `json:"size"`
	GasLimit   uint64                `json:"gasLimit"`
	GasUsed    uint64                `json:"gasUsed"`
	Timestamp  uint64                `json:"timestamp"`
	ViewNumber uint64                `json:"viewNumber"`
	Txs        []*FullTransactionMsg `json:"txs"`
}

// FruitHeaderMsg used in corresponding message transmission
type FruitHeaderMsg struct {
	Number     uint64 `json:"number"`
	Hash       string `json:"hash"`
	Nonce      uint64 `json:"nonce"`
	Miner      string `json:"miner"`
	Difficulty uint64 `json:"difficulty"`
}

// SnailBlockHeaderMsg used in corresponding message transmission
type SnailBlockHeaderMsg struct {
	Number           uint64            `json:"number"`
	Hash             string            `json:"hash"`
	ParentHash       string            `json:"parentHash"`
	Nonce            uint64            `json:"nonce"`
	Miner            string            `json:"miner"`
	Difficulty       uint64            `json:"difficulty"`
	ExtraData        string            `json:"extraData"`
	Size             uint64            `json:"size"`
	GasLimit         uint64            `json:"gasLimit"`
	GasUsed          uint64            `json:"gasUsed"`
	Timestamp        uint64            `json:"timestamp"`
	Fruits           []*FruitHeaderMsg `json:"fruits"`
	StartFruitNumber uint64            `json:"startFruitNumber"`
	EndFruitNumber   uint64            `json:"endFruitNumber"`
}

// ChangeViewMsg used in corresponding message transmission
type ChangeViewMsg struct {
	ViewNumber      uint64   `json:"viewNumber"`
	Members         []string `json:"members"`
	BeginFastNumber uint64   `json:"beginFastNumber"`
	EndFastNumber   uint64   `json:"endFastNumber"`
}

// Account include address and value
type Account struct {
	Address string `json:"address"`
	Value   string `json:"value"`
}

// StateChangeMsg used in corresponding message transmission
type StateChangeMsg struct {
	Height   uint64     `json:"height"`
	Balances []*Account `json:"balances"`
	Rewards  []*Account `json:"rewards"`
}
