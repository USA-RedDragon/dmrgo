// Package constants defines the DMR timers, counters and addresses as Go
// constants: time.Duration values for timers and plain integers for counters
// and addresses.
//
// Annex A of ETSI TS 102 361-2 and ETSI TS 102 361-3 gives its values in
// clauses, so those citations name a clause. Annex A of ETSI TS 102 361-4
// gives its values in Tables A.1 to A.8. A name that ends in Default is a
// value that the library chose, inside the range of the spec where the spec
// gives a range. (ETSI TS 102 361-2 Annex A, ETSI TS 102 361-3 Annex A,
// ETSI TS 102 361-4 Annex A)
//
// # Call timer tokens
//
// The air interface carries the Tier III call timers as tokens, not in
// seconds. In CallTimer_Parms, token 0 means that the MS uses its internal
// timer (ETSI TS 102 361-4 Table 7.70).
//
//   - T_EMERG_TIMER is a 9-bit token. 1 to 10 are 1 s to 10 s, 11 to 20 are
//     15 s to 60 s in 5 s steps, 21 to 28 are 75 s to 180 s in 15 s steps,
//     29 to 40 are 3.5 min to 9 min in 30 s steps, 41 to 51 are 10 min to
//     20 min in 1 min steps, 52 to 510 are 25 min to 2315 min in 5 min
//     steps, and 511 is infinity (ETSI TS 102 361-4 Table A.2).
//   - T_PACKET_TIMER is a 5-bit token. 1 to 5 are 1 s to 5 s, 6 to 10 are
//     10 s to 30 s in 5 s steps, 11 to 12 are 45 s to 60 s in 15 s steps,
//     13 to 20 are 1.5 min to 5 min in 30 s steps, 21 to 25 are 6 min to
//     10 min in 1 min steps, 26 to 30 are 15 min to 35 min in 5 min steps,
//     and 31 is infinity (ETSI TS 102 361-4 Table A.3).
//   - T_MS-MS_TIMER and T_MS-LINE_TIMER are 12-bit tokens. 1 to 59 are 1 s
//     to 59 s, 60 to 107 are 60 s to 295 s in 5 s steps, 108 to 138 are
//     5 min to 20 min in 30 s steps, 139 to 4094 are 21 min to 3976 min in
//     1 min steps, and 4095 is infinity (ETSI TS 102 361-4 Tables A.4 and
//     A.5).
package constants
