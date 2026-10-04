.PHONY: build test frontend
frontend:
	cd frontend && npm ci && npm run build
build: frontend
	go build -o aimangebot .
test:
	go test ./...
	go test -race ./internal/platform
	go vet ./...
	cd frontend && npm run typecheck
