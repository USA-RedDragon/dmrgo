// SPDX-License-Identifier: MIT

#include <stdint.h>

#include "dmrgo.h"


int dmrgo_abi_version(void) { return DMRGO_ABI_VERSION; }


const char *dmrgo_strerror(int status) {
	switch (status) {
	case DMRGO_OK:
		return "ok";
	case DMRGO_ERR_HANDLE:
		return "invalid receiver handle";
	case DMRGO_ERR_ARG:
		return "null argument";
	}
	return "unknown status";
}
