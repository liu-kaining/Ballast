# Ballast — local development orchestration
# v0.1 scope: control plane skeleton + harness-agent + sandbox image + web

.PHONY: all backend frontend harness sandbox-image dev test e2e-test clean fmt vet

SHELL := /bin/bash
ROOT := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))

GO := go

all: backend harness sandbox-image frontend

# ---------- Backend (Go control plane) ----------
backend:
	cd server && $(GO) build -o ../bin/ballast-server ./cmd/ballast-server

backend-run: backend
	./bin/ballast-server -config server/configs/ballast.yaml

backend-tidy:
	cd server && $(GO) mod tidy

backend-test:
	cd server && $(GO) test ./...

# ---------- Harness-Agent ----------
harness:
	cd harness-agent && $(GO) build -o ../bin/harness-agent ./cmd/harness-agent

harness-tidy:
	cd harness-agent && $(GO) mod tidy

harness-test:
	cd harness-agent && $(GO) test ./...

# ---------- Sandbox image ----------
sandbox-image:
	docker build -f sandbox-image/Dockerfile -t ballast-runner-base:dev .

# ---------- Frontend (Next.js 15) ----------
frontend:
	cd web && npm install && npm run build

frontend-dev:
	cd web && npm install && npm run dev

# ---------- Combined ----------
dev: backend frontend harness
	@echo "Run 'docker compose up' for full stack, or 'make backend-run' / 'make frontend-dev' separately"

test: backend-test harness-test
	cd web && npm test

e2e-test:
	./scripts/e2e-smoke.sh

fmt:
	cd server && $(GO) fmt ./...
	cd harness-agent && $(GO) fmt ./...

vet:
	cd server && $(GO) vet ./...
	cd harness-agent && $(GO) vet ./...

clean:
	rm -rf bin
