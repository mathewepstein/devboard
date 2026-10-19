VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -ldflags "-X main.version=$(VERSION)"
PKG      := ./cmd/devboard
ARGS ?=

.PHONY: all build install reload run test vet fmt check clean

all: build

build:
	go build $(LDFLAGS) -o devboard $(PKG)

install:
	go install $(LDFLAGS) $(PKG)

reload: install
	launchctl kickstart -k gui/$$(id -u)/local.devboard
	@$$(go env GOPATH)/bin/devboard agent status --wait 20s

run:
	go run $(PKG) $(ARGS)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

check: vet test

clean:
	rm -f devboard
