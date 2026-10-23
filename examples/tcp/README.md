# tcp

This example runs a TN3270 session over TCP/IPv4 over DMR confirmed data. Two simulated radios carry the IPv4 packets as SAP IP based packet data (ETSI TS 102 361-3 clause 5.5). Each end runs the TCP/IP stack of gVisor (netstack) on a simulated clock, so TCP has its real timers, congestion control (CUBIC) and SACK. The example prints the time of each screen at rate 1/2, 3/4 and 1, on a simplex and a duplex channel, with and without a DMR window.

Run it from this directory:

```sh
go run . [-cycles 5] [-seed 1] [-mtu 576]
```

The example is its own Go module, so gVisor is not a dependency of the dmrgo library. gVisor needs Go 1.27.3.

## The session

The terminal connects to port 23 of the host, and the two ends do the RFC 1576 negotiation (terminal type IBM-3278-2-E, then BINARY and EOR). Then each cycle has two transactions:

1. Submit: the terminal sends the filled service order (160 octets), and the host replies with a list of open orders (1674 octets).
2. Back: the terminal sends PF3 (5 octets), and the host sends the blank form again (316 octets).

The time of a transaction is from the first octet the terminal writes to the last octet of the reply the terminal reads. Setup is from the TCP connect to the end of the first form screen.

## The parts

- [clock.go](clock.go) is a `tcpip.Clock` that moves only when the simulation moves it. The simulation reads the time of the next TCP timer from it.
- [host.go](host.go) is one netstack with one NIC and one IPv4 address, `ip.RadioAddress` of its radio ID.
- [link.go](link.go) carries the IPv4 packets of one radio over `data.Sender` and `data.Receiver`. Each packet is one C_HEAD packet, so the MTU must fit one DMR packet: at most 1266 octets at rate 1/2.
- [air.go](air.go) is the channel model of [../udp](../udp): each bit of a burst flips with the bit error rate (BER), and bursts that overlap are lost.
- [tn3270.go](tn3270.go) is the data stream: Erase/Write, SBA and SF orders, the AID record, and IAC EOR at the end of each record.
- [sim.go](sim.go) runs the session. netstack does its TCP work on its own goroutines, so after each event the simulation waits until the state of both stacks stops changing. `TestRepeatable` runs the grid two times and compares the output.

## Simplex and duplex

- **Simplex:** one DMR logical channel carries both directions: the data of the terminal, the data of the host, and the DMR responses to both.
- **Duplex:** each direction has its own logical channel, for example TS1 from the terminal and TS2 from the host. Each channel carries the data of one end and the DMR responses of the other end. Each radio transmits on one slot while it receives on the other slot, with two data calls at once. An MMDVM duplex modem can do this. Most subscriber radios cannot.

## Results

TN3270 over TCP over DMR, MTU 576, 5 cycles of form submit and back. Records: negotiation 54 octets in 6 records, form screen 316, form input 160, list screen 1674, PF3 5:

DMR window 1 (stop and wait):

| channel | window | rate | BER | setup s | submit mean s | submit p95 s | back mean s | TCP segments | TCP retransmits | RTO timeouts | DMR packets | bursts |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| simplex | 1 | Rate 1/2 | 0 | 22.3 | 20.5 | 21.6 | 4.7 | 95 | 1 | 0 | 95 | 2234 |
| simplex | 1 | Rate 1/2 | 0.001 | 22.3 | 20.5 | 21.6 | 4.7 | 95 | 1 | 0 | 95 | 2234 |
| simplex | 1 | Rate 1/2 | 0.005 | 22.3 | 20.5 | 21.6 | 4.7 | 95 | 1 | 0 | 95 | 2234 |
| simplex | 1 | Rate 1/2 | 0.01 | 22.3 | 20.1 | 22.2 | 5.3 | 96 | 1 | 0 | 96 | 2265 |
| simplex | 1 | Rate 1/2 | 0.02 | 22.8 | 21.7 | 22.3 | 4.9 | 95 | 1 | 0 | 95 | 2345 |
| simplex | 1 | Rate 3/4 | 0 | 20.4 | 15.5 | 16.6 | 3.5 | 97 | 2 | 0 | 97 | 1639 |
| simplex | 1 | Rate 3/4 | 0.001 | 20.4 | 15.6 | 16.6 | 3.5 | 97 | 2 | 0 | 97 | 1649 |
| simplex | 1 | Rate 3/4 | 0.005 | 21.1 | 17.6 | 19.0 | 4.0 | 95 | 0 | 0 | 95 | 1797 |
| simplex | 1 | Rate 3/4 | 0.01 | 23.0 | 24.1 | 36.0 | 4.9 | 93 | 0 | 0 | 93 | 2456 |
| simplex | 1 | Rate 3/4 | 0.02 | 54.7 | 41.3 | 62.9 | 10.3 | 103 | 7 | 2 | 103 | 4699 |
| simplex | 1 | Rate 1 | 0 | 11.7 | 13.3 | 14.5 | 2.9 | 91 | 0 | 0 | 91 | 1291 |
| simplex | 1 | Rate 1 | 0.001 | 21.3 | 19.6 | 27.5 | 6.9 | 97 | 3 | 0 | 97 | 2181 |
| simplex | 1 | Rate 1 | 0.005 | 80.9 | 124.4 | 193.3 | 20.9 | 126 | 27 | 10 | 126 | 6787 |
| simplex | 1 | Rate 1 | 0.01 | session stopped at 2m12.58s: terminal TCP error: operation timed out | | | | | | | | |
| simplex | 1 | Rate 1 | 0.02 | session stopped at 2m7s: terminal TCP error: operation timed out | | | | | | | | |
| duplex | 1 | Rate 1/2 | 0 | 13.1 | 15.5 | 15.7 | 4.6 | 95 | 2 | 0 | 95 | 2192 |
| duplex | 1 | Rate 1/2 | 0.001 | 13.1 | 15.5 | 15.7 | 4.6 | 95 | 2 | 0 | 95 | 2192 |
| duplex | 1 | Rate 1/2 | 0.005 | 13.1 | 15.5 | 15.7 | 4.6 | 95 | 2 | 0 | 95 | 2192 |
| duplex | 1 | Rate 1/2 | 0.01 | 13.1 | 15.9 | 17.3 | 4.6 | 95 | 2 | 0 | 95 | 2223 |
| duplex | 1 | Rate 1/2 | 0.02 | 13.4 | 16.3 | 17.2 | 4.8 | 95 | 2 | 0 | 95 | 2288 |
| duplex | 1 | Rate 3/4 | 0 | 10.5 | 10.4 | 10.4 | 3.3 | 93 | 1 | 0 | 93 | 1530 |
| duplex | 1 | Rate 3/4 | 0.001 | 10.5 | 10.4 | 10.4 | 3.3 | 93 | 1 | 0 | 93 | 1530 |
| duplex | 1 | Rate 3/4 | 0.005 | 10.7 | 11.6 | 12.1 | 3.9 | 93 | 1 | 0 | 93 | 1685 |
| duplex | 1 | Rate 3/4 | 0.01 | 14.4 | 14.4 | 15.0 | 4.6 | 95 | 2 | 0 | 95 | 2060 |
| duplex | 1 | Rate 3/4 | 0.02 | 24.2 | 26.8 | 36.3 | 7.7 | 98 | 3 | 0 | 98 | 3580 |
| duplex | 1 | Rate 1 | 0 | 8.3 | 8.1 | 8.1 | 2.4 | 89 | 1 | 0 | 89 | 1199 |
| duplex | 1 | Rate 1 | 0.001 | 10.7 | 11.7 | 12.7 | 3.8 | 89 | 1 | 0 | 89 | 1660 |
| duplex | 1 | Rate 1 | 0.005 | 37.7 | 380.6 | 1230.7 | 34.5 | 144 | 47 | 25 | 143 | 8334 |
| duplex | 1 | Rate 1 | 0.01 | session stopped at 2m12.58s: half open, the host dropped the handshake and the terminal waits | | | | | | | | |
| duplex | 1 | Rate 1 | 0.02 | session stopped at 2m7s: terminal TCP error: operation timed out | | | | | | | | |

<details><summary>All rows at MTU 576, with DMR window 4</summary>

| channel | window | rate | BER | setup s | submit mean s | submit p95 s | back mean s | TCP segments | TCP retransmits | RTO timeouts | DMR packets | bursts |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| simplex | 1 | Rate 1/2 | 0 | 22.3 | 20.5 | 21.6 | 4.7 | 95 | 1 | 0 | 95 | 2234 |
| simplex | 1 | Rate 1/2 | 0.001 | 22.3 | 20.5 | 21.6 | 4.7 | 95 | 1 | 0 | 95 | 2234 |
| simplex | 1 | Rate 1/2 | 0.005 | 22.3 | 20.5 | 21.6 | 4.7 | 95 | 1 | 0 | 95 | 2234 |
| simplex | 1 | Rate 1/2 | 0.01 | 22.3 | 20.1 | 22.2 | 5.3 | 96 | 1 | 0 | 96 | 2265 |
| simplex | 1 | Rate 1/2 | 0.02 | 22.8 | 21.7 | 22.3 | 4.9 | 95 | 1 | 0 | 95 | 2345 |
| simplex | 1 | Rate 3/4 | 0 | 20.4 | 15.5 | 16.6 | 3.5 | 97 | 2 | 0 | 97 | 1639 |
| simplex | 1 | Rate 3/4 | 0.001 | 20.4 | 15.6 | 16.6 | 3.5 | 97 | 2 | 0 | 97 | 1649 |
| simplex | 1 | Rate 3/4 | 0.005 | 21.1 | 17.6 | 19.0 | 4.0 | 95 | 0 | 0 | 95 | 1797 |
| simplex | 1 | Rate 3/4 | 0.01 | 23.0 | 24.1 | 36.0 | 4.9 | 93 | 0 | 0 | 93 | 2456 |
| simplex | 1 | Rate 3/4 | 0.02 | 54.7 | 41.3 | 62.9 | 10.3 | 103 | 7 | 2 | 103 | 4699 |
| simplex | 1 | Rate 1 | 0 | 11.7 | 13.3 | 14.5 | 2.9 | 91 | 0 | 0 | 91 | 1291 |
| simplex | 1 | Rate 1 | 0.001 | 21.3 | 19.6 | 27.5 | 6.9 | 97 | 3 | 0 | 97 | 2181 |
| simplex | 1 | Rate 1 | 0.005 | 80.9 | 124.4 | 193.3 | 20.9 | 126 | 27 | 10 | 126 | 6787 |
| simplex | 1 | Rate 1 | 0.01 | session stopped at 2m12.58s: terminal TCP error: operation timed out | | | | | | | | |
| simplex | 1 | Rate 1 | 0.02 | session stopped at 2m7s: terminal TCP error: operation timed out | | | | | | | | |
| simplex | 4 | Rate 1/2 | 0 | 16.6 | 16.0 | 17.6 | 7.5 | 101 | 4 | 0 | 101 | 2096 |
| simplex | 4 | Rate 1/2 | 0.001 | 16.6 | 16.0 | 17.6 | 7.5 | 101 | 4 | 0 | 101 | 2096 |
| simplex | 4 | Rate 1/2 | 0.005 | 16.6 | 16.0 | 17.6 | 7.5 | 101 | 4 | 0 | 101 | 2096 |
| simplex | 4 | Rate 1/2 | 0.01 | 16.6 | 31.0 | 91.0 | 7.0 | 105 | 8 | 1 | 105 | 3335 |
| simplex | 4 | Rate 1/2 | 0.02 | 136.3 | 65.8 | 101.1 | 87.1 | 125 | 24 | 14 | 125 | 9242 |
| simplex | 4 | Rate 3/4 | 0 | 14.3 | 10.6 | 11.7 | 4.7 | 99 | 4 | 0 | 99 | 1422 |
| simplex | 4 | Rate 3/4 | 0.001 | 14.3 | 21.2 | 62.2 | 3.6 | 100 | 6 | 2 | 100 | 2218 |
| simplex | 4 | Rate 3/4 | 0.005 | 649.7 | 150.4 | 304.6 | 228.7 | 152 | 53 | 32 | 152 | 15616 |
| simplex | 4 | Rate 3/4 | 0.01 | 40.3 | 311.2 | 493.1 | 11.2 | 153 | 55 | 24 | 153 | 16768 |
| simplex | 4 | Rate 3/4 | 0.02 | 364.7 | 285.5 | 401.3 | 87.6 | 141 | 38 | 7 | 141 | 11809 |
| simplex | 4 | Rate 1 | 0 | 13.4 | 8.8 | 12.2 | 3.9 | 103 | 6 | 0 | 103 | 1158 |
| simplex | 4 | Rate 1 | 0.001 | 225.7 | 183.6 | 290.4 | 10.5 | 157 | 64 | 31 | 157 | 14069 |
| simplex | 4 | Rate 1 | 0.005 | session stopped: not done after 1h10m0s, at record 12 of 27 | | | | | | | | |
| simplex | 4 | Rate 1 | 0.01 | session stopped at 2m25.42s: half open, the host dropped the handshake and the terminal waits | | | | | | | | |
| simplex | 4 | Rate 1 | 0.02 | session stopped at 2m7s: terminal TCP error: operation timed out | | | | | | | | |
| duplex | 1 | Rate 1/2 | 0 | 13.1 | 15.5 | 15.7 | 4.6 | 95 | 2 | 0 | 95 | 2192 |
| duplex | 1 | Rate 1/2 | 0.001 | 13.1 | 15.5 | 15.7 | 4.6 | 95 | 2 | 0 | 95 | 2192 |
| duplex | 1 | Rate 1/2 | 0.005 | 13.1 | 15.5 | 15.7 | 4.6 | 95 | 2 | 0 | 95 | 2192 |
| duplex | 1 | Rate 1/2 | 0.01 | 13.1 | 15.9 | 17.3 | 4.6 | 95 | 2 | 0 | 95 | 2223 |
| duplex | 1 | Rate 1/2 | 0.02 | 13.4 | 16.3 | 17.2 | 4.8 | 95 | 2 | 0 | 95 | 2288 |
| duplex | 1 | Rate 3/4 | 0 | 10.5 | 10.4 | 10.4 | 3.3 | 93 | 1 | 0 | 93 | 1530 |
| duplex | 1 | Rate 3/4 | 0.001 | 10.5 | 10.4 | 10.4 | 3.3 | 93 | 1 | 0 | 93 | 1530 |
| duplex | 1 | Rate 3/4 | 0.005 | 10.7 | 11.6 | 12.1 | 3.9 | 93 | 1 | 0 | 93 | 1685 |
| duplex | 1 | Rate 3/4 | 0.01 | 14.4 | 14.4 | 15.0 | 4.6 | 95 | 2 | 0 | 95 | 2060 |
| duplex | 1 | Rate 3/4 | 0.02 | 24.2 | 26.8 | 36.3 | 7.7 | 98 | 3 | 0 | 98 | 3580 |
| duplex | 1 | Rate 1 | 0 | 8.3 | 8.1 | 8.1 | 2.4 | 89 | 1 | 0 | 89 | 1199 |
| duplex | 1 | Rate 1 | 0.001 | 10.7 | 11.7 | 12.7 | 3.8 | 89 | 1 | 0 | 89 | 1660 |
| duplex | 1 | Rate 1 | 0.005 | 37.7 | 380.6 | 1230.7 | 34.5 | 144 | 47 | 25 | 143 | 8334 |
| duplex | 1 | Rate 1 | 0.01 | session stopped at 2m12.58s: half open, the host dropped the handshake and the terminal waits | | | | | | | | |
| duplex | 1 | Rate 1 | 0.02 | session stopped at 2m7s: terminal TCP error: operation timed out | | | | | | | | |
| duplex | 4 | Rate 1/2 | 0 | 12.0 | 14.8 | 15.2 | 4.2 | 95 | 2 | 0 | 95 | 2120 |
| duplex | 4 | Rate 1/2 | 0.001 | 12.0 | 14.8 | 15.2 | 4.2 | 95 | 2 | 0 | 95 | 2120 |
| duplex | 4 | Rate 1/2 | 0.005 | 12.0 | 14.8 | 15.2 | 4.2 | 95 | 2 | 0 | 95 | 2120 |
| duplex | 4 | Rate 1/2 | 0.01 | 12.0 | 17.0 | 25.7 | 4.2 | 97 | 2 | 0 | 97 | 2328 |
| duplex | 4 | Rate 1/2 | 0.02 | 110.1 | 72.7 | 117.5 | 4.2 | 118 | 23 | 10 | 118 | 8340 |
| duplex | 4 | Rate 3/4 | 0 | 8.9 | 9.7 | 9.7 | 2.9 | 93 | 1 | 0 | 93 | 1446 |
| duplex | 4 | Rate 3/4 | 0.001 | 8.9 | 44.2 | 182.5 | 3.0 | 105 | 13 | 7 | 105 | 4002 |
| duplex | 4 | Rate 3/4 | 0.005 | 53.6 | 298.7 | 522.7 | 120.5 | 149 | 53 | 31 | 149 | 14130 |
| duplex | 4 | Rate 3/4 | 0.01 | 335.9 | 313.5 | 514.8 | 7.7 | 155 | 59 | 27 | 155 | 17669 |
| duplex | 4 | Rate 3/4 | 0.02 | 55.4 | 201.0 | 248.8 | 28.7 | 137 | 39 | 13 | 137 | 11320 |
| duplex | 4 | Rate 1 | 0 | 7.3 | 7.4 | 7.4 | 2.1 | 89 | 1 | 0 | 89 | 1125 |
| duplex | 4 | Rate 1 | 0.001 | 56.9 | 173.7 | 303.6 | 7.6 | 146 | 55 | 22 | 146 | 12433 |
| duplex | 4 | Rate 1 | 0.005 | 125.9 | 388.7 | 543.8 | 48.7 | 158 | 58 | 26 | 158 | 13134 |
| duplex | 4 | Rate 1 | 0.01 | session stopped at 2m25.42s: terminal TCP error: operation timed out | | | | | | | | |
| duplex | 4 | Rate 1 | 0.02 | session stopped at 2m7s: terminal TCP error: operation timed out | | | | | | | | |

</details>

<details><summary>All rows at MTU 1266</summary>

| channel | window | rate | BER | setup s | submit mean s | submit p95 s | back mean s | TCP segments | TCP retransmits | RTO timeouts | DMR packets | bursts |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| simplex | 1 | Rate 1/2 | 0 | 22.3 | 16.2 | 17.0 | 5.8 | 79 | 3 | 0 | 79 | 2063 |
| simplex | 1 | Rate 1/2 | 0.001 | 22.3 | 16.2 | 17.0 | 5.8 | 79 | 3 | 0 | 79 | 2063 |
| simplex | 1 | Rate 1/2 | 0.005 | 22.3 | 16.2 | 17.0 | 5.8 | 79 | 3 | 0 | 79 | 2063 |
| simplex | 1 | Rate 1/2 | 0.01 | 22.3 | 16.3 | 17.0 | 5.8 | 79 | 3 | 0 | 79 | 2074 |
| simplex | 1 | Rate 1/2 | 0.02 | 22.8 | 18.3 | 21.2 | 10.7 | 83 | 4 | 0 | 82 | 2598 |
| simplex | 1 | Rate 3/4 | 0 | 20.4 | 11.1 | 11.4 | 3.5 | 77 | 2 | 0 | 77 | 1414 |
| simplex | 1 | Rate 3/4 | 0.001 | 20.4 | 11.2 | 11.7 | 3.5 | 77 | 2 | 0 | 77 | 1420 |
| simplex | 1 | Rate 3/4 | 0.005 | 21.1 | 12.4 | 13.3 | 3.9 | 75 | 0 | 0 | 75 | 1551 |
| simplex | 1 | Rate 3/4 | 0.01 | 22.8 | 17.0 | 23.7 | 6.0 | 77 | 2 | 0 | 77 | 2094 |
| simplex | 1 | Rate 3/4 | 0.02 | 54.7 | 44.5 | 73.0 | 23.3 | 92 | 12 | 4 | 92 | 5131 |
| simplex | 1 | Rate 1 | 0 | 11.7 | 9.5 | 11.3 | 3.4 | 75 | 2 | 0 | 75 | 1140 |
| simplex | 1 | Rate 1 | 0.001 | 21.3 | 18.5 | 44.9 | 8.9 | 85 | 7 | 1 | 85 | 2342 |
| simplex | 1 | Rate 1 | 0.005 | 80.9 | 373.0 | 1589.9 | 35.3 | 111 | 34 | 21 | 111 | 8467 |
| simplex | 1 | Rate 1 | 0.01 | session stopped at 2m12.58s: terminal TCP error: operation timed out | | | | | | | | |
| simplex | 1 | Rate 1 | 0.02 | session stopped at 2m7s: terminal TCP error: operation timed out | | | | | | | | |
| simplex | 4 | Rate 1/2 | 0 | 16.6 | 15.1 | 20.0 | 8.5 | 87 | 7 | 2 | 87 | 2184 |
| simplex | 4 | Rate 1/2 | 0.001 | 16.6 | 15.1 | 20.0 | 8.5 | 87 | 7 | 2 | 87 | 2184 |
| simplex | 4 | Rate 1/2 | 0.005 | 16.6 | 15.1 | 20.0 | 8.5 | 87 | 7 | 2 | 87 | 2184 |
| simplex | 4 | Rate 1/2 | 0.01 | 16.6 | 30.3 | 84.4 | 8.7 | 91 | 10 | 3 | 91 | 3448 |
| simplex | 4 | Rate 1/2 | 0.02 | 136.3 | 142.5 | 236.6 | 5.6 | 103 | 30 | 20 | 103 | 13156 |
| simplex | 4 | Rate 3/4 | 0 | 14.3 | 9.9 | 13.1 | 5.6 | 87 | 8 | 2 | 87 | 1473 |
| simplex | 4 | Rate 3/4 | 0.001 | 14.3 | 67.2 | 222.8 | 34.4 | 92 | 17 | 9 | 92 | 6009 |
| simplex | 4 | Rate 3/4 | 0.005 | 649.7 | 278.8 | 425.1 | 4.4 | 125 | 47 | 31 | 125 | 15316 |
| simplex | 4 | Rate 3/4 | 0.01 | 54.2 | 308.9 | 496.3 | 9.6 | 114 | 33 | 19 | 114 | 11834 |
| simplex | 4 | Rate 3/4 | 0.02 | 364.7 | 417.5 | 928.9 | 58.2 | 123 | 41 | 19 | 123 | 13923 |
| simplex | 4 | Rate 1 | 0 | 13.4 | 7.1 | 8.1 | 4.2 | 81 | 5 | 0 | 81 | 1054 |
| simplex | 4 | Rate 1 | 0.001 | 225.7 | 111.4 | 195.3 | 4.5 | 107 | 34 | 19 | 107 | 9926 |
| simplex | 4 | Rate 1 | 0.005 | session stopped: not done after 1h10m0s, at record 14 of 27 | | | | | | | | |
| simplex | 4 | Rate 1 | 0.01 | session stopped at 2m25.42s: half open, the host dropped the handshake and the terminal waits | | | | | | | | |
| simplex | 4 | Rate 1 | 0.02 | session stopped at 2m7s: terminal TCP error: operation timed out | | | | | | | | |
| duplex | 1 | Rate 1/2 | 0 | 13.1 | 14.2 | 14.4 | 5.4 | 79 | 4 | 0 | 79 | 2087 |
| duplex | 1 | Rate 1/2 | 0.001 | 13.1 | 14.2 | 14.4 | 5.4 | 79 | 4 | 0 | 79 | 2087 |
| duplex | 1 | Rate 1/2 | 0.005 | 13.1 | 14.2 | 14.4 | 5.4 | 79 | 4 | 0 | 79 | 2087 |
| duplex | 1 | Rate 1/2 | 0.01 | 13.1 | 14.2 | 14.4 | 5.5 | 79 | 4 | 0 | 79 | 2101 |
| duplex | 1 | Rate 1/2 | 0.02 | 13.4 | 16.2 | 20.0 | 7.8 | 86 | 7 | 2 | 86 | 2538 |
| duplex | 1 | Rate 3/4 | 0 | 10.5 | 9.4 | 9.4 | 3.5 | 73 | 1 | 0 | 73 | 1367 |
| duplex | 1 | Rate 3/4 | 0.001 | 10.5 | 9.4 | 9.8 | 3.6 | 73 | 1 | 0 | 73 | 1378 |
| duplex | 1 | Rate 3/4 | 0.005 | 10.7 | 10.5 | 11.0 | 4.2 | 75 | 2 | 0 | 75 | 1524 |
| duplex | 1 | Rate 3/4 | 0.01 | 14.4 | 13.0 | 14.6 | 5.2 | 77 | 3 | 0 | 77 | 1909 |
| duplex | 1 | Rate 3/4 | 0.02 | 24.2 | 26.1 | 40.1 | 7.9 | 78 | 3 | 0 | 78 | 3125 |
| duplex | 1 | Rate 1 | 0 | 8.3 | 7.2 | 7.2 | 2.8 | 69 | 1 | 0 | 69 | 1072 |
| duplex | 1 | Rate 1 | 0.001 | 10.7 | 10.1 | 11.0 | 4.7 | 75 | 4 | 0 | 75 | 1583 |
| duplex | 1 | Rate 1 | 0.005 | 37.7 | 336.1 | 892.1 | 16.5 | 114 | 37 | 21 | 114 | 7940 |
| duplex | 1 | Rate 1 | 0.01 | session stopped at 2m12.58s: half open, the host dropped the handshake and the terminal waits | | | | | | | | |
| duplex | 1 | Rate 1 | 0.02 | session stopped at 2m7s: terminal TCP error: operation timed out | | | | | | | | |
| duplex | 4 | Rate 1/2 | 0 | 12.0 | 13.7 | 14.1 | 5.0 | 79 | 4 | 0 | 79 | 2025 |
| duplex | 4 | Rate 1/2 | 0.001 | 12.0 | 13.7 | 14.1 | 5.0 | 79 | 4 | 0 | 79 | 2025 |
| duplex | 4 | Rate 1/2 | 0.005 | 12.0 | 13.7 | 14.1 | 5.0 | 79 | 4 | 0 | 79 | 2025 |
| duplex | 4 | Rate 1/2 | 0.01 | 12.0 | 16.1 | 25.7 | 5.6 | 85 | 7 | 1 | 85 | 2362 |
| duplex | 4 | Rate 1/2 | 0.02 | 110.1 | 153.1 | 320.5 | 4.6 | 98 | 26 | 18 | 98 | 12092 |
| duplex | 4 | Rate 3/4 | 0 | 8.9 | 8.8 | 8.8 | 3.4 | 75 | 2 | 0 | 75 | 1337 |
| duplex | 4 | Rate 3/4 | 0.001 | 8.9 | 25.7 | 93.1 | 26.1 | 83 | 10 | 5 | 83 | 3716 |
| duplex | 4 | Rate 3/4 | 0.005 | 53.6 | 168.6 | 431.7 | 9.1 | 108 | 35 | 22 | 108 | 11344 |
| duplex | 4 | Rate 3/4 | 0.01 | 304.6 | 83.9 | 117.7 | 12.4 | 102 | 24 | 11 | 102 | 9120 |
| duplex | 4 | Rate 3/4 | 0.02 | 55.4 | 211.5 | 418.0 | 44.6 | 118 | 37 | 15 | 118 | 11191 |
| duplex | 4 | Rate 1 | 0 | 7.3 | 6.7 | 6.7 | 2.6 | 71 | 2 | 0 | 71 | 1042 |
| duplex | 4 | Rate 1 | 0.001 | 56.9 | 197.6 | 364.7 | 5.2 | 106 | 35 | 19 | 106 | 10007 |
| duplex | 4 | Rate 1 | 0.005 | 125.9 | 644.6 | 1046.7 | 151.4 | 144 | 63 | 37 | 144 | 14162 |
| duplex | 4 | Rate 1 | 0.01 | session stopped at 2m25.42s: terminal TCP error: operation timed out | | | | | | | | |
| duplex | 4 | Rate 1 | 0.02 | session stopped at 2m7s: terminal TCP error: operation timed out | | | | | | | | |

</details>

The columns:

- setup: from the TCP connect to the end of the first form screen.
- submit: from Enter on the form to the end of the 1674-octet reply.
- back: from PF3 to the end of the 316-octet form.
- TCP segments, retransmits and RTO timeouts: both ends together, for the whole session, as netstack counts them. Retransmits do not include SYN and SYN-ACK.
- DMR packets: packets that the two Senders took, one for each IPv4 packet.
- bursts: all bursts on all channels.

## What the results show

- **Airtime sets the floor.** A confirmed rate 1/2 block carries 10 octets in one 60 ms burst, so the user data rate is about 1.3 kbit/s (2.1 at rate 3/4, 2.9 at rate 1). The reply screen with its TCP/IP headers is about 1900 octets, so it needs about 11 s of rate 1/2 airtime in one direction. A submit at rate 1/2 takes 20 s on a simplex channel and 15 s on a duplex channel.
- **Each TCP segment costs one DMR packet for its ACK.** netstack sent a separate ACK for each data segment, and an ACK before the data of each reply. On a simplex channel each ACK waits for the channel and has its own preambles, header and DMR response.
- **TCP sends its handshake again on a clean channel.** The first retransmit timer of TCP is 1 s (RFC 6298), and a DMR round trip with preambles and responses is longer. At each rate the terminal sent its SYN again at 1 s, and the host sent its SYN-ACK one or two more times. The retransmits column does not count these; it counts data segments, here small negotiation records sent again.
- **Duplex is faster with DMR window 1.** With window 1, duplex is faster at rate 1/2 and 3/4 for each BER, and at rate 1 up to a BER of 0.001: 15.5 s against 20.5 s for a submit at rate 1/2. It removes the wait for the channel between the two directions, so TCP ACKs and DMR responses do not stop the data of the other end. With window 4 on a channel with errors, both modes are slow and duplex is sometimes slower.
- **A DMR window helps only on a clean channel.** Window 4 cuts a submit at rate 1/2 from 20.5 s to 16.0 s up to a BER of 0.005, but at 0.01 the submit takes 31 s. At rate 3/4 and 1 the window fails at a BER of 0.001. In window mode the receiver keeps the blocks of the last packet only, so one lost block makes the sender send that packet and every later packet of the window again (go-back-N, ETSI TS 102 361-3 clause 5.4.4). Each window counts as one try of each of its packets, so the tries run out.
- **Rate 1 does not work with TCP above a BER of about 0.001.** At a BER of 0.005 a submit takes 2 to 6 minutes. At a BER of 0.01 the session does not start, or it is half open: the terminal has the connection, but the final ACK of the handshake did not arrive and the host gave up its SYN-ACK retries. Telnet waits for the host to speak first, so the terminal waits forever. A box must use TCP keepalive or an application timeout.
- **A larger MTU helps when the channel is good.** At MTU 1266 a reply needs 2 segments, not 4, so it needs fewer ACKs, preambles and DMR responses: a submit at rate 1/2 on a simplex channel takes 16.2 s, not 20.5 s. On the worst channels (rate 3/4 at a BER of 0.02, rate 1 at 0.005) a large packet has more blocks to lose, and MTU 576 is faster.

## Limits of the simulation

- The bit errors are independent. A real mobile channel has fades, which lose bursts in groups.
- The radios talk direct, with no repeater and no network leg.
- netstack is not the TCP of Linux or of a TN3270 product. Its timers and ACK rules are close to Linux, but they are not the same.
