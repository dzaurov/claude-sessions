.PHONY: build install test lint clean

BIN := bin/ccs
INSTALL_DIR := $(HOME)/.local/bin

build:
	mkdir -p bin
	go build -o $(BIN) ./cmd/ccs

install: build
	mkdir -p $(INSTALL_DIR)
	ln -sfn $(CURDIR)/$(BIN) $(INSTALL_DIR)/ccs
	@echo "Installed: $(INSTALL_DIR)/ccs -> $(CURDIR)/$(BIN)"
	@case ":$$PATH:" in *":$(INSTALL_DIR):"*) ;; *) echo "WARNING: $(INSTALL_DIR) not in PATH. Add to ~/.zshrc: export PATH=\"$$HOME/.local/bin:$$PATH\"" ;; esac

test:
	go test ./...

lint:
	go vet ./...
	gofmt -l . | (! grep .)

clean:
	rm -rf bin
