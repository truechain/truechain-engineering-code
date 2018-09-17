package truescan

// TransactionMsg used in corresponding message transmission
type TransactionMsg struct {
	Hash     string `json:"hash"`
	From     string `json:"from"`
	To       string `json:"to"`
	Value    string `json:"value"`
	Gas      uint64 `json:"gas"`
	GasPrice string `json:"gasPrice"`
	Input    string `json:"input"`
}

// FastBlockHeaderMsg used in corresponding message transmission
type FastBlockHeaderMsg struct {
	Number     uint64            `json:"number"`
	Hash       string            `json:"hash"`
	ParentHash string            `json:"parentHash"`
	ExtraData  string            `json:"extraData"`
	Size       uint64            `json:"size"`
	GasLimit   uint64            `json:"gasLimit"`
	GasUsed    uint64            `json:"gasUsed"`
	Timestamp  uint64            `json:"timestamp"`
	Txs        []*TransactionMsg `json:"txs"`
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
