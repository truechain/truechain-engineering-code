// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package params

// MainnetBootnodes are the enode URLs of the P2P bootstrap nodes running on
// the main Truechain network.
var MainnetBootnodes = []string{
	"enode://23afb7b27408aa9e75342055842a0296c8d1cff7451d25a2e70ee6ff48333915b67a1dc3428c5eebc3e2bc5cf1778cfb31278ef4f6ab9e1d3bfab60e13790616@47.241.184.79:30313", // CN
	"enode://f8dcc2a5c18ef8128e6c33f08a5870b69b42fc66a85760f76aa687055fe76f45622e950c027c4b36bf02457e73fb5d0f5640c1fa7eaf63c879c0eba9e4c958a0@13.56.161.52:30313",  // US WEST
	"enode://23afb7b27408aa9e75342055842a0296c8d1cff7451d25a2e70ee6ff48333915b67a1dc3428c5eebc3e2bc5cf1778cfb31278ef4f6ab9e1d3bfab60e13790616@47.241.184.79:30313", // SG
	"enode://4c64220af42271b6a6ea5463e97a125fef86d0bbb077db7d669af9d020d8ccf8ef4b617e3b36bbb9c10096404ecc1a7e06bcec3210a2cdf49b2bce5a0e1c7eb5@8.209.88.41:30313",   // DE

	"enode://8aef1642e8a9dc1f52febf53e2e0c54d5068ad0dea54dc1d308c93280ac833b19a5ca1853a24f896d54e648b674b13143f14e7d8ec8188f20a4747c048ead517@101.132.179.192:30313", // CN
}

// TestnetBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Ropsten test network.
var TestnetBootnodes = []string{
	"enode://a395d2799c1e63307b9a5ecc44729e9ba2fb8fa6d64e362e8498ce9aba85b7b405755ad28bd662a9a48d941bbbfe18d29e0ea46105258110e2318fd6faab8c09@39.108.212.229:30313", // CN
	"enode://946dd380c75f756696e4183a3bba661f5a72dcd4af231189a966de7fb2b561ecdff7ef531ca090b6c22e32876368c5360a069d3ca709a107359d511c248eb0ac@52.167.174.211:30313", // US
	//"enode://50ac2f679890052610954f986157d434eeef8ed78eb3b2da62334f5822658ac96eff75e3d12fbf81289a6fb559cec03ba8179db02e98459b5eeb86b15d80cf69@138.128.216.58:30313", // EU
	"enode://cf04b2cfadb241358c8a08001e88244f79c1e12f8d3f57251c27b8cf5010dc7588c2de75fe9ea09eecfa3c7d16b2513290d3a3d1d1203324fef77e6fc231c707@47.74.185.172:30313", // SG
}

// DevnetBootnodes are the enode URLs of the P2P bootstrap nodes running on
// the dev Truechain network.
var DevnetBootnodes = []string{
	"enode://ec1e13e3d0177196a55570dfc1c810b2ea05109cb310c4dc7397ae6f3109467ec0d13a5f28ebdfb553511d492a4892ffa3a8283ce69bc5f93fce079dbfbfa5f4@39.100.120.25:30310",
}

// DiscoveryV5Bootnodes are the enode URLs of the P2P bootstrap nodes for the
// experimental RLPx v5 topic-discovery network.
var DiscoveryV5Bootnodes = []string{
	"enode://ebb007b1efeea668d888157df36cf8fe49aa3f6fd63a0a67c45e4745dc081feea031f49de87fa8524ca29343a21a249d5f656e6daeda55cbe5800d973b75e061@39.98.171.41:30315",
	"enode://b5062c25dc78f8d2a8a216cebd23658f170a8f6595df16a63adfabbbc76b81b849569145a2629a65fe50bfd034e38821880f93697648991ba786021cb65fb2ec@39.98.43.179:30312",
}
