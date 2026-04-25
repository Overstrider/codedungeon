VERSION ?= v0.8.0
SRC     := ./src/codedungeon
RELEASE := ./release
BIN     := $(RELEASE)/bin

LDFLAGS_BASE := -s -w -X main.Version=$(VERSION)
LDFLAGS_CODEX := $(LDFLAGS_BASE) -X github.com/loldinis/codedungeon/internal/provider.DefaultProvider=codex
LDFLAGS_CLAUDE := $(LDFLAGS_BASE) -X github.com/loldinis/codedungeon/internal/provider.DefaultProvider=claude
LDFLAGS_GENERIC := $(LDFLAGS_BASE)

.PHONY: all build build-linux build-windows build-darwin-amd64 build-darwin-arm64 test clean release install help

help:
	@echo "codedungeon build targets:"
	@echo "  make build           # linux-amd64 only"
	@echo "  make release         # all 4 platforms + re-sync skill into release/"
	@echo "  make test            # go test ./..."
	@echo "  make install         # build linux + run release/install.sh"
	@echo "  make clean           # rm release/bin/*"
	@echo ""
	@echo "VERSION=$(VERSION)  (override: VERSION=v0.9.0 make release)"

all: release

build: build-linux

build-linux:
	cd $(SRC) && GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS_CODEX)" -o ../../$(BIN)/codedungeon-codex .
	cd $(SRC) && GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS_CLAUDE)" -o ../../$(BIN)/codedungeon-claude .
	cd $(SRC) && GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS_GENERIC)" -o ../../$(BIN)/codedungeon .

build-windows:
	cd $(SRC) && GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS_CODEX)" -o ../../$(BIN)/codedungeon-codex.exe .
	cd $(SRC) && GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS_CLAUDE)" -o ../../$(BIN)/codedungeon-claude.exe .
	cd $(SRC) && GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS_GENERIC)" -o ../../$(BIN)/codedungeon.exe .

build-darwin-amd64:
	cd $(SRC) && GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS_CODEX)" -o ../../$(BIN)/codedungeon-codex-darwin-amd64 .
	cd $(SRC) && GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS_CLAUDE)" -o ../../$(BIN)/codedungeon-claude-darwin-amd64 .
	cd $(SRC) && GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS_GENERIC)" -o ../../$(BIN)/codedungeon-darwin-amd64 .

build-darwin-arm64:
	cd $(SRC) && GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS_CODEX)" -o ../../$(BIN)/codedungeon-codex-darwin-arm64 .
	cd $(SRC) && GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS_CLAUDE)" -o ../../$(BIN)/codedungeon-claude-darwin-arm64 .
	cd $(SRC) && GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS_GENERIC)" -o ../../$(BIN)/codedungeon-darwin-arm64 .

release: build-linux build-windows build-darwin-amd64 build-darwin-arm64
	@mkdir -p $(RELEASE)/skills/grimoire-cli
	@cp $(SRC)/internal/prompts/files/skills/grimoire-cli/SKILL.md $(RELEASE)/skills/grimoire-cli/SKILL.md
	@echo "[release] bin + skill synced at $(RELEASE)/"
	@ls -la $(BIN)/

test:
	cd $(SRC) && go test ./...

install: build-linux
	bash $(RELEASE)/install.sh

clean:
	rm -f $(BIN)/*
