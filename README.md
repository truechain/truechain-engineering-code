## TrueChain Engineering Code

prototype for TrueChain fruit chain consensus

Refer to:
https://eprint.iacr.org/2016/916.pdf


## Building the source


Building getrue requires both a Go (version 1.7 or later) and a C compiler.
You can install them using your favourite package manager.
Once the dependencies are installed, run

    make
    make getrue

or, to build the full suite of utilities:

    make
    make all

The execuable command getrue will be found in the `cmd` directory.

## Running getrue

### Defining the private genesis state

First, you'll need to create the genesis state of your networks, which all nodes need to be aware of
and agree upon. This consists of a small JSON file (e.g. call it `genesis.json`):

```json
{
  "config": {
    "chainId": 10,
    "homesteadBlock": 0,
    "eip155Block": 0,
    "eip158Block": 0
  },
  "alloc":{
    "0xbd54a6c8298a70e9636d0555a77ffa412abdd71a" : { "balance" : 90000000000000000000000},
    "0x3c2e0a65a023465090aaedaa6ed2975aec9ef7f9" : { "balance" : 10000000000000000000000}
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
  "gasLimit"   : "0x2fefd8",
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
$ getrue --nodiscover --singlenode --bft --mine --etherbase 0x8a45d70f096d3581866ed27a5017a4eeec0db2a1 --bftkeyhex "c1581e25937d9ab91421a3e1a2667c85b0397c75a195e643109938e987acecfc" --bftip "192.168.68.43" --bftport 10080
```

Which will start sending transactions periodly to this node and mining fruits and blocks.
