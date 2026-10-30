module github.com/tx7do/go-wind-flow/server

go 1.26.0

require (
	github.com/tx7do/go-crud/api v0.0.7
	github.com/tx7do/go-wind-flow v0.0.0
	github.com/tx7do/go-wind-flow/api v0.0.0
	google.golang.org/grpc v1.85.0
	google.golang.org/protobuf v1.36.12
)

require (
	github.com/google/gnostic v0.7.1 // indirect
	github.com/google/gnostic-models v0.7.1 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/net v0.60.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20261019180522-ce908d29299b // indirect
)

replace github.com/tx7do/go-wind-flow => ../

replace github.com/tx7do/go-wind-flow/api => ../api
