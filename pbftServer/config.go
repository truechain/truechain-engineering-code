package pbftServer

//import (
//"os"
//"path"
//)
//
//const OutputThreshold = 0
//
//const BasePort = 40540
//
//type Config struct {
//	N         int
//	KD        string
//	LD        string
//	IPList    []string
//	Ports     []int
//	GrpcPorts []int
//	HostsFile string
//	NumQuest  int
//	NumKeys   int
//	Blocksize int
//}
//
//
//func (cfg *Config) LoadPbftSimConfig() {
//	cfg.HostsFile = path.Join(os.Getenv("HOME"), "hosts")
//	cfg.NumKeys = len(cfg.IPList)
//	cfg.N = cfg.NumKeys - 1
//	cfg.NumQuest = 100
//	cfg.Blocksize = 10
//}
