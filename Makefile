.PHONY: all build install clean test lint fmt

# Version information
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.0.1-dev")
LDFLAGS := -ldflags "-X github.com/pulumi/pulumi-language-hcl/pkg/version.Version=$(VERSION)"

# Build output directory
BIN_DIR := bin

# Binary names
LANGUAGE_HOST := pulumi-language-hcl
CONVERTER := pulumi-converter-hcl

_ := $(shell mkdir -p $(BIN_DIR))
_ := $(shell go build -o $(BIN_DIR)/helpmakego github.com/iwahbe/helpmakego)

all: build

build: $(BIN_DIR)/$(LANGUAGE_HOST) $(BIN_DIR)/$(CONVERTER)

$(BIN_DIR)/$(LANGUAGE_HOST): $(shell $(BIN_DIR)/helpmakego ./cmd/pulumi-language-hcl)
	@mkdir -p $(BIN_DIR)
	go build $(LDFLAGS) -o $@ ./cmd/pulumi-language-hcl

$(BIN_DIR)/$(CONVERTER): $(shell $(BIN_DIR)/helpmakego ./cmd/pulumi-converter-hcl)
	@mkdir -p $(BIN_DIR)
	go build $(LDFLAGS) -o $@ ./cmd/pulumi-converter-hcl

install: build
	cp $(BIN_DIR)/$(LANGUAGE_HOST) $(GOPATH)/bin/
	cp $(BIN_DIR)/$(CONVERTER) $(GOPATH)/bin/

clean:
	rm -rf $(BIN_DIR)
	go clean

test:
	go test -v -race ./...

test_cover:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

lint:
	golangci-lint run ./...

fmt:
	go fmt ./...
	gofumpt -w .

tidy:
	go mod tidy

# Development helpers
dev: ~/.pulumi/bin/$(LANGUAGE_HOST) ~/.pulumi/bin/$(CONVERTER)

~/.pulumi/bin/$(LANGUAGE_HOST): $(BIN_DIR)/$(LANGUAGE_HOST)
	cp $< $@

~/.pulumi/bin/$(CONVERTER): $(BIN_DIR)/$(CONVERTER)
	cp $< $@

.PHONY: generate
generate:
	go generate ./...
