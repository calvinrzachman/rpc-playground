module github.com/calvinrzachman/rpc-playground

go 1.13

require (
	github.com/calvinrzachman/rpc-playground/learnrpc v0.0.0
	github.com/golang/protobuf v1.4.3
	google.golang.org/grpc v1.34.0
)

replace github.com/calvinrzachman/rpc-playground/learnrpc => ./learnrpc

// https://thewebivore.com/using-replace-in-go-mod-to-point-to-your-local-module/
