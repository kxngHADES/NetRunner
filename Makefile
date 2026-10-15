.PHONY: build test vet race ci

build:
	go build -o bin/netrunner ./cmd/netrunner

test:
	go test ./...

vet:
	go vet ./...

race:
	go test -race ./...

ci: vet race
