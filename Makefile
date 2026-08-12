
.PHONY: deps-update build-bin unittest

ARG ?=

deps-update:
	go mod tidy

build-bin:
	mkdir -p ./bin
	go build -o ./bin/bond ./bond/

unittest:
	docker run --rm --privileged \
		-v $(CURDIR):/workspace \
		-v $(CURDIR)/.gomodcache:/go/pkg/mod \
		-w /workspace \
		golang:1.26 \
		go test -race -covermode=atomic -coverprofile=profile.out ./... $(ARG)
