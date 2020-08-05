GIT_VERSION=$(shell git describe --dirty)
CURRENT_DATE=$(shell date --universal --iso-8601=seconds)

TOP_DIR=$(dir $(realpath $(firstword $(MAKEFILE_LIST))))
PKG_SRC=$(shell find $(TOP_DIR)pkg -type f -name '*.go')
VENDOR_SRC=$(shell find $(TOP_DIR)vendor -type f -name '*.go')
ITZO_SRC=$(shell find $(TOP_DIR)cmd/itzo -type f -name '*.go')

LDFLAGS=-ldflags "-X github.com/elotl/itzo/pkg/util.BuildVersion=$(GIT_VERSION) -X github.com/elotl/itzo/pkg/util.BuildDate=$(CURRENT_DATE)"

all: itzo

itzo: $(PKG_SRC) $(ITZO_SRC) $(VENDOR_SRC)
	cd cmd/itzo && go build $(LDFLAGS) -o $(TOP_DIR)itzo

clean:
	rm -f itzo

.PHONY: all clean
