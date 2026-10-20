CC ?= cc
BUILD ?= build

.PHONY: all test clean

all: $(BUILD)/libdmrgo.so

$(BUILD)/libdmrgo.so: $(wildcard *.go *.c) dmrgo.h go.mod
	go build -buildmode=c-shared -o $@ .
	rm -f $(BUILD)/libdmrgo.h

	$(CC) -std=c11 -Wall -Wextra -Werror -o $@ $< -I. -L$(BUILD) -ldmrgo -Wl,-rpath,'$$ORIGIN'

test: $(BUILD)/capi_test

clean:
	rm -rf $(BUILD)
