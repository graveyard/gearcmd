VERSION := $(shell cat VERSION)
SHELL := /bin/bash
PKG = github.com/Clever/gearcmd/cmd/gearcmd
PKGS := $(shell go list ./... | grep -v /vendor)
EXECUTABLE := gearcmd
BUILDS := \
	build/$(EXECUTABLE)-v$(VERSION)-darwin-amd64 \
	build/$(EXECUTABLE)-v$(VERSION)-linux-amd64
COMPRESSED_BUILDS := $(BUILDS:%=%.tar.gz)
RELEASE_ARTIFACTS := $(COMPRESSED_BUILDS:build/%=release/%)
.PHONY: test $(PKGS) clean release vendor

GOVERSION := $(shell go version | grep 1.5)
ifeq "$(GOVERSION)" ""
  $(error must be running Go version 1.5)
endif
export GO15VENDOREXPERIMENT = 1

all: test build

FGT := $(GOPATH)/bin/fgt
$(FGT):
	go get github.com/GeertJohan/fgt

GOLINT := $(GOPATH)/bin/golint
$(GOLINT):
	go get github.com/golang/lint/golint

GODEP := $(GOPATH)/bin/godep
$(GODEP):
	go get -u github.com/tools/godep

test: $(PKGS)

$(PKGS): cmd/gearcmd/version.go $(GOLINT) $(FGT)
	@echo "FORMATING..."
	@$(FGT) gofmt -l=true $(GOPATH)/src/$@*/**.go
	@echo "LINTING..."
	@$(FGT) $(GOLINT) $(GOPATH)/src/$@*/**.go
	@echo "TESTING..."
	@go test -v $@

build/*: cmd/gearcmd/version.go
cmd/gearcmd/version.go: VERSION
	@echo 'package main' > $@
	@echo '' >> $@ # Write a go file that lints :)
	@echo '// Version denotes the version of the executable' >> $@ # golint compliance
	echo 'const Version = "$(VERSION)"' >> $@

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

vendor: $(GODEP)
	$(GODEP) save $(PKGS)
	find vendor/ -path '*/vendor' -type d | xargs -IX rm -r X # remove any nested vendor directories
