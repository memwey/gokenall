GO      ?= go
MODULES := . ./tools/gendata

.PHONY: all
all: test

.PHONY: test
test:
	$(GO) test -race ./... ./tools/gendata/...

.PHONY: vet
vet:
	$(GO) vet ./... ./tools/gendata/...

.PHONY: lint
lint: vet
	$(GO) run honnef.co/go/tools/cmd/staticcheck@latest ./... ./tools/gendata/...

.PHONY: fmt
fmt:
	$(GO) fmt ./... ./tools/gendata/...

# Rebuild data/kenall.bin from Japan Post. Needs network access.
.PHONY: data
data:
	$(GO) generate ./...

# Same, but reuse anything already in tmp/ so repeated runs skip the download.
.PHONY: data-cached
data-cached:
	$(GO) run ./tools/gendata -out data/kenall.bin -cache tmp -v

.PHONY: tidy
tidy:
	$(GO) mod tidy
	cd tools/gendata && GOWORK=off $(GO) mod tidy
	$(GO) work sync

.PHONY: bench
bench:
	$(GO) test -run '^$$' -bench . -benchmem ./...

.PHONY: clean
clean:
	$(GO) clean
	rm -rf tmp
