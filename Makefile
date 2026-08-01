.PHONY: lint test proto

lint:
	@# golangci-lint must be built with a toolchain >= the repo's go directive or it refuses to load.
	GOTOOLCHAIN=go$$(go list -m -f '{{.GoVersion}}') go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run --timeout 2m ./...

test:
	go test ./...

proto:
	protoc --go_out=. --go_opt=paths=source_relative \
	       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
	       app/rpc/app.proto
