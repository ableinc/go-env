.PHONY: fmt build test tidy

fmt:
	gofmt -w *.go

build:
	go build -ldflags="-w -s" -o goenv ./...

test:
	go test ./...

tidy:
	go mod tidy
