module github.com/sao-lang/lania-g/integrations/kafka/v3

go 1.25.0

require (
	github.com/sao-lang/lania-g/kernel/v3 v3.0.0
	github.com/sao-lang/lania-g/protocol/mq/v3 v3.0.0
	github.com/segmentio/kafka-go v0.4.51
)

require (
	github.com/gabriel-vasile/mimetype v1.4.13 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.30.2 // indirect
	github.com/klauspost/compress v1.15.9 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
	golang.org/x/crypto v0.49.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.35.0 // indirect
)

replace github.com/sao-lang/lania-g/kernel/v3 => ../../../kernel/v3

replace github.com/sao-lang/lania-g/protocol/mq/v3 => ../../../protocol/mq/v3
