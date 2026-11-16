BINARY := beadle
VERSION := $(shell git describe --tags --always --dirty)
LDFLAGS ?= -X github.com/andrew-bierman/beadle/internal/cmd.version=$(VERSION)

.PHONY: build install test vet cross clean

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

install: build
	ln -sf "$(CURDIR)/$(BINARY)" "$(HOME)/.local/bin/$(BINARY)"
	@echo "linked: ~/.local/bin/$(BINARY)"

test:
	go test ./...

vet:
	go vet ./...

# Release artifacts: beadle-<os>-<arch> tarballs + checksums in dist/.
cross:
	mkdir -p dist
	@for os in linux darwin; do \
	  for arch in amd64 arm64; do \
	    CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-$$os-$$arch .; \
	  done; \
	done
	cd dist && tar czf $(BINARY)-linux-amd64.tar.gz $(BINARY)-linux-amd64 && \
	  tar czf $(BINARY)-linux-arm64.tar.gz $(BINARY)-linux-arm64 && \
	  tar czf $(BINARY)-darwin-amd64.tar.gz $(BINARY)-darwin-amd64 && \
	  tar czf $(BINARY)-darwin-arm64.tar.gz $(BINARY)-darwin-arm64 && \
	  rm -f $(BINARY)-linux-amd64 $(BINARY)-linux-arm64 $(BINARY)-darwin-amd64 $(BINARY)-darwin-arm64 && \
	  shasum -a 256 * > SHA256SUMS

clean:
	rm -rf dist $(BINARY)
