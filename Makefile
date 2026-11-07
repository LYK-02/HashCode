.PHONY: all build test clean demo-ready demo

GO ?= go
BINARY := kvdb

all: build

build:
	CGO_ENABLED=0 GOPROXY=off $(GO) build -trimpath -ldflags="-s -w" -o $(BINARY) ./cmd/kvdb

test:
	GOPROXY=off $(GO) test -v -count=1 ./...

demo: build
	./demo.sh

clean:
	rm -f $(BINARY)
	rm -f *.db *.lock *.wal *.tmp *.compact.tmp
	rm -rf testdata/tmp artifacts

demo-ready: build test
	@echo "Demo ready: build and test passed offline."
