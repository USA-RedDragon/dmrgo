/* SPDX-License-Identifier: MIT */

/*
 * dmrgo.h is the C API of libdmrgo, the dmrgo library
 * (github.com/USA-RedDragon/dmrgo/v2) built as a shared library.
 *
 */

#ifndef DMRGO_H
#define DMRGO_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

/* DMRGO_ABI_VERSION is the version of this API. dmrgo_abi_version returns
 * the version of the loaded library. */
#define DMRGO_ABI_VERSION 1



/* The status codes of the API. Errors are negative. */
enum dmrgo_status {
	DMRGO_OK = 0,
	/* The receiver handle is NULL or freed. */
	DMRGO_ERR_HANDLE = -2,
	/* A pointer argument is NULL. */
	DMRGO_ERR_ARG = -3,
};






#ifndef DMRGO_NO_PROTOTYPES

/* dmrgo_abi_version returns DMRGO_ABI_VERSION of the library. */
int dmrgo_abi_version(void);


/* dmrgo_strerror returns a static message for a status code. */
const char *dmrgo_strerror(int status);











#endif

#ifdef __cplusplus
}
#endif

#endif
