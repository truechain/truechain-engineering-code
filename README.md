## TrueChain Engineering Code

prototype for TrueChain fruit chain consensus

Refer to:
https://eprint.iacr.org/2016/916.pdf


## Building the source


Building getrue requires both a Go (version 1.7 or later) and a C compiler.
You can install them using your favourite package manager.
Once the dependencies are installed, run

    make getrue

or, to build the full suite of utilities:

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
  "alloc"      : {
	  "0x970e8128ab834e8eac17ab8e3812f010678cf791" : { "balance" : 90000000000000000000000},
	  "0x68f2517b6c597ede0ae7c0559cdd4a84fd08c928" : { "balance" : 10000000000000000000000}
	  },
  "coinbase"   : "0x0000000000000000000000000000000000000000",
  "difficulty" : "0x200",
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

To start a Getrue instance for fpow, run it with these flags:

```
$ getrue --nodiscover --mine --etherbase=0x8a45d70f096d3581866ed27a5017a4eeec0db2a1
```

Which will start sending transactions periodly to this node and mining fruits and blocks.
