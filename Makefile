.PHONY: build ui sync-ui clean run release

# 版本号：优先用 git tag，否则用 commit hash
VERSION := $(shell git describe --tags --abbrev=0 2>/dev/null || git rev-parse --short HEAD 2>/dev/null || echo "dev")
LDFLAGS := -X main.Version=$(VERSION)

# Build UI + sync + compile Go binary (本机)
build: ui sync-ui
	go build -ldflags "$(LDFLAGS)" -o bin/aipanel ./cmd/aipanel/

# Build Vue frontend
ui:
	cd ui && npm run build

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

# 交叉编译所有平台（需先 make ui sync-ui）
release: sync-ui
	mkdir -p bin/release
	GOOS=linux  GOARCH=amd64  go build -ldflags "$(LDFLAGS)" -o bin/release/zyhive-linux-amd64   ./cmd/aipanel/
	GOOS=linux  GOARCH=arm64  go build -ldflags "$(LDFLAGS)" -o bin/release/zyhive-linux-arm64   ./cmd/aipanel/
	GOOS=darwin GOARCH=arm64  go build -ldflags "$(LDFLAGS)" -o bin/release/zyhive-darwin-arm64  ./cmd/aipanel/
	GOOS=darwin GOARCH=amd64  go build -ldflags "$(LDFLAGS)" -o bin/release/zyhive-darwin-amd64  ./cmd/aipanel/
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/release/zyhive-windows-amd64.exe ./cmd/aipanel/
	GOOS=windows GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/release/zyhive-windows-arm64.exe ./cmd/aipanel/
	ls -lh bin/release/

clean:
	rm -rf cmd/aipanel/ui_dist ui/dist bin/aipanel bin/release
