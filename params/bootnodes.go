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
// the main Ethereum network.
var MainnetBootnodes = []string{
	// Ethereum Foundation Go Bootnodes
	//"enode://64f706387bd0fd22852406ea6e1f376f6fce2ea6fad47e7ab25e8511081818a0f19871b7a4844f0660f57699f057de8764e0618251dc4e0b559b6f8c3b43d895@47.92.209.38:9215",
	//"enode://7d2142bcff0f3ea520f5230948e3d807ffb24ccb9ca3831adc454911b06f347df36c69b7d2642a4fc9f7e04f08768c6c8b20caffa5a5152bdfd44b3c39a72792@47.92.172.168:30301",
	//"enode://da44dc833d7b40cc5c8475cb01a1d3996778db42dd2aa6f5b423b28bdb7f6b57be550d607c80871a7e4b49efc2f362643dda84c769831f8f21cd89d87637db04@47.92.118.221:9215",
	//"enode://14ae6dbbd988f57f498e280a14fbaf82e1d994057b97c57a8224a09d18b326703fa2c053c88e5c7e44035ec2897dd4c0db56c93041451144b286cc51bbd3038d@47.92.30.181:9215",
	"enode://64f706387bd0fd22852406ea6e1f376f6fce2ea6fad47e7ab25e8511081818a0f19871b7a4844f0660f57699f057de8764e0618251dc4e0b559b6f8c3b43d895@47.92.209.100:9215",
	"enode://07e866b8e19ba682f677ccb4d3cbd00768fbc7a08e24202a1903d998b2f25af8da38c0ad9d06987388f60458ab6cbdbdb8239dc355be8a277e52de5e9834284e@47.92.135.242:9215",
	"enode://99b0117d29ac053f977fc65a2bf3a322bd2a58357b30450e6e28ffd48deb262fd367fdc11c020e0a3bc5ab7a0c1ddf8adf9cc224233c53feca6c4b04ff4cec56@47.92.198.8:9215",
	"enode://c74a96e58a189ccd0d0c8a1699c6cbe04f1e648034ce7d261da26564ddbb1cfd2769df24bbd879390c46835eaf477cca40111ea91799d0df01c247e136521bce@47.92.207.147:9215",
	"enode://190cb9a8bae11ac54dd1d2d96cb91ed1673fcd44c4a01b3c92b43a936d6e9c2004321ebf2214af11660f4b0c3d4a51ac861e9008ea1c4c691ebb8e36a876e3fc@47.92.75.213:9215",
}

// TestnetBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Ropsten test network.
var TestnetBootnodes = []string{
	"enode://7f8edaa4bc698a19666019f3858e35cc5b420907d61eb585aafd51a6a6f62e47f6cc5767890a9a38b51a3409520db566f04e60372b8523ecd9e770aa764ee603@47.92.224.044:9215",
	"enode://f54641b27755a6284ce6c3bf44410f6e83cc433604ac9cb8c862e4b41c5900ad0c4cd68c9fa51779732bc37ff9b34153eaa68b2d95b1d641db325fde211f6482@47.92.214.211:9215",
	"enode://1a998266cc8357581e1b2405f76039804d000fdbe71e58bc41f3facf2b9086d50303caf18be59bc2ed4bd283021a34e93e469ef8bae82540dff3c27f50467f1d@47.92.220.171:9215",
	"enode://676bf12d833e2c00e75ef0217980a6f2bfcd13e41cfaf4e47baab46b18042cd655c4b13e313dffa8e36b32bf6c7175bd29f2db7fb1d08465aa81e1859bf6e991@47.92.194.176:9215",
	"enode://676bf12d833e2c00e75ef0217980a6f2bfcd13e41cfaf4e47baab46b18042cd655c4b13e313dffa8e36b32bf6c7175bd29f2db7fb1d08465aa81e1859bf6e991@47.92.211.169:9215",
}

// RinkebyBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Rinkeby test network.
var RinkebyBootnodes = []string{
	"enode://a24ac7c5484ef4ed0c5eb2d36620ba4e4aa13b8c84684e1b4aab0cebea2ae45cb4d375b77eab56516d34bfbd3c1a833fc51296ff084b770b94fb9028c4d25ccf@52.169.42.101:30303", // IE
	"enode://343149e4feefa15d882d9fe4ac7d88f885bd05ebb735e547f12e12080a9fa07c8014ca6fd7f373123488102fe5e34111f8509cf0b7de3f5b44339c9f25e87cb8@52.3.158.184:30303",  // INFURA
	"enode://b6b28890b006743680c52e64e0d16db57f28124885595fa03a562be1d2bf0f3a1da297d56b13da25fb992888fd556d4c1a27b1f39d531bde7de1921c90061cc6@159.89.28.211:30303", // AKASHA
}
