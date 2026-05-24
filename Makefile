.PHONY: fmt build test tidy fix

fmt:
	gofmt -w *.go

build:
	go build -ldflags="-w -s" -o goenv ./...

test:
	go test -v ./...

tidy:
	go mod tidy

fix:
	go fix ./...
