.PHONY: build run test race vet clean

build:
	go build ./cmd/server/

run:
	go run cmd/server/main.go

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

clean:
	rm -f feature-flag-engine

default: build
