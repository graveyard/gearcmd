include golang.mk
.DEFAULT_GOAL := test # override default goal set in library makefile

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

$(eval $(call golang-version-check,1.6))

all: test build

test: $(PKGS)

$(PKGS): golang-test-all-deps cmd/gearcmd/version.go
	$(call golang-test-all,$@)

build/*: cmd/gearcmd/version.go
cmd/gearcmd/version.go: VERSION
	@echo 'package main' > $@
	@echo '' >> $@  # Write a go file that lints :)
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

vendor: golang-godep-vendor-deps
	$(call golang-godep-vendor,$(PKGS))
