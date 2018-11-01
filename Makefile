VERSION=$(shell head -n 1 version)
GIT_REVISION=$(shell git log --pretty=format:'%h' -n 1)

TOP_DIR=$(dir $(realpath $(firstword $(MAKEFILE_LIST))))
PKG_SRC=$(shell find $(TOP_DIR)pkg -type f -name '*.go')
VENDOR_SRC=$(shell find $(TOP_DIR)vendor -type f -name '*.go')
ITZO_SRC=$(shell find $(TOP_DIR)cmd/itzo -type f -name '*.go')

LDFLAGS=-ldflags "-X github.com/elotl/itzo/pkg/util.VERSION=$(VERSION) -X github.com/elotl/itzo/pkg/util.GIT_REVISION=$(GIT_REVISION)"

all: itzo

itzo: $(PKG_SRC) $(ITZO_SRC) $(VENDOR_SRC)
	cd cmd/itzo && go build $(LDFLAGS) -o $(TOP_DIR)itzo

clean:
	rm -f itzo

.PHONY: all clean