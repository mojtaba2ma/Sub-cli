# sb-ech-gateway - build helpers
BINARY  := sb-ech-gateway
VERSION := 2.0.0
LDFLAGS := -s -w
GOFLAGS := -trimpath

.PHONY: build run selftest test vet fmt linux-amd64 linux-arm64 samples check clean

build:
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BINARY) .

run: build
	./$(BINARY) --config config.json

selftest: build
	./$(BINARY) --selftest

# parse + build smoke test (no network needed beyond the optional ECH fetch)
test: build
	./$(BINARY) --from-file tests/links.txt --ech-mode dynamic --out /tmp/sb-profile.json
	@command -v sing-box >/dev/null 2>&1 && sing-box check -c /tmp/sb-profile.json || echo "tip: install sing-box to validate the output"

samples: selftest
	@echo "samples/profile-{dynamic,static}.json and samples/profile-mihomo.yaml refreshed"

vet:
	go vet ./...

fmt:
	gofmt -w .

# static builds (no libc dependency) - works on any Linux, including Alpine
linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o dist/sb-ech-gateway-linux-amd64 .

linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o dist/sb-ech-gateway-linux-arm64 .

# cross-compile for release assets + checksums
release: linux-amd64 linux-arm64
	cd dist && sha256sum sb-ech-gateway-linux-* > SHA256SUMS

check:
	@bash -n install.sh && echo "install.sh: syntax OK"
	@bash -n scripts/install-remote.sh && echo "scripts/install-remote.sh: syntax OK"
	@bash -n scripts/update.sh && echo "scripts/update.sh: syntax OK"
	@bash -n scripts/test-on-server.sh && echo "scripts/test-on-server.sh: syntax OK"

clean:
	rm -rf dist $(BINARY) samples/*.tmp
