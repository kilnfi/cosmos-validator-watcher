BUILD_FOLDER  ?= build
BINARY_NAME   ?= cosmos-validator-watcher
PACKAGES      ?= $(shell go list ./... | egrep -v "testutils" )
VERSION       ?= $(shell git describe --tags)

.PHONY: build
build:
	@go build -o $(BUILD_FOLDER)/$(BINARY_NAME) -v -ldflags="-X 'main.Version=$(VERSION)'"

.PHONY: test
test:
	@go test -v $(PACKAGES)

.PHONY: version
version:
	@echo $(VERSION)
