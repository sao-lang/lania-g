module github.com/sao-lang/lania-g/test

go 1.25.0

require (
	github.com/segmentio/kafka-go v0.4.51
	go.opentelemetry.io/otel/sdk v1.43.0
	go.opentelemetry.io/otel/sdk/metric v1.43.0
	google.golang.org/grpc v1.81.0
	google.golang.org/protobuf v1.36.11
	gorm.io/gorm v1.31.1
)

require (
	golang.org/x/mod v0.33.0 // indirect
	golang.org/x/tools v0.42.0 // indirect
)

require (
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/andybalholm/brotli v1.0.5 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/gabriel-vasile/mimetype v1.4.13 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.30.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/gomodule/redigo v1.9.3 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gookit/color v1.5.4 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/graphql-go/graphql v0.8.1 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/klauspost/compress v1.18.2 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/montanaflynn/stats v0.7.1 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
	github.com/sao-lang/lania-g/application/v3 v3.0.0
	github.com/sao-lang/lania-g/integrations/auth/v3 v3.0.0
	github.com/sao-lang/lania-g/integrations/cache/v3 v3.0.0
	github.com/sao-lang/lania-g/integrations/config/v3 v3.0.0
	github.com/sao-lang/lania-g/integrations/events/v3 v3.0.0
	github.com/sao-lang/lania-g/integrations/grpc/v3 v3.0.0
	github.com/sao-lang/lania-g/integrations/kafka/v3 v3.0.0
	github.com/sao-lang/lania-g/integrations/logger/v3 v3.0.0
	github.com/sao-lang/lania-g/integrations/orm/v3 v3.0.0
	github.com/sao-lang/lania-g/integrations/otel/v3 v3.0.0
	github.com/sao-lang/lania-g/integrations/outbox/v3 v3.0.0
	github.com/sao-lang/lania-g/integrations/resilience/v3 v3.0.0
	github.com/sao-lang/lania-g/kernel/v3 v3.0.0
	github.com/sao-lang/lania-g/protocol/graphql/v3 v3.0.0
	github.com/sao-lang/lania-g/protocol/grpc/v3 v3.0.0
	github.com/sao-lang/lania-g/protocol/http/v3 v3.0.0
	github.com/sao-lang/lania-g/protocol/mq/v3 v3.0.0
	github.com/sao-lang/lania-g/protocol/scheduler/v3 v3.0.0
	github.com/sao-lang/lania-g/protocol/ws/v3 v3.0.0
	github.com/sao-lang/lania-g/tools v0.0.0
	github.com/vmihailenco/msgpack/v5 v5.3.5 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.1.2 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/xo/terminfo v0.0.0-20210125001918-ca9a967f8778 // indirect
	github.com/youmark/pkcs8 v0.0.0-20240726163527-a2c0da244d78 // indirect
	github.com/zishang520/engine.io v1.5.9 // indirect
	github.com/zishang520/engine.io-go-parser v1.2.2 // indirect
	github.com/zishang520/socket.io v1.3.2 // indirect
	github.com/zishang520/socket.io-go-parser v1.0.4 // indirect
	go.mongodb.org/mongo-driver v1.17.9 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.43.0 // indirect
	go.opentelemetry.io/otel/metric v1.43.0 // indirect
	go.opentelemetry.io/otel/trace v1.43.0 // indirect
	golang.org/x/crypto v0.49.0 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.35.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260226221140-a57be14db171 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/sao-lang/lania-g/integrations/orm/v3 => ../integrations/orm/v3

replace github.com/sao-lang/lania-g/protocol/graphql/v3 => ../protocol/graphql/v3

replace github.com/sao-lang/lania-g/application/v3 => ../application/v3

replace github.com/sao-lang/lania-g/integrations/auth/v3 => ../integrations/auth/v3

replace github.com/sao-lang/lania-g/integrations/logger/v3 => ../integrations/logger/v3

replace github.com/sao-lang/lania-g/protocol/scheduler/v3 => ../protocol/scheduler/v3

replace github.com/sao-lang/lania-g/integrations/outbox/v3 => ../integrations/outbox/v3

replace github.com/sao-lang/lania-g/protocol/mq/v3 => ../protocol/mq/v3

replace github.com/sao-lang/lania-g/protocol/ws/v3 => ../protocol/ws/v3

replace github.com/sao-lang/lania-g/tools => ../tools

replace github.com/sao-lang/lania-g/integrations/grpc/v3 => ../integrations/grpc/v3

replace github.com/sao-lang/lania-g/integrations/kafka/v3 => ../integrations/kafka/v3

replace github.com/sao-lang/lania-g/integrations/otel/v3 => ../integrations/otel/v3

replace github.com/sao-lang/lania-g/integrations/resilience/v3 => ../integrations/resilience/v3

replace github.com/sao-lang/lania-g/kernel/v3 => ../kernel/v3

replace github.com/sao-lang/lania-g/protocol/grpc/v3 => ../protocol/grpc/v3

replace github.com/sao-lang/lania-g/protocol/http/v3 => ../protocol/http/v3

replace github.com/sao-lang/lania-g/integrations/cache/v3 => ../integrations/cache/v3

replace github.com/sao-lang/lania-g/integrations/config/v3 => ../integrations/config/v3

replace github.com/sao-lang/lania-g/integrations/events/v3 => ../integrations/events/v3
