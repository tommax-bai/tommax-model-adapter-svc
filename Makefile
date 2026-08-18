.PHONY: build run lint test
build:
	go build -o bin/model-adapter ./cmd/server
run: build
	./bin/model-adapter -config configs/config.yaml
lint:
	golangci-lint run ./...
test:
	go test -race ./...
