package pbftServer


import (
	"net"
	"github.com/truechain/truechain-engineering-code/crypto/sha3"
	"github.com/truechain/truechain-engineering-code/rlp"
	"log"
	"golang.org/x/net/context"
	"time"
	"google.golang.org/grpc"
	"sync"
	"google.golang.org/grpc/reflection"
	"fmt"
	"math/rand"
	"crypto/sha256"
	"bytes"
	"encoding/gob"
	"crypto/ecdsa"
	"math/big"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/crypto"
	"encoding/hex"
)

const (
	port = ":17546"
	address   = "45.40.241.133:17546"

)

type pServer struct {
}

var (
	privkeys = make([]*ecdsa.PrivateKey,0,0)
	keysCount = 6
	txHashs  [][]byte
	allTxs = make([]*Transaction,0,0)
	mutex = &sync.Mutex{}
)
var heartbeatRe chan bool = make(chan bool)

func randomRange(min, max int) int {
	rand.Seed(time.Now().Unix())
	return rand.Intn(max-min) + min
}

func InitTxDataTest()[]TxData{
	total :=randomRange(6,10)
	var TD   []TxData
	for i := 0;i <total; i++ {
		r :=10*i+i%2+i%1
		test := TxData{
			AccountNonce: 	uint64(1 + r),
			Price:       	int64((r + 1) * 10),
			GasLimit:    	int64((r + 1) * 100),
			Recipient:  	 nil,
			Amount:      	int64((r + 1) * 1000),
			Payload:     	nil,
			V:           	int64((r + 1) * 1000),
			R:           	int64((r + 1) * 1000),
			S:           	int64((r + 1) * 1000),
		}
		if i !=7{
			test.Hash =test.DoHash()
		}else{
			test.Hash =[]byte{}
		}
		TD = append(TD, test)
	}
	return TD
}

func (txData TxData) Serialize() []byte {
	var encoded bytes.Buffer
	enc := gob.NewEncoder(&encoded)
	err := enc.Encode(txData)
	if err != nil {
		log.Panic(err)
	}

	return encoded.Bytes()
}

func (txData TxData) DoHash() []byte {
	var hash [32]byte
	hash = sha256.Sum256(txData.Serialize())
	return hash[:]
}

func init(){
	for i:=0;i<keysCount;i++ {
		k,_ := crypto.GenerateKey()
		privkeys = append(privkeys,k)
	}
}

func GetPub(priv *ecdsa.PrivateKey) *ecdsa.PublicKey {
	pub := ecdsa.PublicKey{
		Curve: 	priv.Curve,
		X: 		new(big.Int).Set(priv.X),
		Y: 		new(big.Int).Set(priv.Y),
	}
	return &pub
}

func rlpHash(x interface{}) (h []byte) {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
}

func DoBlock(pbTxs []*Transaction)error{
	conn, err := grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
		return err
	}
	defer conn.Close()
	c := NewHybridConsensusHelpClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var block *TruePbftBlock
	select {
	case ok := <-heartbeatRe:
		if ok  {
			block =GenerateBlock(pbTxs)
		}
	case <-time.After(time.Duration(1000) * time.Millisecond):
		block =GenerateBlock(pbTxs)
		break
	}

	_, err1 := c.PutBlock(ctx, block)
	if err1 != nil {
		return err1
	}

	return nil
}

func GenerateBlock(pbTxs  []*Transaction)*TruePbftBlock{



	now := time.Now().Unix()
	head := TruePbftBlockHeader{
		Number:				10,
		GasLimit:			100,
		GasUsed:			80,
		Time:				now,
	}
	block := TruePbftBlock{
		Header:			&head,
		Txs:			&Transactions{Txs:pbTxs},
	}
	mutex.Lock()
	if (len(pbTxs) >= len(allTxs)){
		allTxs =allTxs[:0]
	}else{
		allTxs =allTxs[len(pbTxs):len(allTxs)]
	}
	mutex.Unlock()
	fmt.Println("len of allTxs: ",len(allTxs))
	msg := rlpHash(block.Txs)
	var sigs []string
	for i:=0;i<keysCount/2;i++ {
		sig,err := crypto.Sign(msg,privkeys[i])
		if err != nil {
			log.Panic(err)
		}
		sigs = append(sigs,common.ToHex(sig))

		//sig :=strconv.Itoa(i)
		//sigs = append(sigs,sig)
	}
	block.Sigs =sigs
	fmt.Println("block:",block)

	return &block
}

func (t *pServer)SetTransactions(ctx context.Context,txs *Transactions) (*CommonReply,error){

	var block *TruePbftBlock
	//var pbTxs  []*Transaction
	fmt.Println("beign len:",len(txs.Txs))



	go DoBlock(allTxs)
CircleTxs:
	for _,vv := range txs.Txs {
		//to := make([]byte,0,0)
		//if tt := vv.To(); tt != nil {
		//	to = tt.Bytes()
		//}
		//v,r,s := vv.RawSignatureValues()

		for _,txHash := range txHashs{
			if bytes.Equal(txHash,vv.Data.Hash) == true{
				fmt.Println("vv.Data.Hash:",vv.Data.Hash)
				continue CircleTxs
			}
		}
		mutex.Lock()
		txHashs =append(txHashs,vv.Data.Hash)
		mutex.Unlock()

		data :=vv.Data
		tx := &Transaction{
			/*AccountNonce V  R  S   Price   GasLimit  Recipient  Amount  Payload  Hash*/
			Data:       &TxData{
				AccountNonce:       data.AccountNonce,
				Price:              data.Price,
				GasLimit:           data.GasLimit,
				Recipient:			data.Recipient,
				Amount:             data.Amount,
				Payload:			data.Payload,
				V:                  data.V,
				R:                  data.R,
				S:                  data.S,
				Hash:				data.Hash,
			},
		}
		mutex.Lock()
		allTxs = append(allTxs,tx)
		mutex.Unlock()
	}
	fmt.Println("end len:",len(allTxs))

	mutex.Lock()
	if len(allTxs)>=30 && !<-heartbeatRe{
		heartbeatRe <- true
	}
	mutex.Unlock()

	fmt.Println("block",block)

	return &CommonReply{Message: "success"}, nil
}

func (s *pServer) Start(ctx context.Context, b *BftPrivateKey) (*CommonReply, error) {
	priv := privkeys[1]
	pub := GetPub(priv)
	nodeid := hex.EncodeToString(crypto.FromECDSAPub(pub))
	return &CommonReply{Message:nodeid}, nil
}

func (s *pServer) Stop(ctx context.Context, e *EmptyParam) (*CommonReply, error) {
	return &CommonReply{Message: "success "}, nil
}

func (s *pServer) MembersNodes(ctx context.Context, a *Nodes) (*CommonReply, error) {
	return &CommonReply{Message: "success "}, nil
}

func StartPbftServers() {
	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	RegisterPyHybConsensusServer(s, &pServer{})
	// Register reflection service on gRPC server.
	reflection.Register(s)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}

}

