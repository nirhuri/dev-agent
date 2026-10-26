.PHONY: build test vet fmt doctor

build:
	go build -o bin/dev-agent ./cmd/dev-agent

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

doctor: build
	./bin/dev-agent doctor
