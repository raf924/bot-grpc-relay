package api

//go:generate protoc -I=definitions -I=$GOPATH/src -I=$GOPATH/src/github.com/raf924/bot/api/definitions --go_out=grpc --go_opt=paths=source_relative --go-grpc_out=grpc --go-grpc_opt=paths=source_relative definitions/connector.proto
