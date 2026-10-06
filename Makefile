.PHONY: fmt check

fmt:
	gofmt -w .

check:
	@test -z "$$(gofmt -l .)" || (echo "gofmt found unformatted files:"; gofmt -l .; exit 1)
	go vet ./...
	CGO_ENABLED=0 go build ./...
	CGO_ENABLED=0 go test ./...
