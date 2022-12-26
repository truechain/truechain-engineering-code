module github.com/truechain/truechain-engineering-code

go 1.15

replace (
	github.com/Azure/azure-storage-go v0.0.0-20170302221508-12ccaadb081c => github.com/loinfish/azure-storage-go v0.0.1
	github.com/agl/ed25519 v0.0.0-20170116200512-5312a6153412 => github.com/neoiss/ed25519 v0.0.0-20221222024706-01d22003a3d1
	gopkg.in/fatih/set.v0 v0.0.0-20141210084824-27c40922c40b => github.com/loinfish/set v0.0.1
)

require (
	github.com/Azure/azure-storage-go v0.0.0-20170302221508-12ccaadb081c
	github.com/Azure/go-autorest/autorest v0.11.28 // indirect
	github.com/StackExchange/wmi v0.0.0-20180116203802-5d049714c4a6 // indirect
	github.com/VictoriaMetrics/fastcache v1.5.8
	github.com/agl/ed25519 v0.0.0-20170116200512-5312a6153412
	github.com/allegro/bigcache v1.2.1-0.20190218064605-e24eb225f156
	github.com/apilayer/freegeoip v3.5.1-0.20180702111401-3f942d1392f6+incompatible
	github.com/aristanetworks/goarista v0.0.0-20170210015632-ea17b1a17847
	github.com/btcsuite/btcd v0.0.0-20171128150713-2e60448ffcc6
	github.com/cespare/cp v0.1.0
	github.com/cloudflare/cloudflare-go v0.10.1
	github.com/davecgh/go-spew v1.1.1
	github.com/deckarep/golang-set v0.0.0-20180603214616-504e848d77ea
	github.com/docker/docker v17.12.0-ce-rc1.0.20210128214336-420b1d36250f+incompatible
	github.com/elastic/gosigar v0.8.1-0.20180330100440-37f05ff46ffa
	github.com/fatih/color v1.8.0
	github.com/fjl/memsize v0.0.0-20180418122429-ca190fb6ffbc
	github.com/fsnotify/fsnotify v1.4.7 // indirect
	github.com/gizak/termui v2.2.1-0.20170117222342-991cd3d38091+incompatible
	github.com/go-ole/go-ole v1.2.1 // indirect
	github.com/go-stack/stack v1.5.4
	github.com/golang/protobuf v1.5.2
	github.com/golang/snappy v0.0.3
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/gofuzz v1.0.0 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/hashicorp/golang-lru v0.5.4
	github.com/holiman/bloomfilter/v2 v2.0.3
	github.com/holiman/uint256 v1.2.0
	github.com/howeyc/fsnotify v0.9.1-0.20151003194602-f0c08ee9c607 // indirect
	github.com/hpcloud/tail v1.0.0 // indirect
	github.com/huin/goupnp v0.0.0-20161224104101-679507af18f3
	github.com/influxdata/influxdb v1.2.3-0.20180221223340-01288bdb0883
	github.com/jackpal/go-nat-pmp v1.0.2-0.20160603034137-1fa385a6f458
	github.com/julienschmidt/httprouter v1.3.0
	github.com/karalabe/hid v0.0.0-20170821103837-f00545f9f374
	github.com/kr/pretty v0.3.0 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/maruel/panicparse v0.0.0-20160720141634-ad661195ed0e // indirect
	github.com/maruel/ut v1.0.2 // indirect
	github.com/mattn/go-colorable v0.1.0
	github.com/mattn/go-isatty v0.0.4
	github.com/mattn/go-runewidth v0.0.13 // indirect
	github.com/mitchellh/go-wordwrap v0.0.0-20150314170334-ad45545899c7 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826
	github.com/naoina/go-stringutil v0.1.0 // indirect
	github.com/naoina/toml v0.1.2-0.20170918210437-9fafd6967416
	github.com/nsf/termbox-go v0.0.0-20170211012700-3540b76b9c77 // indirect
	github.com/onsi/ginkgo v1.11.0 // indirect
	github.com/onsi/gomega v1.4.1 // indirect
	github.com/oschwald/maxminddb-golang v1.3.1-0.20190523235738-1960b16a5147 // indirect
	github.com/pborman/uuid v1.2.0
	github.com/peterh/liner v1.0.1-0.20170902204657-a37ad3984311
	github.com/pkg/errors v0.9.1
	github.com/prometheus/prometheus v1.7.2-0.20170814170113-3101606756c5
	github.com/rjeczalik/notify v0.9.1
	github.com/robertkrimen/otto v0.0.0-20170205013659-6a77b7cbc37d
	github.com/rogpeppe/go-internal v1.8.1 // indirect
	github.com/rs/cors v0.0.0-20160617231935-a62a804a8a00
	github.com/rs/xhandler v0.0.0-20160618193221-ed27b6fd6521 // indirect
	github.com/stretchr/testify v1.8.1
	github.com/syndtr/goleveldb v0.0.0-20180502072349-ae970a0732be
	github.com/tendermint/go-amino v0.12.0
	golang.org/x/crypto v0.0.0-20220722155217-630584e8d5aa
	golang.org/x/net v0.0.0-20211112202133-69e39bad7dc2
	golang.org/x/sys v0.0.0-20210615035016-665e8c7367d1
	golang.org/x/time v0.0.0-20220224211638-0e9765cccd65 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
	gopkg.in/fatih/set.v0 v0.0.0-20141210084824-27c40922c40b
	gopkg.in/fsnotify.v1 v1.4.7 // indirect
	gopkg.in/karalabe/cookiejar.v2 v2.0.0-20150724131613-8dcd6a7f4951
	gopkg.in/natefinch/npipe.v2 v2.0.0-20160621034901-c1b8fa8bdcce
	gopkg.in/olebedev/go-duktape.v3 v3.0.0-20180302121509-abf0ba0be5d5
	gopkg.in/sourcemap.v1 v1.0.5 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/urfave/cli.v1 v1.20.0
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gotest.tools v2.2.0+incompatible // indirect
)
