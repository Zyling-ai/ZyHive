.PHONY: build ui sync-ui clean run

# Build UI + sync + compile Go binary
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

clean:
	rm -rf cmd/aipanel/ui_dist ui/dist bin/aipanel
