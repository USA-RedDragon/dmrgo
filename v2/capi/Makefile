CC ?= cc
CXX ?= c++
BUILD ?= build
ABI := $(shell sed -n 's/^abi //p' abi.txt)
SONAME := libdmrgo.so.$(ABI)
VERSION ?= $(shell git describe --tags --match 'v2.*' --always --dirty 2>/dev/null)
LDFLAGS := -X main.version=$(VERSION) -extldflags=-Wl,-soname,$(SONAME),--version-script=$(CURDIR)/libdmrgo.map
SRC := $(wildcard *.go *.c *.h) abi.txt libdmrgo.map go.mod
RUN = $(1)/api_test ../layer2/testdata/parrot_kerchunk.bin
STATIC_LIBS := $(shell pkg-config --libs codec2 2>/dev/null) -lpthread -lm

.PHONY: all static test clean

all: $(BUILD)/libdmrgo.so

$(BUILD)/$(SONAME): $(SRC)
	go build -buildmode=c-shared -ldflags='$(LDFLAGS)' -o $@ .
	rm -f $(BUILD)/libdmrgo.so.h $(BUILD)/libdmrgo.h

$(BUILD)/libdmrgo.so: $(BUILD)/$(SONAME)
	ln -sf $(SONAME) $@

static: $(BUILD)/static/libdmrgo.a

$(BUILD)/static/libdmrgo.a: $(SRC)
	mkdir -p $(BUILD)/static
	go build -buildmode=c-archive -ldflags='-X main.version=$(VERSION)' -o $(BUILD)/static/go.a .
	ld -r --whole-archive $(BUILD)/static/go.a -o $(BUILD)/static/libdmrgo.o
	sed -n 's/^\t\t\(dmrgo_[a-z0-9_]*\);$$/\1/p' libdmrgo.map | sort > $(BUILD)/static/symbols.api
	objcopy --keep-global-symbols=$(BUILD)/static/symbols.api $(BUILD)/static/libdmrgo.o
	rm -f $@ $(BUILD)/static/go.a $(BUILD)/static/go.h
	ar rcs $@ $(BUILD)/static/libdmrgo.o

$(BUILD)/static/api_test: test/api_test.c dmrgo.h dmrgo_gen.h $(BUILD)/static/libdmrgo.a
	$(CC) -std=c11 -Wall -Wextra -Werror -pedantic -DDMRGO_TEST_VERSION='"$(VERSION)"' -o $@ $< -I. $(BUILD)/static/libdmrgo.a $(STATIC_LIBS)

$(BUILD)/stack/$(SONAME): $(SRC)
	go build -buildmode=c-shared -gcflags=all=-d=maymorestack=runtime.mayMoreStackMove -ldflags='$(LDFLAGS)' -o $@ .

$(BUILD)/check2/$(SONAME): $(SRC)
	GOEXPERIMENT=cgocheck2 go build -buildmode=c-shared -ldflags='$(LDFLAGS)' -o $@ .

$(BUILD)/%_test: test/%_test.c dmrgo.h dmrgo_gen.h $(BUILD)/libdmrgo.so
	$(CC) -std=c11 -Wall -Wextra -Werror -pedantic -DDMRGO_TEST_VERSION='"$(VERSION)"' -o $@ $< -I. -L$(BUILD) -ldmrgo -Wl,-rpath,'$$ORIGIN'

test: $(BUILD)/api_test $(BUILD)/stack/$(SONAME) $(BUILD)/check2/$(SONAME) $(BUILD)/static/api_test
	echo '#include "dmrgo.h"' | $(CXX) -std=c++11 -Wall -Wextra -Werror -pedantic -fsyntax-only -I. -x c++ -
	nm -D --defined-only $(BUILD)/$(SONAME) | awk '$$2 == "T" { print $$3 }' | sort > $(BUILD)/symbols
	sed -n 's/^\t\t\(dmrgo_[a-z0-9_]*\);$$/\1@@DMRGO_$(ABI)/p' libdmrgo.map | sort | diff -u - $(BUILD)/symbols
	$(call RUN,$(BUILD))
	$(call RUN,$(BUILD)/static)
	nm -g --defined-only $(BUILD)/static/libdmrgo.a | awk 'NF == 3 { print $$3 }' | sort | diff -u $(BUILD)/static/symbols.api -
	LD_LIBRARY_PATH=$(BUILD)/stack sh -c '$(call RUN,$(BUILD))'
	LD_LIBRARY_PATH=$(BUILD)/check2 sh -c '$(call RUN,$(BUILD))'
	GOEXPERIMENT=cgocheck2 go test -count=1 -run 'TestABI|AsGo' .
	go test -count=1 -gcflags=all=-d=maymorestack=runtime.mayMoreStackMove -run 'TestABI|AsGo' .

clean:
	rm -rf $(BUILD)
