
package truechain

import  (
    

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


func (CMG *CommitteeMemberGroup)GetPbftNodesFromCfg() []string{

	file, _ := os.Open("conf.json")
 
    defer file.Close()
 
   
    decoder := json.NewDecoder(file)
 
 
    conf := configuration{}
    
    err := decoder.Decode(&conf) 
    if err != nil {
        fmt.Println("Error:", err)
    }
	
	

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



 

