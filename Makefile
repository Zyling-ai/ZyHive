.PHONY: build ui test check sync-ui clean run release release-dry-run release-e2e

# 版本号：优先用 git tag，否则用 commit hash
VERSION := $(shell git describe --tags --abbrev=0 2>/dev/null || git rev-parse --short HEAD 2>/dev/null || echo "dev")
LDFLAGS := -X main.Version=$(VERSION)

# Build UI + sync + compile Go binary (本机)
build: ui sync-ui
	go build -ldflags "$(LDFLAGS)" -o bin/aipanel ./cmd/aipanel/

# Build Vue frontend
ui:
	cd ui && npm run build

# Run backend and frontend tests
test:
	go test ./... -count=1 -timeout=5m
	cd ui && npm test

# Local equivalent of the required CI quality gate
check:
	go vet ./...
	go test ./... -count=1 -timeout=5m
	go build ./...
	cd ui && npm ci && npm test && npm run build
	$(MAKE) sync-ui
	git diff --no-index --exit-code -- ui/dist cmd/aipanel/ui_dist

# Sync ui/dist → cmd/aipanel/ui_dist (required for go:embed)
sync-ui:
	rm -rf cmd/aipanel/ui_dist
	cp -r ui/dist cmd/aipanel/ui_dist

# Build Go only (assumes ui_dist is already synced)
go-only:
	go build -ldflags "$(LDFLAGS)" -o bin/aipanel ./cmd/aipanel/

# Run server
run:
	AIPANEL_CONFIG=aipanel.json ./bin/aipanel

# Validate, build four official platforms, and publish a GitHub Release.
# Usage: make release RELEASE_VERSION=26.8.1v1
release:
	@test -n "$(RELEASE_VERSION)" || (echo "请提供 RELEASE_VERSION=YY.M.DvN" >&2; exit 2)
	./scripts/release.sh "$(RELEASE_VERSION)"

# Same quality and build flow without creating a GitHub Release.
release-dry-run:
	@test -n "$(RELEASE_VERSION)" || (echo "请提供 RELEASE_VERSION=YY.M.DvN" >&2; exit 2)
	./scripts/release.sh "$(RELEASE_VERSION)" --dry-run

# Build the current host artifact and verify clean install, basic API and update.
release-e2e:
	@test -n "$(RELEASE_VERSION)" || (echo "请提供 RELEASE_VERSION=YY.M.DvN" >&2; exit 2)
	rm -rf /tmp/zyhive-release-e2e-make
	mkdir -p /tmp/zyhive-release-e2e-make
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.Version=$(RELEASE_VERSION)" \
		-o /tmp/zyhive-release-e2e-make/zyhive-$(shell go env GOOS)-$(shell go env GOARCH) ./cmd/aipanel/
	scripts/test/release-e2e/run.sh local "$(RELEASE_VERSION)" /tmp/zyhive-release-e2e-make

clean:
	rm -rf cmd/aipanel/ui_dist ui/dist bin/aipanel bin/release
