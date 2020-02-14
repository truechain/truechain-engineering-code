## TrueChain Impawn CLI

TrueChain Impawn CLI is a tool, which can call deposit contract participate in POS.

<a href="https://github.com/truechain/truechain-engineering-code/blob/master/COPYING"><img src="https://img.shields.io/badge/license-GPL%20%20truechain-lightgrey.svg"></a>

## Building the source


Building impawn requires both a Go (version 1.9 or later) and a C compiler.
You can install them using your favourite package manager.
Once the dependencies are installed, run

    go build -o impawn  main.go query_stake.go impawn.go


### Command

The impawn project comes with several Sub Command.

|    SubCommand    | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| :-----------: | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
|  `append`   | After deposit, if you want continue to participate in deposit, you can use append command, no fix fee and pubkey.          |
|   `cancel`    | If you want withdraw you money, First of all, you must cancel it. |
|  `delegate`   | You can find a validator address to delegate, contain sub command `deposit`,`cancel`,`withdraw`.                                             |
|  `querystaking`     | If you want withdraw you money, you should send tx after lock height,which will print this height.                  |
| `querytx` | If there no have validator to process your transaction, you can waiting some minutes and use it to query.              |
|   `send`   | If you want send no contract transaction, you can use send command.       |
|   `updatefee`   | If you want modify delegate fee, you only use this.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
|   `withdraw`   | After send a cancel transaction, you can withdraw you money in correct height.                                                                                                                                        |
### Flag
  * `--key` If you use private key, you need specify a file which contains private key. 
  * `--keystore` If you use private key, you need specify a file which contains private key. 
  * `--rpcaddr` HTTP-RPC server listening interface (default: `localhost`)
  * `--rpcport` HTTP-RPC server listening port (default: `8545`)
  * `--value` Staking value units one true no wei
  * `--fee` Staking fee 0 - 10000(default: 0)
  * `--address` Transfer address or validator address in delegate
  * `--txhash` query tx exec result
## Running CLI

### Impawn

```
$ impawn --key key/bftkey --rpcaddr 39.100.97.129 --rpcport 8545 --value 50000 --fee 100

```

This command will:

 * Load private key in key/bftkey file, connect http://39.100.97.129:8545 node, this node should you run validator node, because of it will use your local bft pk to election.
 * Deposit 50000 true to staking address, fee(0-10000) set to 100, feeRate = fee / 10000.
 * If you want become a validator candidate, you must deposit balance > 50000(true). 
 
### Cancel

```
$ impawn --key key/bftkey --rpcaddr 39.100.97.129 --rpcport 8545 --value 10  cancel

```

This command will:

 * Load private key in key/bftkey file, connect http://39.100.97.129:8545 node.
 * Due to withdraw must call cancel first, sub command cancel represent you will cancel 10 true to locked state, next epoch you can withdraw. 

### withdraw

```
$ impawn --key key/bftkey --rpcaddr 39.100.97.129 --rpcport 8545 --value 10 withdraw

```

This command will:

 * Load private key in key/bftkey file, connect http://39.100.97.129:8545 node.
 * Sub command append represent you want withdraw 10 true to your address. 

### Append

```
$ impawn --key key/bftkey --rpcaddr 39.100.97.129 --rpcport 8545 --value 10 append

```

This command will:

 * Load private key in key/bftkey file, connect http://39.100.97.129:8545 node, and deposit 10 true to staking address.
 * Sub command append represent you want continue staking after already having deposit.  

### UpdateFee

```
$ impawn --key key/bftkey --rpcaddr 39.100.97.129 --rpcport 8545 --fee 10 updatefee

```

This command will:

 * Load private key in key/bftkey file, connect http://39.100.97.129:8545 node.
 * Sub command only update validator fee(0-10000), which will influence delegator benefit.

### Send

```
$ impawn --key key/bftkey --rpcaddr 39.100.97.129 --rpcport 8545 --value 10 --address 0x3f944d3f12e904e1A647E5FF9f531B8deE2346B2 send 

```

This command will:

 * Load private key in key/bftkey file, connect http://39.100.97.129:8545 node.
 * Sub command send is send normal transaction not contract,value is transfer value, address is To address.

### QueryStaking

```
$ impawn --key key/bftkey --rpcaddr 39.100.97.129 --rpcport 8545 querystaking

```

This command will:

 * Load private key in key/bftkey file, connect http://39.100.97.129:8545 node.
 * querystaking will print in staking count(Staked), already cancel count(Locked), can withdraw count(Unlocked).
 * Print withdraw height, after this, you can call withdraw. 

### QueryTx

```
$ impawn  --rpcaddr 39.100.97.129 --rpcport 8545 --txhash 0x40c78769add225421c45fa2e9dc206c1d9a03199f78c34644f3c0bf274f3066b querytx

```

This command will:

 * Connect http://39.100.97.129:8545 node, and use querytx command specify txhash to query.
 
### Delegate deposit
 
 ```
 $ impawn --key key/bftkey --rpcaddr 39.100.97.129 --rpcport 8545 --value 10 --address 0x3f944d3f12e904e1A647E5FF9f531B8deE2346B2 delegate deposit
 
 ```
 
 This command will:
 
  * Load private key in key/bftkey file, connect http://39.100.97.129:8545 node, and delegate 10 true to staking address.
  * Sub command delegate deposit express this is delegate call, address is you select validator address.
  
### Delegate cancel
 
 ```
 $ impawn --key key/bftkey --rpcaddr 39.100.97.129 --rpcport 8545 --value 10 --address 0x3f944d3f12e904e1A647E5FF9f531B8deE2346B2 delegate deposit
 
 ```
 
 This command will:
 
  * Load private key in key/bftkey file, connect http://39.100.97.129:8545 node, and delegate 10 true to staking address.
  * Sub command delegate cancel express this is cancel call, address is you select validator address.  
  
### Delegate withdraw
 
 ```
 $ impawn --key key/bftkey --rpcaddr 39.100.97.129 --rpcport 8545 --value 10 --address 0x3f944d3f12e904e1A647E5FF9f531B8deE2346B2 delegate withdraw
 
 ```
 
 This command will:
 
  * Load private key in key/bftkey file, connect http://39.100.97.129:8545 node, and delegate 10 true to staking address.
  * Sub command delegate withdraw express this is withdraw call, address is you select validator address.  