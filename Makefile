VERSION=$(shell head -n 1 version)
GIT_REVISION=$(shell git log --pretty=format:'%h' -n 1)

TOP_DIR=$(dir $(realpath $(firstword $(MAKEFILE_LIST))))
PKG_SRC=$(shell find $(TOP_DIR)pkg -type f -name '*.go')
ITZO_SRC=$(shell find $(TOP_DIR)cmd/itzo -type f -name '*.go')
GO_BIN?="go"

LDFLAGS=-ldflags "-X github.com/elotl/itzo/pkg/util.VERSION=$(VERSION) -X github.com/elotl/itzo/pkg/util.GIT_REVISION=$(GIT_REVISION)"

all: itzo itzo-darwin

itzo: $(PKG_SRC) $(ITZO_SRC) go.mod go.sum
	$(GO_BIN) build $(LDFLAGS) -o $(TOP_DIR)$@ $(TOP_DIR)cmd/itzo

itzo-darwin: $(PKG_SRC) $(ITZO_SRC) go.mod go.sum
	GOOS=darwin GOARCH=amd64 $(GO_BIN) build $(LDFLAGS) -o $(TOP_DIR)itzo-darwin $(TOP_DIR)cmd/itzo

clean:
	rm -f itzo

.PHONY: all clean
