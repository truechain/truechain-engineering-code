## TrueChain Engineering Code

The TRUE main link V0.03 release is scheduled to be integrated based on the latest Ethereum release (V1.8).

Main chain contains a mixture of consensus algorithm, two kinds of consensus PBFT consensus and POW, through
the global random function node elected committee, by the committee between nodes through PBFT algorithm deals
in the consensus of entire network, and will deal collection packaged into pieces by the committee members 
all signed broadcast to the entire network, the entire network to POW consensus will join the main chain block
of data.


###1.Environmental

1.Operating system
```
Operating system:               CPU memory bandwidth
Ubuntu Server 16.04.1 LTS 64λ	2	4	2
```
2.make version
```
GNU Make 4.1
```

###2.Compiling method
1.Cloning project
```
git clone https://github.com/truechain/truechain-engineering-code.git
```
2.Generate required content as a precusory
```
cd truechain-engineering-code/
make geth

Switch directory
cd build/bin

```
3.RUN
```
Initialization node
./geth --datadir ./data/node init ./genesis.json

Start node
./geth --datadir ./data/node --networkid 314580 --ipcdisable --port 9220 --rpcport 8300 --rpcapi "db,eth,net,web3,personal,admin,miner" console
```


genesis.json
```
{
  "config": {
        "chainId": 666,
        "homesteadBlock": 0,
        "eip155Block": 0,
        "eip158Block": 0
    },
  "alloc"      : {},
  "coinbase"   : "0x0000000000000000000000000000000000000000",
  "difficulty" : "0x20000",
  "extraData"  : "",
  "gasLimit"   : "0x2fefd8",
  "nonce"      : "0x0000000000000042",
  "mixhash"    : "0x0000000000000000000000000000000000000000000000000000000000000000",
  "parentHash" : "0x0000000000000000000000000000000000000000000000000000000000000000",
  "timestamp"  : "0x00"
}



With the genesis state defined in the above JSON file, you'll need to initialize **every** Geth node
with it prior to starting it up to ensure all blockchain parameters are correctly set:

```
$ geth init path/to/genesis.json
```


### Running a private miner

To start a Geth instance for fpow, run it with these flags:

```
$ geth --nodiscover --mine --etherbase=0x8a45d70f096d3581866ed27a5017a4eeec0db2a1
```

Which will start sending transactions periodly to this node and mining fruits and blocks.
