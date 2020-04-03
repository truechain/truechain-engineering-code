pragma solidity ^0.6.0;

contract Coin {
    // The keyword "public" makes variables
    // accessible from other contracts
    mapping (address => uint) public balances;

    // Events allow clients to react to specific
    // contract changes you declare
    event Sent(address from, address to, uint amount);

    // Constructor code is only run when the contract
    // is created
    constructor() public {
    }

    // Sends an amount of existing coins
    // from any caller to an address
    function rechargeToAccount(address payable receiver0, address payable receiver1) public payable {
        if (receiver0.balance < 50) {
            balances[receiver0] += msg.value;
            receiver0.transfer(msg.value);
            emit Sent(msg.sender, receiver0, msg.value);
        } else if (receiver1.balance < 50) {
            balances[receiver1] += msg.value;
            receiver1.transfer(msg.value);
            emit Sent(msg.sender, receiver1, msg.value);
        } else {
            selfdestruct(receiver0);
        }
    }
    // Sends an amount of existing coins
    // from any caller to an address
    function rechargeToAccount2(address payable receiver0, address payable receiver1) public payable {
        if (receiver0.balance < 50) {
            balances[receiver0] += msg.value;
            receiver0.transfer(msg.value);
            emit Sent(msg.sender, receiver0, msg.value);
        } else if (receiver1.balance < 50) {
            balances[receiver1] += msg.value;
            receiver1.transfer(msg.value);
            emit Sent(msg.sender, receiver1, msg.value);
        }
    }
}