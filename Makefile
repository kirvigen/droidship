.PHONY: build test lint smoke install
build:
	go build -o droidship ./cmd/droidship
install:
	go install ./cmd/droidship
lint:
	test -z "$$(gofmt -l .)" && go vet ./...
test: lint
	go test -count=1 ./...
smoke: build
	./scripts/smoke.sh
