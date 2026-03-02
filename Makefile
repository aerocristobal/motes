.PHONY: build test vet install clean

BINARY := mote
BUILD_DIR := .

build:
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/mote

test:
	go test ./...

vet:
	go vet ./...

install: build
	cp $(BUILD_DIR)/$(BINARY) ~/.local/bin/$(BINARY)

clean:
	rm -f $(BUILD_DIR)/$(BINARY)
