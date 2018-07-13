
package main

import (
	"log"
	pb "github.com/pbftServer"
	"google.golang.org/grpc"
	//"time"
	//"golang.org/x/net/context"
	"encoding/gob"
	"bytes"
	"fmt"
	"net"
	"google.golang.org/grpc/reflection"
)

const (
	address     = "localhost:38535"
)

func GenerateData()	 *pb.Transactions{
	var txArr []*pb.Transaction
	TD := pb.InitTxDataTest()
	for _,td := range TD {
		tx := &pb.Transaction{Data:&td}
		txArr = append(txArr,tx)
	}
	txs := &pb.Transactions{Txs:txArr}
	fmt.Println("len:",len(txs.Txs))
	return txs
}


func DeserializeBlock(d []byte) *pb.TruePbftBlock {
	if len(d) == 0{
		return nil
	}
	var block pb.TruePbftBlock
	decoder := gob.NewDecoder(bytes.NewReader(d))
	err := decoder.Decode(&block)
	if err != nil {
		log.Panic(err)
	}
	return &block
}

//func runSetTransactions(client pb.PyHybConsensusClient,txs *pb.Transactions){
//	log.Printf("已启动 ")
//	ctx, cancel := context.WithTimeout(context.Background(), 1000*time.Millisecond)
//	defer cancel()
//
//	//生成虚拟交易
//	txs = GenerateData()
//	commonReply,err := client.SetTransactions(ctx, txs)
//	if err != nil {
//		log.Panic(err)
//	}
//	fmt.Println("message: ",commonReply.Message)
//}
//func main() {
//	flag.Parse()
//	conn, err := grpc.Dial(address, grpc.WithInsecure())
//	if err != nil {
//		log.Fatalf("fail to dial: %v", err)
//	}
//	defer conn.Close()
//	client := pb.NewPyHybConsensusClient(conn)
//
//	for i:=0; i<4; i++ {
//		runSetTransactions(client, &pb.Transactions{})
//		_, cancel := context.WithTimeout(context.Background(), 2000*time.Millisecond)
//		defer cancel()
//	}
//}

func main(){
	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterHybridConsensusHelpServer(s, &pServer{})
	// Register reflection service on gRPC server.
	reflection.Register(s)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}

}
