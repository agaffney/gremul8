BINARY=gremul8

.PHONY: all clean

all: $(BINARY)

clean:
	rm -f $(BINARY)

$(BINARY): $(shell find -name '*.go')
	go build -o $(BINARY) cmd/$(BINARY)

.PHONY: test

test:
	go test -v ./...
