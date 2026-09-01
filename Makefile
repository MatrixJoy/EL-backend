.PHONY: fmt generate test integration-test vet check run-api run-worker

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

test:
	go test ./...

generate:
	sqlc generate

integration-test:
	test -n "$$VOA_INTEGRATION_DATABASE_URL"
	go test ./internal/identity ./internal/httpapi -count=1

vet:
	go vet ./...

check: generate test vet

run-api:
	go run ./cmd/api

run-worker:
	go run ./cmd/worker
