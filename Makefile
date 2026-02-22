.PHONY: build ui sync-ui clean run release

# Build UI + sync + compile Go binary (本机)
build: ui sync-ui
	go build -o bin/aipanel ./cmd/aipanel/

# Build Vue frontend
ui:
	cd ui && npm run build

# Sync ui/dist → cmd/aipanel/ui_dist (required for go:embed)
sync-ui:
	rm -rf cmd/aipanel/ui_dist
	cp -r ui/dist cmd/aipanel/ui_dist

# Build Go only (assumes ui_dist is already synced)
go-only:
	go build -o bin/aipanel ./cmd/aipanel/

# Run server
run:
	AIPANEL_CONFIG=aipanel.json ./bin/aipanel

# 交叉编译所有平台（需先 make ui sync-ui）
release: sync-ui
	mkdir -p bin/release
	GOOS=linux  GOARCH=amd64  go build -o bin/release/aipanel-linux-amd64   ./cmd/aipanel/
	GOOS=linux  GOARCH=arm64  go build -o bin/release/aipanel-linux-arm64   ./cmd/aipanel/
	GOOS=darwin GOARCH=arm64  go build -o bin/release/aipanel-darwin-arm64  ./cmd/aipanel/
	GOOS=darwin GOARCH=amd64  go build -o bin/release/aipanel-darwin-amd64  ./cmd/aipanel/
	ls -lh bin/release/

clean:
	rm -rf cmd/aipanel/ui_dist ui/dist bin/aipanel bin/release
