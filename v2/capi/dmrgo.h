/* SPDX-License-Identifier: MIT */

/*
 * dmrgo.h is the C API of libdmrgo, the dmrgo library
 * (github.com/USA-RedDragon/dmrgo/v2) built as a shared library.
 *
 * dmrgo_gen.h, which this header includes, holds the C form of the whole
 * library. Its functions return DMRGO_OK or a negative status and write
 * their results to out pointers, also when they return an error. Outputs
 * follow these rules:
 *
 * - A result array or string (T *out, size_t out_cap, size_t *out_len,
 *   size_t *out_need) gets what fits; *out_len is the count written and
 *   *out_need the count needed (for a string, the bytes with the NUL).
 * - An append (T *dst, size_t dst_cap, size_t *dst_len, size_t *dst_need)
 *   adds after the first *dst_len items. *dst_len is the fill: on return
 *   it is the new fill (also with an error, as Go returns the extended
 *   slice), and after DMRGO_ERR_SHORT it is unchanged. *dst_need is the
 *   fill that the call needs.
 * - The strings and arrays inside output structs are copied, at call time,
 *   into the caller's arena (void *arena, size_t arena_cap, size_t
 *   *arena_len, size_t *arena_need). *arena_len is the fill, as for an
 *   append: the call adds after it, so pointers from earlier calls stay
 *   valid, and set it to 0 to reuse the arena. *arena_need is the fill the
 *   call needs, at any alignment of the arena. arena may be NULL with
 *   arena_cap 0 to ask for the size. Items are aligned for their type.
 * - When an output does not fit, the call returns DMRGO_ERR_SHORT and
 *   leaves every fill unchanged. A call on a handle has a _need function
 *   that gives the items and arena bytes the call can use for the same
 *   arguments, without a change to the handle. The call checks that room
 *   first: with less it returns DMRGO_ERR_SHORT before it runs, so the
 *   handle does not change. A call on a handle without a bound says so in
 *   its comment. A call without a handle can be repeated.
 */

#ifndef DMRGO_H
#define DMRGO_H

#include <stddef.h>
#include <stdint.h>

#include "dmrgo_gen.h"

#ifdef __cplusplus
extern "C" {
#endif

/* The status codes of the API. Errors are negative. */
enum dmrgo_status {
	DMRGO_OK = 0,
	/* A handle is NULL, freed or of another type. */
	DMRGO_ERR_HANDLE = -2,
	/* A pointer argument is NULL. */
	DMRGO_ERR_ARG = -3,
	/* An output did not fit its buffer. The lengths give the full size;
	 * see the output rules above for the retry. */
	DMRGO_ERR_SHORT = -6,
	/* The library panicked. dmrgo_last_error_message gives the panic. */
	DMRGO_ERR_PANIC = -7,
	/* The library returned an error that has no status code of its own.
	 * dmrgo_last_error_message gives it. Codes from -100 down are the
	 * errors of the library, as dmrgo_gen.h defines them. */
	DMRGO_ERR_OTHER = -8,
	/* An output passed the bound that the _need function of the call gave,
	 * which only an implementation in C of a Go interface can cause, such
	 * as a codec whose decode returns more than frame_samples() samples.
	 * The call took effect, and the fills are unchanged. */
	DMRGO_ERR_BOUND = -9
};

#ifndef DMRGO_NO_PROTOTYPES

/* dmrgo_abi_version returns DMRGO_ABI_VERSION of the library. The ABI
 * version changes when a declaration of dmrgo.h or dmrgo_gen.h changes in a
 * way that breaks a program built against an older one. */
int dmrgo_abi_version(void);

/* dmrgo_version writes the version of the dmrgo module that the library
 * was built from. */
int dmrgo_version(char *buf, size_t cap, size_t *len, size_t *need);

/* dmrgo_last_error_code returns the status code of the last error that a
 * function returned on this thread, or DMRGO_OK. */
int dmrgo_last_error_code(void);

/* dmrgo_last_error_message writes the message of the last error that a
 * function returned on this thread, as a result string. The library keeps
 * the first 511 bytes of a message; it returns DMRGO_ERR_SHORT when the
 * message was cut there or does not fit cap. A function that succeeds does
 * not clear it. */
int dmrgo_last_error_message(char *buf, size_t cap, size_t *len, size_t *need);

/* dmrgo_strerror returns a static message for a status code. */
const char *dmrgo_strerror(int status);

#endif

#ifdef __cplusplus
}
#endif

#endif
