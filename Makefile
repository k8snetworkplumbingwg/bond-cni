
# Go environment:
GOPATH=$(CURDIR)/.gopath
GOBIN=$(CURDIR)/bin

export GOPATH
export GOBIN

# Go tools:
GOCOVXML = $(GOBIN)/gocov-xml
GOCOVMERGE = $(GOBIN)/gocovmerge
GOCOV = $(GOBIN)/gocov
GCOV2LCOV = $(GOBIN)/gcov2lcov

# Package info
PACKAGE=bond-cni
ORG_PATH=github.com/k8snetworkplumbingwg

# Build info
REPO_PATH=$(ORG_PATH)/$(PACKAGE)
BASE=$(GOPATH)/src/$(REPO_PATH)
PKGS = $(or $(PKG),$(shell cd $(BASE) && env GOPATH=$(GOPATH) go list ./... | grep -v "^$(PACKAGE)/vendor/"))

# Test artifacts and settings:
TESTPKGS = $(shell env GOPATH=$(GOPATH) go list -f '{{ if or .TestGoFiles .XTestGoFiles }}{{ .ImportPath }}{{ end }}' $(PKGS))
COVERAGE_MODE = atomic
COVERAGE_DIR = $(CURDIR)/test/coverage
COVERAGE_PROFILE = $(COVERAGE_DIR)/profile.out
COVERAGE_XML = $(COVERAGE_DIR)/coverage.xml
COVERAGE_HTML = $(COVERAGE_DIR)/index.html

.PHONY: $(BASE)
$(BASE): ; $(info  Setting GOPATH...)
	@mkdir -p $(dir $@)
	@ln -sf $(CURDIR) $@


deps-update:
	go mod tidy

gofmt:
	@echo "Running gofmt"
	gofmt -s -l `find . -path ./vendor -prune -o -type f -name '*.go' -print`

build-bin:
	./build.sh

test: build-bin # Tests need sudo due to network interfaces creation
	sudo -E bash -c "umask 0; PATH=${GOPATH}/bin:$(pwd)/bin:${PATH} go test -race ./bond/"


# tool that takes the results from multiple go test -coverprofile runs and merges them into one profile
$(GOCOVMERGE): | $(BASE) ; $(info  building gocovmerge...)
	go install github.com/wadey/gocovmerge@latest

# Convert golang test coverage to lcov format (which can be uploaded to coveralls).
$(GCOV2LCOV): | $(BASE) ; $(info  building gcov2lcov...)
	go install github.com/jandelgado/gcov2lcov@latest

# A tool to generate Go coverage in XML report
$(GOCOVXML): | $(BASE) ; $(info  building gocov-xml...)
	go install github.com/AlekSi/gocov-xml@latest

#Coverage reporting tool
$(GOCOV): | $(BASE) ; $(info  building gocov...)
	go install github.com/axw/gocov/gocov@v1.1.0


.PHONY: test-coverage test-coverage-tools
test-coverage-tools: | $(GOCOVMERGE) $(GOCOV) $(GOCOVXML) $(GCOV2LCOV)
test-coverage: COVERAGE_DIR := $(CURDIR)/test/coverage
test-coverage: test-coverage-tools | $(BASE) ; $(info  Running coverage tests...) @ ## Run coverage tests
	mkdir -p $(COVERAGE_DIR)/coverage
	cd $(BASE) && for pkg in $(TESTPKGS); do \
		go test \
			-coverpkg=$$(go list -f '{{ join .Deps "\n" }}' $$pkg | \
					grep '^$(PACKAGE)/' | grep -v '^$(PACKAGE)/vendor/' | \
					tr '\n' ',')$$pkg \
			-covermode=$(COVERAGE_MODE) \
			-coverprofile="$(COVERAGE_DIR)/coverage/`echo $$pkg | tr "/" "-"`.cover" $$pkg ;\
	done
	$(GOCOVMERGE) $(COVERAGE_DIR)/coverage/*.cover > $(COVERAGE_PROFILE)
	go tool cover -html=$(COVERAGE_PROFILE) -o $(COVERAGE_HTML)
	$(GOCOV) convert $(COVERAGE_PROFILE) | $(GOCOVXML) > $(COVERAGE_XML)
	$(GCOV2LCOV) -infile $(COVERAGE_PROFILE) -outfile $(COVERAGE_DIR)/lcov.info