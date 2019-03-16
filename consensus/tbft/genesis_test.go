package tbft

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/crypto"
	"io/ioutil"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func getPk(privs string) string {
	key, err := hex.DecodeString(privs)
	if err != nil {
		fmt.Println(err)
	}
	priv, err := crypto.ToECDSA(key)
	if err != nil {
		fmt.Println(err.Error())
	}
	pk := hex.EncodeToString(GetPub(priv))
	//fmt.Println(pk)
	return pk
}

func GetCommittee(cnt int) []map[string]interface{} {
	PrivArr := []string{
		"0577aa0d8e070dccfffc5add7ea64ab8a167a3a8badb4ce18e336838e4ce3757",
		"77b635c48aa8ef386a3cee1094de7ea90f58a634ddb821206e0381f06b860f0f",
		"d0c3b151031a8a90841dc18463d838cc8db29a10e7889b6991be0a3088702ca7",
		"c007a7302da54279edc472174a140b0093580d7d73cdbbb205654ea79f606c95",
		"9177485bfecacf47d2f9f63a0c29cc0eae299c955283ce1fbb2060fee041c040",
		"56fd29d16b6e6ed9e28d71905fc4514dfb57bbb2ce353a373d22d38f96fd9653",
		"63a57d50a6b295e427772460c71d65c651a1939ff8b65a6bfeaaba6815d40d49",
		"61273ae43cd45ebfced19da23bab4de17c6beaab5a93be88a82e93c91fe0acda",
		"20cd359032ba766bcb1468466cf17af81952e6a6be6c8968ed3b18b856950e04",
		"14deff62bd0a4b7968cb9fe7d08e41e48d0535ea791d01b5a6d590f265b1ae1c",
		"def826b08a92039b9aa6576b37fc7fe43b14a47add1811896330d3df160200b6",
		"c275034412d84dd499306c98a9b1b86bc0416e14518be8dc5f96810bf7cb7dad",
		"43d598a8cb1621f36a9bf1b3c6a2a90a841dbce015eff6f3e18d75bd32d5bf4f",
		"73cbeb5aba75192d496855e9229255a0427f04321993878c4c63ed3f775c7ace",
		"8e21d3d4aafb420c2d2b4088450ec162cd49e43437e5896eadca1a7acaa0f123",
		"9fd85d0b5d96fa3abac90741b2a9381361c290c417473404d1b7000c8b153ea4",
		"33fc70f37883dc7fd5d287e6005e56a77d9c2e08e4b9c81fe62eef46f7bd1795",
		"dc37637ccfbb687c347c5a7f325f9cab8bc7ef4e809d9af3ca9c8049d65cf632",
		"39b9865b91db7f90b9d0bf9255f08da52551c84907957a34bc5ad6dae1472972",
		"3289494109c94675c7193633d2bb784a792eed4114259630776fcb44760898c4",
		"47cbf3f696b64c0b6165163d9ee7e5c8ca1e09f82d678f388f09a35bade6e366",
		"9bd323b3e4021957018cdb81147a830d19bc3352c2492adfac377c1068c8a853",
		"0bf0ab615d5b14f491126f64392191f271bac1f6a2563a0f529e6c40cc54ba08",
		"20acbe5d6c3a0b6c8c3fefb626c4213e5bb8d76b0d4f82a9091272af46e9094e",
		"b3b9a14fbab67e2e5116df48d5e383d2f5581b46e93242db472f5961d95d1e4f",
		"e02a888b7af307634ad777a6159a5baefbda54e3ae1d452f320893ed6a58c3a6",
		"b7fa1e8b3a76d5e3e64e4874f055c4f59faeb838cbad91544466d0fdfa02a6f5",
		"80aa082c87dbc4749afb6fad6a4875d1e8c1e02b6a031f6b54e89615603c6fb0",
		"f0dbb1b4c785d87893c6421a58f3103c3bbb538ea2e72e0269f5444a016698df",
		"cdf4fe841825c7b281652ee54ba07c08ea6c3bccec72737965d3724504a0a410",
		"6031d64fca166a07719c95b15fdbdd9fff4ef2d89f7ec80a2beddfd41c7ac2c4",
		"0813e992dce0f659f4473dabacd6f7488d6ec64de7c8495122aa803bda40e677",
		"7052cca69ac1132bd418d97a26f72581975dcf72f27a5f242d72fbf87bfa0a09",
		"e042331fee4106536a4861312ca2d7817cfcaf414fb43a12618cdbb6a4a1d7e2",
		"1e0306b5ecdca0cf493adafed869aee3199af2eca2351dba00e8c4ddbac1a548",
		"82e84ee306354f6a0278800d0ac4a60cca9162537defc000985cd5b8b606e1e6",
		"ca18f33ad8eaef52b4af41c3d3408a1706e6b6ce0ef8f19e007a2635d83d9ec6",
		"e0305af76de5e418821353ff2988ece91c8cae1fc2e4c4f3246fbd6412c19a5a",
		"b3d661bc7e5fdd025c73f868cf491238785430a8f8bfa9182cc18eb6f14a2f1b",
		"e98bacfbcb186f46d4c7edb3a9a0764e43164cdb37aed4cf8b9448be7d069027",
		"a53c8e1dab6e40b4f53880dffd2c47a851442efbee32f5c144150dd8f23e9b0b",
		"6358710ab517ae8c013f8bb587ee941ac22cd361bbb28dc4a46a2bb7bf474a60",
		"60b8c3f8bddcabb70e008ac48e2eb80bdb09ec58f98cbbf6e5ec212c88c512fe",
		"e283683e75f65201783b0d8029fce16d3b191911474dacf98484153c3295a816",
		"0517b235fb517af5d54b6fe9b15407ac28496ebc2b2ca067c693d6b912afdddf",
		"682596e7f477b10a65c076c929761e7f60b0106cb53b391648b39a4e11f538e9",
		"bb0ac70399b73a338857a77eb131f8b50d6a69c07eebd349686821fdf458d96a",
		"d02ea7e907ddbf93d0228a801616ab32a5f9540f10621899ad9b115967968ea7",
		"1ee31cba8e9c9418c529f8021317436cfc6ae57fff089e27cc6dd6195f4c56fa",
		"d9261584fcaf11dd4ca75dcd4cf57dd60f511205da3b0d822aca69325442c70b",
		"2a013b718f498ffa4c258794dad547b58b954b0df01875b26ff72c3f637cd6f5",
		"6186f4de637df2d492e4d42365443e9229a57c7f330313a47d4cb4d628114788",
	}

	comm := make([]map[string]interface{}, 0)
	i := 0
	for _, v := range PrivArr {
		if i == cnt {
			break
		}
		value := make(map[string]interface{})
		addr := RandHexBytes20()
		value["address"] = "0x" + hex.EncodeToString(addr[:])
		value["publickey"] = "0x" + getPk(v)
		comm = append(comm, value)
		i++
	}
	return comm
}

func TestOutGenesisJson(t *testing.T) {

	b, _ := ioutil.ReadFile("E:\\truechain\\src\\github.com\\truechain\\truechain-engineering-code\\cmd\\getrue\\genesis.json")
	out := make(map[string]interface{})
	json.Unmarshal(b, &out)
	for i := 1; i <= 52; i++ {
		out["committee"] = GetCommittee(i)
		bytes, _ := json.Marshal(out)
		bytes = []byte(strings.Replace(string(bytes), "9e+22", "90000000000000000000000", -1))
		e := ioutil.WriteFile("E:\\truechain\\src\\github.com\\truechain\\truechain-engineering-code\\cmd\\getrue\\"+fmt.Sprintf("genesis_%s.json", strconv.Itoa(i)), bytes, os.ModePerm)
		if e != nil {
			fmt.Println(string(bytes))
		}
	}
}

func TestGetOrderPk(t *testing.T) {
	privArray := []string{
		"61273ae43cd45ebfced19da23bab4de17c6beaab5a93be88a82e93c91fe0acda",
		"73cbeb5aba75192d496855e9229255a0427f04321993878c4c63ed3f775c7ace",
		"9bd323b3e4021957018cdb81147a830d19bc3352c2492adfac377c1068c8a853",
		"0bf0ab615d5b14f491126f64392191f271bac1f6a2563a0f529e6c40cc54ba08",
		"b3b9a14fbab67e2e5116df48d5e383d2f5581b46e93242db472f5961d95d1e4f",
		"80aa082c87dbc4749afb6fad6a4875d1e8c1e02b6a031f6b54e89615603c6fb0",
		"cdf4fe841825c7b281652ee54ba07c08ea6c3bccec72737965d3724504a0a410",
		"0813e992dce0f659f4473dabacd6f7488d6ec64de7c8495122aa803bda40e677",
		"e042331fee4106536a4861312ca2d7817cfcaf414fb43a12618cdbb6a4a1d7e2",
		"e98bacfbcb186f46d4c7edb3a9a0764e43164cdb37aed4cf8b9448be7d069027",
		"6358710ab517ae8c013f8bb587ee941ac22cd361bbb28dc4a46a2bb7bf474a60",
		"60b8c3f8bddcabb70e008ac48e2eb80bdb09ec58f98cbbf6e5ec212c88c512fe",
		"2a013b718f498ffa4c258794dad547b58b954b0df01875b26ff72c3f637cd6f5",
	}

	var pk []string

	for _, v := range privArray {
		pk = append(pk, getPk(v))
	}

	sort.Strings(pk)
	fmt.Println(pk)
}
