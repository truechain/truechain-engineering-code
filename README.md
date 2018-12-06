## TrueChain Engineering Code

TrueChain is a truly fast, permissionless, secure and scalable public blockchain platform which is supported by hybrid consensus technology called Minerva and a global developer community. 
 
TrueChain uses hybrid consensus combining PBFT and fPoW to solve the biggest problem confronting public blockchain: the contradiction between decentralization and efficiency. 

TrueChain uses PBFT as fast-chain to process transactions, and leave the oversight and election of PBFT to the hands of PoW nodes. Besides, TrueChain integrates fruitchain technology into the traditional PoW protocol to become fPoW, 
 to make the chain even more decentralized and fair. 
 
 TrueChain also creates a hybrid consensus incentive model and a stable gas fee mechanism to lower the cost for the developers and operators of DApps, and provide better infrastructure for decentralized eco-system. 


## Building the source


Building getrue requires both a Go (version 1.9 or later) and a C compiler.
You can install them using your favourite package manager.
Once the dependencies are installed, run

    make getrue

or, to build the full suite of utilities:

    make all

The execuable command getrue will be found in the `cmd` directory.

## Running getrue

### Defining the private genesis state

First, you'll need to create the genesis state of your networks, which all nodes need to be aware of
and agree upon. We provide a single node JSON file at cmd/getrue/genesis_single.json:

```json
{
  "config": {
    "chainId": 10,
    "minerva":{"durationLimit" : "0x3c",
      "minimumDifficulty":"0x7d0",
      "minimumFruitDifficulty":"0x32"
      }
  },
  "alloc":{
    "0x7c357530174275dd30e46319b89f71186256e4f7" : { "balance" : 90000000000000000000000},
    "0x4cf807958b9f6d9fd9331397d7a89a079ef43288" : { "balance" : 90000000000000000000000}
  },
  "committee":[
    {
      "address": "0x76ea2f3a002431fede1141b660dbb75c26ba6d97",
      "publickey": "0x04044308742b61976de7344edb8662d6d10be1c477dd46e8e4c433c1288442a79183480894107299ff7b0706490f1fb9c9b7c9e62ae62d57bd84a1e469460d8ac1"
    }
  ]
,
  "coinbase"   : "0x0000000000000000000000000000000000000000",
  "difficulty" : "0x100",
  "extraData"  : "",
  "gasLimit"   : "0x1500000",
  "nonce"      : "0x0000000000000042",
  "mixhash"    : "0x0000000000000000000000000000000000000000000000000000000000000000",
  "parentHash" : "0x0000000000000000000000000000000000000000000000000000000000000000",
  "timestamp"  : "0x00"
}
```

With the genesis state defined in the above JSON file, you'll need to initialize **every** Getrue node
with it prior to starting it up to ensure all blockchain parameters are correctly set:

```
$ getrue init path/to/genesis.json
```


### Running a private miner

To start a getrue instance for single node, use genesis as above, and run it with these flags:

```
$ getrue --nodiscover --singlenode --mine --election --etherbase 0x8a45d70f096d3581866ed27a5017a4eeec0db2a1 --bftkeyhex "c1581e25937d9ab91421a3e1a2667c85b0397c75a195e643109938e987acecfc" --bftip "192.168.68.43" console
```

Which will start sending transactions periodly to this node and mining fruits and blocks.
