deps-update:
	go mod tidy && \
	go mod vendor

gofmt:
	@echo "Running gofmt"
	gofmt -s -l `find . -path ./vendor -prune -o -type f -name '*.go' -print`

build-bin:
	./build.sh

test: build-bin # Tests need sudo due to network interfaces creation
	sudo -E bash -c "umask 0; PATH=${GOPATH}/bin:$(pwd)/bin:${PATH} go test ./bond/"
