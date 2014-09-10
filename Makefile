SHELL := /bin/bash
PKG = #github.com/Clever/gearcmd
SUBPKGS := \
github.com/Clever/gearcmd/gearcmd \
github.com/Clever/gearcmd/argsparser
PKGS := $(PKG) $(SUBPKGS)

.PHONY: test golint

golint:
	@go get github.com/golang/lint/golint

test: $(PKGS)

$(PKGS): golint
	@go get -d -t $@
	@gofmt -w=true $(GOPATH)/src/$@*/**.go
	@echo "LINTING..."
	@PATH=$(PATH):$(GOPATH)/bin golint $(GOPATH)/src/$@*/**.go
	@echo ""
ifeq ($(COVERAGE),1)
	@go test -cover -coverprofile=$(GOPATH)/src/$@/c.out $@ -test.v
	@go tool cover -html=$(GOPATH)/src/$@/c.out
else
	@echo "TESTING..."
	@go test $@ -test.v
endif
