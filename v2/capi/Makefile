CC ?= cc
CXX ?= c++
BUILD ?= build
ABI := $(shell sed -n 's/^abi //p' abi.txt)
SONAME := libdmrgo.so.$(ABI)
LDFLAGS := -extldflags=-Wl,-soname,$(SONAME),--version-script=$(CURDIR)/libdmrgo.map
SRC := $(wildcard *.go *.c *.h) abi.txt libdmrgo.map go.mod
RUN = $(1)/api_test ../layer2/testdata/parrot_kerchunk.bin

.PHONY: all test clean

all: $(BUILD)/libdmrgo.so

$(BUILD)/$(SONAME): $(SRC)
	go build -buildmode=c-shared -ldflags='$(LDFLAGS)' -o $@ .
	rm -f $(BUILD)/libdmrgo.so.h $(BUILD)/libdmrgo.h

$(BUILD)/libdmrgo.so: $(BUILD)/$(SONAME)
	ln -sf $(SONAME) $@

$(BUILD)/stack/$(SONAME): $(SRC)
	go build -buildmode=c-shared -gcflags=all=-d=maymorestack=runtime.mayMoreStackMove -ldflags='$(LDFLAGS)' -o $@ .

$(BUILD)/check2/$(SONAME): $(SRC)
	GOEXPERIMENT=cgocheck2 go build -buildmode=c-shared -ldflags='$(LDFLAGS)' -o $@ .

$(BUILD)/%_test: test/%_test.c dmrgo.h dmrgo_gen.h $(BUILD)/libdmrgo.so
	$(CC) -std=c11 -Wall -Wextra -Werror -pedantic -o $@ $< -I. -L$(BUILD) -ldmrgo -Wl,-rpath,'$$ORIGIN'

test: $(BUILD)/api_test $(BUILD)/stack/$(SONAME) $(BUILD)/check2/$(SONAME)
	echo '#include "dmrgo.h"' | $(CXX) -std=c++11 -Wall -Wextra -Werror -pedantic -fsyntax-only -I. -x c++ -
	nm -D --defined-only $(BUILD)/$(SONAME) | awk '$$2 == "T" { print $$3 }' | sort > $(BUILD)/symbols
	sed -n 's/^\t\t\(dmrgo_[a-z0-9_]*\);$$/\1/p' libdmrgo.map | sort | diff -u - $(BUILD)/symbols
	$(call RUN,$(BUILD))
	LD_LIBRARY_PATH=$(BUILD)/stack sh -c '$(call RUN,$(BUILD))'
	LD_LIBRARY_PATH=$(BUILD)/check2 sh -c '$(call RUN,$(BUILD))'
	GOEXPERIMENT=cgocheck2 go test -count=1 -run 'TestABI|AsGo' .
	go test -count=1 -gcflags=all=-d=maymorestack=runtime.mayMoreStackMove -run 'TestABI|AsGo' .

clean:
	rm -rf $(BUILD)
