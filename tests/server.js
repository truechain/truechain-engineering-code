const Web3 = require('web3')

const PROVIDER = 'http://39.105.126.32:8545'
const FROM_ADDRESS = '0x7eFf122b94897EA5b0E2A9abf47B86337FAfebdC'
const TO_ADDRESS = '0xca73a25939a929acc64c1372a22cfb736aaafd56'
const TX_COUNT = 10
const PASSWORD = '1234'

function sleep(time) {
  return new Promise(resolve => setTimeout(resolve, time))
}

let blockInfo = {}

const web3 = new Web3(PROVIDER)
console.log('web3 server start')
console.log('-----------------')
web3.eth.personal.unlockAccount(FROM_ADDRESS, PASSWORD).then(async success => {
  if (!success) {
    throw new Error('unable unlock account')
  }
  // const nonceBefore = await web3.eth.getTransactionCount(FROM_ADDRESS, 'pending')
  // console.log(`nonce before test: ${nonceBefore}`)
  const time = new Dat()
  for (let i = 0; i < TX_COUNT; i++) {
    await sleep(2)
    web3.eth.sendTransaction({
      // nonce: nonceBefore + i,
      from: FROM_ADDRESS,
      to: TO_ADDRESS,
      chainId: '151515',
      value: '0',
      gas: '21000',
     gasPrice: '0'
    }).then(tx => {
      let num = tx.blockNumber
      if (!num) {
        return
      } else if (!blockInfo[num]) {
        blockInfo[num] = 0
      }
      blockInfo[num] ++
    }).catch(err => {
      void(0)
      console.error(err.message)
    })
  }
  const dtime = new Date() - time
  console.log(`spend ${dtime}ms`)
  await sleep(2000)
 console.log('-----------------')
 // const nonceAfter = await web3.eth.getTransactionCount(FROM_ADDRESS, 'pending')  console.log(`send ${TX_COUNT}txs`)
  console.log('block info:')
  for (const num in blockInfo) {
    if (blockInfo.hasOwnProperty(num)) {
      const txCount = blockInfo[num]
      const totalTxCount = await web3.eth.getBlockTransactionCount(num)
      console.log(`> in block ${num} send ${txCount} txs, has ${totalTxCount} txs`)
    }
  }
  console.log(blockInfo)
}).catch(err => {
  console.error(err.message)
})
