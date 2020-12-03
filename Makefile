# Go parameters
ROOT_DIR:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
GOCMD=go
GOBUILD=$(GOCMD) build --mod=vendor
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
BINARY_NAME=./bin/bond

# the following vars are overidable with ENV vars
# example:
#  before running make set vars like so
#  export DOCKER_REPO=my_repo
IMAGE_REPO?=jdambly
IMAGE_VERSION?=v0.1
IMAGE_NAME?=bond-cni

help: ## Show available Makefile targets
	@awk '\
	BEGIN { \
		FS = ":.*##"; \
		printf "Usage: make \033[36m<target>\033[0m\n\n" \
	} \
	/^[a-zA-Z_-]+:.*?##/ { \
		printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 \
	} \
	/^##@/ { \
		printf "\n\033[1m%s\033[0m\n", substr($$0, 5) \
	}' $(MAKEFILE_LIST)

all: test build ## test and build
build: ## build go binary
	$(GOBUILD) -o $(BINARY_NAME) -v ./bond
test: ## run tests
	$(GOTEST) -v ./...
clean: ## remove binaries
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_UNIX)
deps: ##  go mod <deps>
	go mod tidy && \
	go mod vendor

docker-build:
	docker build -t $(IMAGE_REPO)/$(IMAGE_NAME):($IMAGE_VERSION) .

gofmt:
	@echo "Running gofmt"
	gofmt -s -l `find . -path ./vendor -prune -o -type f -name '*.go' -print`
