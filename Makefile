SHELL := /usr/bin/env bash

.PHONY: help server-test server-build server-run client-install client-test client-dev loadtest

help:
	@echo "Common targets:"
	@echo "  make server-test     Run Go server tests (if server/ exists)"
	@echo "  make server-build    Build Go server binary (if server/ exists)"
	@echo "  make client-install  Install client dependencies (if client/ exists)"
	@echo "  make client-test     Run client tests (if client/ exists)"
	@echo "  make client-dev      Start client dev server (if client/ exists)"

server-test:
	@if [ -d server ]; then \
		echo "Running server tests..."; \
		( cd server && go test ./... ); \
	else \
		echo "Skipping: server/ not found."; \
	fi

server-build:
	@if [ -d server ]; then \
		echo "Building server..."; \
		( cd server && go build ./... ); \
	else \
		echo "Skipping: server/ not found."; \
	fi

client-install:
	@if [ -d client ]; then \
		echo "Installing client dependencies..."; \
		( cd client && npm install ); \
	else \
		echo "Skipping: client/ not found."; \
	fi

client-test:
	@if [ -d client ]; then \
		echo "Running client tests..."; \
		( cd client && npm test ); \
	else \
		echo "Skipping: client/ not found."; \
	fi

client-dev:
	@if [ -d client ]; then \
		echo "Starting client dev server..."; \
		( cd client && npm run dev ); \
	else \
		echo "Skipping: client/ not found."; \
	fi

server-run:
	@( cd server && go run ./cmd/mmothello )

loadtest:
	@node scripts/loadtest/bots.mjs --base http://localhost:8080 --clients 50 --duration 30
