// SPDX-License-Identifier: MIT

#include <stdint.h>

#include "dmrgo.h"

void *dmrgo__ptr(uintptr_t h) { return (void *)(h * 16 + 0x100000u); }

const char *dmrgo__gen_strerror(int status);

int dmrgo_abi_version(void) { return DMRGO_ABI_VERSION; }

const char *dmrgo_strerror(int status) {
	switch (status) {
	case DMRGO_OK:
		return "ok";
	case DMRGO_ERR_HANDLE:
		return "invalid handle";
	case DMRGO_ERR_ARG:
		return "bad argument";
	case DMRGO_ERR_SHORT:
		return "output buffer too small";
	case DMRGO_ERR_PANIC:
		return "panic in the library";
	case DMRGO_ERR_OTHER:
		return "error without a status code";
	case DMRGO_ERR_BOUND:
		return "output passed the bound of its _need function";
	case DMRGO_ERR_NO_METHOD:
		return "vtable has no function for the method";
	}
	return dmrgo__gen_strerror(status);
}
