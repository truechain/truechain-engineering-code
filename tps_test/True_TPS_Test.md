## True TPS Test

### **一：测试环境**

**1：源码地址：**
```
git clone https://github.com/truechain/truechain-engineering-code.git
```
切换到tps\_test\_v1.1.3 分支

**2：go版本：**
```
go version go1.10.4 linux/amd6
```
**3：服务器配置：**

硬件配置：阿里云4vCPU、16GiB、高效云盘ECS服务器

操作系统：Ubuntu  18.04 64位

带宽：阿里云局域网内网传输

**4：节点数目**

4个节点分别对应服务器名称：host\_1，host\_2，host\_3，host\_4

### **二：测试过程**

**1：下载源码包进行编译**

编译：切换到项目目录使用命令：
```
make getrue
```
**2：使用json文件**

4个节点都使用同一个json文件进行初始化，请参tps\_test目录下的[genesis.json](https://github.com/truechain/truechain-engineering-code/blob/tps_test_v1.1.3/tps_test/genesis.json)文件

**3：初始化过程**

切换到项目目录
```
build/bin/getrue --datadir getrue\_data init ./genesis.json
```
**4：****bftkey****值**

四个节点每台服务器对应的bftkey值如下所示
```
Host\_1:

b169a4945fc7a436601865199e9b848616ae25dad947fd659625f6c0722f826f

Host\_2:

b698adfcfb7b42aeaefb572ebf789eab76bc3370ccd7a5eba29ee56f67956be8

Host\_3:

ae2dae5e525705582d61492f3767f85a7a146031e028ed25f88b1511565fe1a3

Host\_4:

74a6cb08943ce2871101588be401b5673324c2106e25b83fe81a7847e1f0c546
```
注：在初始化完成后把对应主机的bftkey文件替换到相应位置

把getrue\_data/getrue/bftkey文件覆盖掉

**5：getrue启动参数**

切换到项目目录
```
build/bin/getrue --datadir getrue\_data --port 60606 --verbosity 1 --syncmode "fast" --bftip "172.26.73.55"\--bftkeyhex "b169a4945fc7a436601865199e9b848616ae25dad947fd659625f6c0722f826f" --gcmode "archive"  --rpc --rpcaddr "0.0.0.0" --rpcport 8888 --rpccorsdomain "\*" --rpcvhosts "\*"  --rpcapi "eth,etrue,net,web3,personal" console
```
注：启动参数中指定的IP地址为本机的IP地址，\--bftkeyhex为账户的私钥

**6：编译发送交易程序**

切换到源码路径下  
```
cd truechain-engineering-code/send\_transaction/tbft/

go build main.go 生成可执行文件 main
```
**7：发送交易启动参数**
```
./main 1000 1000000 1000 100000 0 1
```
### **三：测试结果**

进入console使用命令查看测试结果
```
debug.metrics(false).etrue.pbftAgent
![](https://github.com/truechain/truechain-engineering-code/blob/tps_test_v1.1.3/tps_test/test_result.png)
```
