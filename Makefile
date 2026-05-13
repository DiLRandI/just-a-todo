BINARY := todo
MAIN := ./cmd/todo

.PHONY: build install test fmt tidy run today clean

build:
	go build -o bin/$(BINARY) $(MAIN)

install:
	go install $(MAIN)

test:
	go test ./...

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
