.PHONY: help build frontend-build backend-build dev start init-admin seed-demo test test-e2e

help:
	@echo "build       Build frontend and independent Go server"
	@echo "dev         Run Go API and Vite (localhost:3000)"
	@echo "start       Run built application (localhost:8080)"
	@echo "init-admin  Initialize an administrator; no default password"
	@echo "seed-demo   Create clearly labelled synthetic teacher/student records"
	@echo "test        Run backend tests, frontend build, and lint"
	@echo "test-e2e    Run black-box acceptance with controlled model HTTP"

build: frontend-build backend-build

frontend-build:
	npm run build

backend-build:
	mkdir -p bin
	cd backend && go build -o ../bin/zhiwei ./cmd/server

dev: backend-build
	./scripts/dev.sh

start:
	./scripts/server.sh serve

init-admin: backend-build
	./scripts/server.sh init-admin

seed-demo: backend-build
	./scripts/server.sh seed-demo

test:
	cd backend && go test -race ./...
	npm run build
	npm run lint
	npm run test:ui

test-e2e: build
	python3 scripts/e2e.py
