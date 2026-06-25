module github.com/sao-lang/lania-g/integrations/otel/v3

go 1.25.0

require (
	github.com/sao-lang/lania-g/application/v3 v3.0.0
	github.com/sao-lang/lania-g/integrations/logger/v3 v3.0.0
	github.com/sao-lang/lania-g/kernel/v3 v3.0.0
	github.com/google/uuid v1.6.0
	go.opentelemetry.io/otel v1.43.0
	go.opentelemetry.io/otel/metric v1.43.0
	go.opentelemetry.io/otel/sdk v1.43.0
	go.opentelemetry.io/otel/sdk/metric v1.43.0
	go.opentelemetry.io/otel/trace v1.43.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/gabriel-vasile/mimetype v1.4.13 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.30.2 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	golang.org/x/crypto v0.49.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.35.0 // indirect
)

replace github.com/sao-lang/lania-g/application/v3 => ../../../application/v3

replace github.com/sao-lang/lania-g/integrations/logger/v3 => ../../../integrations/logger/v3

replace github.com/sao-lang/lania-g/kernel/v3 => ../../../kernel/v3

replace github.com/sao-lang/lania-g/protocol/http/v3 => ../../../protocol/http/v3
