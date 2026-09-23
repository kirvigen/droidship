.PHONY: build test lint smoke install
build:
	go build -o droidship ./cmd/droidship
install:
	go install ./cmd/droidship
test:
	go test -count=1 ./...
lint:
	test -z "$$(gofmt -l .)" && go vet ./...
smoke: build
	./scripts/smoke.sh
