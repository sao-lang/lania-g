module github.com/sao-lang/lania-g/test

go 1.25.0

require (
	github.com/sao-lang/lania-g/application/v3 v3.0.0
	github.com/sao-lang/lania-g/integrations/auth/v3 v3.0.0
	github.com/sao-lang/lania-g/integrations/cache/v3 v3.0.0
	github.com/sao-lang/lania-g/integrations/config/v3 v3.0.0
	github.com/sao-lang/lania-g/integrations/es/v3 v3.0.0
	github.com/sao-lang/lania-g/integrations/events/v3 v3.0.0
	github.com/sao-lang/lania-g/integrations/grpc/v3 v3.0.0
	github.com/sao-lang/lania-g/integrations/http/v3 v3.0.0
	github.com/sao-lang/lania-g/integrations/kafka/v3 v3.0.0
	github.com/sao-lang/lania-g/integrations/logger/v3 v3.0.0
	github.com/sao-lang/lania-g/integrations/minio/v3 v3.0.0
	github.com/sao-lang/lania-g/integrations/mongodb/v3 v3.0.0
	github.com/sao-lang/lania-g/integrations/orm/v3 v3.0.0
	github.com/sao-lang/lania-g/integrations/otel/v3 v3.0.0
	github.com/sao-lang/lania-g/integrations/outbox/v3 v3.0.0
	github.com/sao-lang/lania-g/integrations/resilience/v3 v3.0.0
	github.com/sao-lang/lania-g/integrations/swagger/v3 v3.0.0
	github.com/sao-lang/lania-g/integrations/terminus/v3 v3.0.0
	github.com/sao-lang/lania-g/kernel/v3 v3.0.0
	github.com/sao-lang/lania-g/protocol/graphql/v3 v3.0.0
	github.com/sao-lang/lania-g/protocol/grpc/v3 v3.0.0
	github.com/sao-lang/lania-g/protocol/http/v3 v3.0.0
	github.com/sao-lang/lania-g/protocol/mq/v3 v3.0.0
	github.com/sao-lang/lania-g/protocol/scheduler/v3 v3.0.0
	github.com/sao-lang/lania-g/protocol/ws/v3 v3.0.0
)

replace (
	github.com/sao-lang/lania-g/application/v3 => ../application/v3
	github.com/sao-lang/lania-g/integrations/auth/v3 => ../integrations/auth/v3
	github.com/sao-lang/lania-g/integrations/cache/v3 => ../integrations/cache/v3
	github.com/sao-lang/lania-g/integrations/config/v3 => ../integrations/config/v3
	github.com/sao-lang/lania-g/integrations/es/v3 => ../integrations/es/v3
	github.com/sao-lang/lania-g/integrations/events/v3 => ../integrations/events/v3
	github.com/sao-lang/lania-g/integrations/grpc/v3 => ../integrations/grpc/v3
	github.com/sao-lang/lania-g/integrations/http/v3 => ../integrations/http/v3
	github.com/sao-lang/lania-g/integrations/kafka/v3 => ../integrations/kafka/v3
	github.com/sao-lang/lania-g/integrations/logger/v3 => ../integrations/logger/v3
	github.com/sao-lang/lania-g/integrations/minio/v3 => ../integrations/minio/v3
	github.com/sao-lang/lania-g/integrations/mongodb/v3 => ../integrations/mongodb/v3
	github.com/sao-lang/lania-g/integrations/orm/v3 => ../integrations/orm/v3
	github.com/sao-lang/lania-g/integrations/otel/v3 => ../integrations/otel/v3
	github.com/sao-lang/lania-g/integrations/outbox/v3 => ../integrations/outbox/v3
	github.com/sao-lang/lania-g/integrations/resilience/v3 => ../integrations/resilience/v3
	github.com/sao-lang/lania-g/integrations/swagger/v3 => ../integrations/swagger/v3
	github.com/sao-lang/lania-g/integrations/terminus/v3 => ../integrations/terminus/v3
	github.com/sao-lang/lania-g/kernel/v3 => ../kernel/v3
	github.com/sao-lang/lania-g/protocol/graphql/v3 => ../protocol/graphql/v3
	github.com/sao-lang/lania-g/protocol/grpc/v3 => ../protocol/grpc/v3
	github.com/sao-lang/lania-g/protocol/http/v3 => ../protocol/http/v3
	github.com/sao-lang/lania-g/protocol/mq/v3 => ../protocol/mq/v3
	github.com/sao-lang/lania-g/protocol/scheduler/v3 => ../protocol/scheduler/v3
	github.com/sao-lang/lania-g/protocol/ws/v3 => ../protocol/ws/v3
)
