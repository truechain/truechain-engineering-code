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
	"enode://cd99daa76de43e2b7a5806c3455d33012cd127bca9b2e271be3af5d78e402c153a77e1d408f708770fb390e597621407f963f1c444090c21f91e03e03caa2110@39.98.216.197:30313", // CN
	"enode://e95937d68263a59c95ac1199eecc450b3590624accaf1542c7e51d8dc3ca3bfa6d3f60785b021c408b4a9a67b2869da33237c75448ae29b70506164a2bfe6931@13.52.156.74:30313",  // US WEST
	"enode://85ac935873a1ac9a898e371e4583ef9ffbd91ce580a647bf9875ef890108bfad4ade4b74efe4b510aaeeaa7096c11278600abf98eefa319843a6d2dbbc3c56a4@104.160.39.87:30313", // US EAST
	"enode://9032cc37954363b4d2dd37a898959aadf213718ff1bdb146848fb8c9a5adfd31d543ca870a08a223b27da2309051d0ce41775fa6de9337ed519b64cfa85b5b0c@52.77.99.47:30313",   // SG
	"enode://6f5f92f2515c96f1f222e2de70c47022c0976947d1e7a42576af2e2cbbbfc8fc44de0e5f4ecab51f4a0d0dfeb07018802f9dad030a2f1c61542c5f115f05c108@35.157.61.21:30313",  // DE

	"enode://fb331ff6aded86b393d9de2f9c449d313b356af0c4c0b9500e0f6c51bcb4ed31ca45dc2ab64c6182d1876eb9e3fd073d488277a40a6d357bc6e63350a2e00ffc@101.132.183.35:30313", // CN
}

// TestnetBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Ropsten test network.
var TestnetBootnodes = []string{
	"enode://a395d2799c1e63307b9a5ecc44729e9ba2fb8fa6d64e362e8498ce9aba85b7b405755ad28bd662a9a48d941bbbfe18d29e0ea46105258110e2318fd6faab8c09@39.108.212.229:30313", // CN
	"enode://946dd380c75f756696e4183a3bba661f5a72dcd4af231189a966de7fb2b561ecdff7ef531ca090b6c22e32876368c5360a069d3ca709a107359d511c248eb0ac@3.209.142.83:30313",   // US
	"enode://50ac2f679890052610954f986157d434eeef8ed78eb3b2da62334f5822658ac96eff75e3d12fbf81289a6fb559cec03ba8179db02e98459b5eeb86b15d80cf69@138.128.216.58:30313", // EU
	"enode://cf04b2cfadb241358c8a08001e88244f79c1e12f8d3f57251c27b8cf5010dc7588c2de75fe9ea09eecfa3c7d16b2513290d3a3d1d1203324fef77e6fc231c707@47.74.185.172:30313",  // SG
}

// DevnetBootnodes are the enode URLs of the P2P bootstrap nodes running on
// the dev Truechain network.
var DevnetBootnodes = []string{
	"enode://5b1e518049e36c975095efc54af7d351f8e7cc9a6f81a4db4ec87b105e4a5f6754dc7e417d61ff57732254b18a15000e8844bef6533a00f49442357f01423506@172.26.214.13:30313",
	"enode://28d7353497c2bdc9d7fd5a3a1ff08a684cc6a9ef8f3553b6e48a6e49001fabfb7f0ff33b152deebe99d0017b3577b6e844f25e7dd6973cb933dae41fe449596b@172.26.214.9:30313",
}
