BINARY := todo
MAIN := ./cmd/todo

.PHONY: build install test vet check fmt tidy run today clean

build:
	go build -o bin/$(BINARY) $(MAIN)

install:
	go install $(MAIN)

test:
	go test ./...

vet:
	go vet ./...

check:
	test -z "$$(gofmt -l cmd internal)"
	go test ./...
	go vet ./...

fmt:
	gofmt -w cmd internal

tidy:
	go mod tidy

run:
	go run $(MAIN)

today:
	go run $(MAIN) today

clean:
	rm -rf bin
