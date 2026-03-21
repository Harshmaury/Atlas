# Makefile — Atlas
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -X main.atlasVersion=$(VERSION)
BINDIR  := $(HOME)/bin

.PHONY: all build clean

all: build

build:
	@echo "  → atlas $(VERSION)"
	@CGO_ENABLED=1 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/atlas ./cmd/atlas/

clean:
	@rm -f $(BINDIR)/atlas
