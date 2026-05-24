.PHONY: build test vet install install-hooks clean

BINARY := mote
BUILD_DIR := .

build:
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/mote

test:
	go test ./...

vet:
	go vet ./...

install-hooks:
	@if [ -d .git ]; then \
		git config core.hooksPath .githooks; \
		echo "core.hooksPath set to .githooks"; \
	else \
		echo "Not a git checkout; skipping hooks setup."; \
	fi

install: build install-hooks
	cp $(BUILD_DIR)/$(BINARY) ~/.local/bin/$(BINARY)

clean:
	rm -f $(BUILD_DIR)/$(BINARY)
