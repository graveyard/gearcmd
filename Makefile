VERSION := $(shell cat VERSION)
SHELL := /bin/bash
PKG = github.com/Clever/gearcmd/cmd/gearcmd
SUBPKGS := \
github.com/Clever/gearcmd/argsparser \
github.com/Clever/gearcmd/gearcmd
PKGS := $(PKG) $(SUBPKGS)
EXECUTABLE := gearcmd
BUILDS := \
	build/$(EXECUTABLE)-v$(VERSION)-darwin-amd64 \
	build/$(EXECUTABLE)-v$(VERSION)-linux-amd64
COMPRESSED_BUILDS := $(BUILDS:%=%.tar.gz)
RELEASE_ARTIFACTS := $(COMPRESSED_BUILDS:build/%=release/%)

.PHONY: test $(PKGS) clean release

GOVERSION := $(shell go version | grep 1.5)
ifeq "$(GOVERSION)" ""
  $(error must be running Go version 1.5)
endif

export GO15VENDOREXPERIMENT = 1

$(GOPATH)/bin/golint:
	@go get github.com/golang/lint/golint


test: $(PKGS)

$(PKGS): cmd/gearcmd/version.go $(GOPATH)/bin/golint
	@gofmt -w=true $(GOPATH)/src/$@*/**.go
	@echo "LINTING..."
	@$(GOPATH)/bin/golint $(GOPATH)/src/$@*/**.go
	@echo ""
ifeq ($(COVERAGE),1)
	@go test -cover -coverprofile=$(GOPATH)/src/$@/c.out $@ -test.v
	@go tool cover -html=$(GOPATH)/src/$@/c.out
else
	@echo "TESTING..."
	@go test $@ -test.v
endif

build/*: cmd/gearcmd/version.go
cmd/gearcmd/version.go: VERSION
	echo 'package main' > cmd/gearcmd/version.go
	echo '' >> cmd/gearcmd/version.go # Write a go file that lints :)
	echo 'const Version = "$(VERSION)"' >> cmd/gearcmd/version.go

build/$(EXECUTABLE)-v$(VERSION)-darwin-amd64:
	GOARCH=amd64 GOOS=darwin go build -o "$@/$(EXECUTABLE)" $(PKG)
build/$(EXECUTABLE)-v$(VERSION)-linux-amd64:
	GOARCH=amd64 GOOS=linux go build -o "$@/$(EXECUTABLE)" $(PKG)
build: $(BUILDS)

%.tar.gz: %
	tar -C `dirname $<` -zcvf "$<.tar.gz" `basename $<`

$(RELEASE_ARTIFACTS): release/% : build/%
	mkdir -p release
	cp $< $@

release: $(RELEASE_ARTIFACTS)

clean:
	rm -rf build release
