.PHONY: build run test clean

build:
	go build -o bin/dmsg ./cmd/dmsg

run: build
	./bin/dmsg start

test:
	go test ./internal/...

clean:
	rm -rf bin/
