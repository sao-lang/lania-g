module github.com/sao-lang/lania-g/cmd

go 1.25.0

require (
	github.com/sao-lang/lania-g/application/v3 v3.0.0
	github.com/sao-lang/lania-g/integrations/swagger/v3 v3.0.0
	github.com/sao-lang/lania-g/kernel/v3 v3.0.0
	github.com/sao-lang/lania-g/protocol/graphql/v3 v3.0.0
	github.com/sao-lang/lania-g/protocol/grpc/v3 v3.0.0
	github.com/sao-lang/lania-g/protocol/http/v3 v3.0.0
	github.com/sao-lang/lania-g/protocol/ws/v3 v3.0.0
	google.golang.org/grpc v1.81.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/gabriel-vasile/mimetype v1.4.13 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.30.2 // indirect
	github.com/gofrs/uuid v4.0.0+incompatible // indirect
	github.com/gomodule/redigo v1.8.4 // indirect
	github.com/googollee/go-socket.io v1.7.0 // indirect
	github.com/gorilla/websocket v1.4.2 // indirect
	github.com/graphql-go/graphql v0.8.1 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	golang.org/x/crypto v0.49.0 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.35.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260226221140-a57be14db171 // indirect
)

replace github.com/sao-lang/lania-g/application/v3 => ../application/v3

replace github.com/sao-lang/lania-g/kernel/v3 => ../kernel/v3

replace github.com/sao-lang/lania-g/protocol/graphql/v3 => ../protocol/graphql/v3

replace github.com/sao-lang/lania-g/protocol/grpc/v3 => ../protocol/grpc/v3

replace github.com/sao-lang/lania-g/protocol/http/v3 => ../protocol/http/v3

replace github.com/sao-lang/lania-g/integrations/swagger/v3 => ../integrations/swagger/v3

replace github.com/sao-lang/lania-g/protocol/ws/v3 => ../protocol/ws/v3
