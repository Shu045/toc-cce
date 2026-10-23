.PHONY: backend frontend test
backend:
	cd backend && go run ./cmd/server
frontend:
	cd frontend && npm install && npm run dev
test:
	cd backend && go vet ./... && go test ./...
