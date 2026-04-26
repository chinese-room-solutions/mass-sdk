.PHONY: lint test proto

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run --timeout 2m ./...

test:
	go test ./...

proto:
	protoc --go_out=. --go_opt=paths=source_relative \
	       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
	       app/rpc/app.proto
