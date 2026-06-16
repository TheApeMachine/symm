module github.com/theapemachine/symm

go 1.26.1

require (
	github.com/bytedance/sonic v1.15.2
	github.com/smartystreets/goconvey v1.8.1
	github.com/spf13/cobra v1.10.2
	go.yaml.in/yaml/v3 v3.0.4
	golang.org/x/sync v0.21.0
)

require (
	capnproto.org/go/capnp/v3 v3.1.0-alpha.2 // indirect
	github.com/andybalholm/brotli v1.2.1 // indirect
	github.com/bytedance/gopkg v0.1.4 // indirect
	github.com/bytedance/sonic/loader v0.5.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudwego/base64x v0.1.7 // indirect
	github.com/colega/zeropool v0.0.0-20230505084239-6fb4a4f75381 // indirect
	github.com/elastic/elastic-transport-go/v8 v8.11.0 // indirect
	github.com/elastic/go-elasticsearch/v9 v9.4.1 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/fxamacker/cbor/v2 v2.9.2 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/gofiber/schema v1.7.1 // indirect
	github.com/gofiber/utils/v2 v2.0.6 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gopherjs/gopherjs v1.20.2 // indirect
	github.com/jtolds/gls v4.20.0+incompatible // indirect
	github.com/klauspost/compress v1.18.6 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.22 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/phuslu/log v1.0.124 // indirect
	github.com/sagikazarmark/locafero v0.11.0 // indirect
	github.com/savsgio/gotils v0.0.0-20250924091648-bce9a52d7761 // indirect
	github.com/smarty/assertions v1.16.0 // indirect
	github.com/smarty/go-disruptor v0.5.0 // indirect
	github.com/sourcegraph/conc v0.3.1-0.20240121214520-5f936abd7ae8 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/tinylib/msgp v1.6.4 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.71.0 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.43.0 // indirect
	go.opentelemetry.io/otel/metric v1.43.0 // indirect
	go.opentelemetry.io/otel/trace v1.43.0 // indirect
	golang.org/x/arch v0.27.0 // indirect
	golang.org/x/crypto v0.52.0 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	golang.org/x/tools v0.44.0 // indirect
)

require (
	github.com/fasthttp/websocket v1.5.12
	github.com/gofiber/contrib/v3/websocket v1.2.0
	github.com/gofiber/fiber/v3 v3.3.0
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/smallnest/ringbuffer v0.1.1
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/spf13/viper v1.21.0
	github.com/theapemachine/datura v1.2.3
	github.com/theapemachine/errnie v1.2.4
	github.com/theapemachine/nomagique v0.0.2
	github.com/theapemachine/qpool v1.2.4
	gonum.org/v1/gonum v0.17.0
)

replace (
	github.com/theapemachine/datura => ../datura
	github.com/theapemachine/errnie => ../errnie
	github.com/theapemachine/nomagique => ../nomagique
	github.com/theapemachine/qpool => ../qpool
)
