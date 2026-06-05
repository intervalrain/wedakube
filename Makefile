APP_NAME := kube
VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE     := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS  := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: build install uninstall clean cross

## Build the binary for the current platform.
build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(APP_NAME) .

## Install to /usr/local/bin (codesign avoids Gatekeeper kill on macOS).
install: build
	sudo cp $(APP_NAME) /usr/local/bin/$(APP_NAME)
	sudo codesign -f -s - /usr/local/bin/$(APP_NAME) 2>/dev/null || true
	@echo "✓ installed $(APP_NAME) $(VERSION) to /usr/local/bin"

## Remove the installed binary.
uninstall:
	sudo rm -f /usr/local/bin/$(APP_NAME)
	@echo "✓ removed /usr/local/bin/$(APP_NAME)"

## Cross-compile for the common platforms.
cross:
	GOOS=linux  GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/$(APP_NAME)_linux_amd64/$(APP_NAME) .
	GOOS=linux  GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/$(APP_NAME)_linux_arm64/$(APP_NAME) .
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/$(APP_NAME)_darwin_amd64/$(APP_NAME) .
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/$(APP_NAME)_darwin_arm64/$(APP_NAME) .

## Clean build artifacts.
clean:
	rm -f $(APP_NAME)
	rm -rf dist/
