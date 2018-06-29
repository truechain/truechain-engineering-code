
package truechain

import  (
    //"strconv"
    //"log"
	//p "github.com/ethereum/truechain-engineering-code/eth/truechain"
	//"bytes"
	//"encoding/binary"
	//"fmt"

	"encoding/json"
	"fmt"
	"os"
)

type configuration struct {
    Enabled bool
	Path    string
	node	[]string
}
 

type CommitteeMemberGroup struct{
  Cmg []CommitteeMember
  tt  *TrueHybrid 
  

}
//func (CMG *CommitteeMemberGroup)Init(){
//
//   for i := 1; i<= 5 ; i++{
//      i_addr := "127.0.0." + strconv.Itoa(i)
//      i_Nodeid := "Nodeid_" + strconv.Itoa(i)
//     Init := CommitteeMember{
//         Nodeid: i_Nodeid,
//         addr:   i_addr,
//         port:   34567}
//         CMG.Cmg = append(CMG.Cmg, Init)
//   }
//}

func (CMG *CommitteeMemberGroup)GetPbftNodesFromCfg() []string{

	file, _ := os.Open("conf.json")
 
    defer file.Close()
 
   
    decoder := json.NewDecoder(file)
 
 
    conf := configuration{}
    
    err := decoder.Decode(&conf) 
    if err != nil {
        fmt.Println("Error:", err)
    }
	
	 
	//buf := new(bytes.Buffer)
	//m := make([]CommitteeMember,0)

	// var data []interface{}
	// for i := 0; i < 5; i++{
	// 	I_addr := "127.0.0." + strconv.Itoa(i)
	// 	I_Nodeid := "Nodeid_" + strconv.Itoa(i)

	// 	data = []interface{}{
	// 		string(I_Nodeid),
	// 		string(I_addr),
	// 		int(34567)}

	// }


	// for _, v := range data {
	// 	err := binary.Write(buf, binary.LittleEndian, v)
	// 	if err != nil {
	// 		fmt.Println("binary.Write failed:", err)
	// 	}
	// 	Test  := CommitteeMember{}
	// 	Test.FromByte(buf.Bytes())
	// 	//m = append(m, Test)
	// 	CMG.Cmg =  append(CMG.Cmg, Test)
	// }

	// fmt.Println(CMG.Cmg)

	return conf.node
}



func (tt  *TrueHybrid )GetFirstStart()bool{
	file, _ := os.Open("conf.json")
 
    defer file.Close()
 
   
    decoder := json.NewDecoder(file)
 
 
    conf := configuration{}
    
    err := decoder.Decode(&conf) 
    if err != nil {
        fmt.Println("Error:", err)
    }
    return conf.Enabled
}



 

