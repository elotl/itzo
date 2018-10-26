BUILD_DATE=$(shell date -u +.%Y%m%d.%H%M%S)

TOP_DIR=$(dir $(realpath $(firstword $(MAKEFILE_LIST))))
PKG_SRC=$(shell find $(TOP_DIR)pkg -type f -name '*.go')
ITZO_SRC=$(shell find $(TOP_DIR)cmd/itzo -type f -name '*.go')

all: itzo

itzo: $(PKG_SRC) $(MILPA_SRC)
	cd cmd/itzo && go build -ldflags "-X main.buildDate=$(BUILD_DATE)" -o $(TOP_DIR)itzo

clean:
	rm -f itzo

.PHONY: all clean
