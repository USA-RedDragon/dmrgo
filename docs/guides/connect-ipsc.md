# Connect to an IPSC system

This guide shows how to join a Motorola IP Site Connect (IPSC) system as a peer, or run its master, with the HMAC-SHA1 authentication of MOTOTRBO repeaters, and how to pass the voice and data packets of a call.

## Goal

Register a peer with the master of an IPSC system over UDP, get the peer list, keep the link alive, and send and receive the packets of a call. Or run the master that MOTOTRBO repeaters register with.

## What you need

- Go and a checkout of dmrgo. The example runs offline: both sides use UDP on the loopback interface, and it contacts no network.
- The packages [transport/ipsc](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc), [transport/ipsc/ipscsession](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscsession) and [transport/ipsc/ipscauth](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscauth).
- The peer ID of your side and, when the system uses authentication, its key: the hex key of the MOTOTRBO or DMRlink configuration.

Run the example from the top of the repository:

```sh
go test -C v2 -run 'ExamplePeer_loopback$' -v ./transport/ipsc/ipscsession
```

## Steps

### 1. Parse the key and make the Peer and the Master

[ipscauth.ParseKey](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscauth#ParseKey) reads the hex key of the configuration and pads it on the left to 20 octets, as DMRlink does. Set `Auth` and `Key` in the config of each side. [ipscsession.NewPeer](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscsession#NewPeer) takes the peer ID and the address of the master as an [Addr](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscsession#Addr). [AddrOf](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscsession#AddrOf) converts the IP and port of a `net.UDPAddr`, as the example does for each packet that it reads. [ipscsession.NewMaster](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscsession#NewMaster) makes the master.

<!-- dmrgo:code file=v2/transport/ipsc/ipscsession/udp_example_test.go symbol=newMachines -->
```go
func newMachines(masterAt net.Addr) (*ipscsession.Peer, *ipscsession.Master, error) {
	key, err := ipscauth.ParseKey("123456789abcdef1234")
	if err != nil {
		return nil, nil, err
	}
	at, err := netip.ParseAddrPort(masterAt.String())
	if err != nil {
		return nil, nil, err
	}
	masterAddr := ipscsession.Addr{IP: at.Addr().As4(), Port: at.Port()}
	master, err := ipscsession.NewMaster(&ipscsession.MasterConfig{ID: 1, Auth: true, Key: key})
	if err != nil {
		return nil, nil, err
	}
	peer, err := ipscsession.NewPeer(&ipscsession.PeerConfig{ID: 314926, Master: masterAddr, Auth: true, Key: key})
	if err != nil {
		return nil, nil, err
	}
	return peer, master, nil
}
```
<!-- /dmrgo:code -->

### 2. Pass packets between the socket and the machines

The machines do no I/O. Give each UDP packet to `Packet` of the machine with the address that it came from. Send the `Packet` of each `OutputSend` output to its `Addr`: an IPSC peer also talks to the other peers of the list, so each output has its own address. Call `Tick` at the time that `Deadline` returns. The first `Tick` of a Peer sends the registration request, and later ticks send the keepalives.

The Peer returns `OutputUp` when the master answers its registration, and `OutputPeers` when it gets a peer list. It then registers with each other peer of the list. The Master returns `OutputUp` for each new peer.

<!-- dmrgo:code file=v2/transport/ipsc/ipscsession/udp_example_test.go symbol=passPackets -->
```go
func passPackets(call []byte, masterConn, peerConn *net.UDPConn, peer *ipscsession.Peer, master *ipscsession.Master) error {
	start := time.Now()
	toMaster, toPeer := 0, 0
	send := func(conn *net.UDPConn, n *int, outs []ipscsession.Output) error {
		for _, o := range outs {
			if o.Kind != ipscsession.OutputSend {
				continue
			}
			to := &net.UDPAddr{IP: o.Addr.IP[:], Port: int(o.Addr.Port)}
			if _, err := conn.WriteTo(o.Packet, to); err != nil {
				return err
			}
			*n++
		}
		return nil
	}
	read := func(conn *net.UDPConn, buf []byte) ([]byte, ipscsession.Addr, error) {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			return nil, ipscsession.Addr{}, err
		}
		a, _ := ipscsession.AddrOf(from.IP, from.Port)
		return buf[:n], a, nil
	}
	if err := send(peerConn, &toMaster, peer.Tick(time.Since(start), nil)); err != nil {
		return err
	}

	buf := make([]byte, ipscsession.MaxPacket)
	for toMaster+toPeer > 0 {
		for ; toMaster > 0; toMaster-- {
			p, from, err := read(masterConn, buf)
			if err != nil {
				return err
			}
			fmt.Println("master gets", ipsc.Opcode(p[0]))
			outs := master.Packet(time.Since(start), from, p, nil, nil)
			for _, o := range outs {
				switch o.Kind {
				case ipscsession.OutputData:
					u, err := ipsc.DecodeUser(o.Packet)
					if err != nil {
						return err
					}
					fmt.Println("master:", o.Kind, o.ID, u.Opcode, "from", u.Source, "to", u.Destination)
				case ipscsession.OutputSend:
				default:
					fmt.Println("master:", o.Kind, o.ID)
				}
			}
			if err := send(masterConn, &toPeer, outs); err != nil {
				return err
			}
		}
		for ; toPeer > 0; toPeer-- {
			p, from, err := read(peerConn, buf)
			if err != nil {
				return err
			}
			fmt.Println("peer gets", ipsc.Opcode(p[0]))
			outs := peer.Packet(time.Since(start), from, p, nil)
			for _, o := range outs {
				if o.Kind == ipscsession.OutputSend {
					continue
				}
				fmt.Println("peer:", o.Kind, o.ID)
				if o.Kind == ipscsession.OutputPeers {
					if err := sendSigned(peer, peerConn, masterConn.LocalAddr(), call); err != nil {
						return err
					}
					toMaster++
				}
			}
			if err := send(peerConn, &toMaster, outs); err != nil {
				return err
			}
		}
	}
	return nil
}
```
<!-- /dmrgo:code -->

### 3. Send and receive the packets of a call

With `Auth`, sign each voice or data packet with [Peer.Sign](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscsession#Peer.Sign) before you send it. The machines check the trailer of each packet that they get, and return the voice and data packets of registered peers as `OutputData`, without the trailer. [ipsc.DecodeUser](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc#DecodeUser) decodes one. The Master does not send them on by itself; send each one to the other peers that should get it.

<!-- dmrgo:code file=v2/transport/ipsc/ipscsession/udp_example_test.go symbol=sendSigned -->
```go
func sendSigned(peer *ipscsession.Peer, conn *net.UDPConn, to net.Addr, packet []byte) error {
	signed, err := peer.Sign(append([]byte(nil), packet...))
	if err != nil {
		return err
	}
	_, err = conn.WriteTo(signed, to)
	return err
}
```
<!-- /dmrgo:code -->

An IPSC user packet carries the decoded payload of a burst in place of the 264-bit burst: three AMBE+2 frames without FEC, or the payload of a data burst. [User.RawBurst](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc#User.RawBurst) rebuilds the burst, [User.SetRawBurst](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc#User.SetRawBurst) takes one apart, and [User.Hint](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc#User.Hint) gives the `layer2.Hint` for the voice and data machines. The hint marks voice bursts A and E only; a [layer2.PositionTracker](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#PositionTracker) places B, C, D and F. To build the packets of a call from scratch, [ipsc.UserOpcode](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc#UserOpcode) picks the opcode and an [ipsc.Stream](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/ipsc#Stream) fills the RTP header and the call fields of each packet.

<!-- dmrgo:code file=v2/transport/ipsc/example_test.go symbol=ExampleDecodeUser -->
```go
func ExampleDecodeUser() {
	for _, s := range []string{
		"800004ce2e1d30b43c0000090200000d9e2080dd7b5f482e7d3e0000000001c0000a800a006000000000000930b43ce9f8ed0071466f",
		"800004ce2e0130b43c000009020000465e00805d2d490a223107000000000a1440541cbd8ee6d793cd54372cbe82e8c4004a2a78",
	} {
		p, _ := hex.DecodeString(s)
		u, err := ipsc.DecodeUser(p)
		if err != nil {
			fmt.Println(err)
			return
		}
		h := u.Hint(0)
		fmt.Println(u.Opcode, u.Source, u.Destination, u.Info.Slot(), u.IsVoice(), h.Kind)
	}
}
```
<!-- /dmrgo:code -->

## Output

The registration with the master, the peer list, and the voice LC header of a call at the master:

<!-- dmrgo:output test=ipscsession.ExamplePeer_loopback -->
```text
master gets RegisterRequest
master: Up 314926
peer gets RegisterReply
peer: Up 1
master gets PeerListRequest
peer gets PeerListReply
peer: Peers 0
master gets GroupVoice
master: Data 314926 GroupVoice from 3191868 to 9
```
<!-- /dmrgo:output -->

A voice LC header and a voice burst from a Motorola XPR 8400:

<!-- dmrgo:output test=ipsc.ExampleDecodeUser -->
```text
GroupVoice 3191868 9 TS2 false Data
GroupVoice 3191868 9 TS1 true Voice
```
<!-- /dmrgo:output -->

The authentication trailer of a registration request:

<!-- dmrgo:output test=ipscauth.ExampleKey_Sign -->
```text
5a09decc4db02fba02dc
900004ce2e6a0000805c04080400 true
```
<!-- /dmrgo:output -->

<!-- dmrgo:config pkg=transport/ipsc/ipscsession type=PeerConfig -->
| Field | Type | Default | Description |
|---|---|---|---|
| `ID` | `uint32` | zero value | ID is the peer ID. |
| `Mode` | `ipsc.Mode` | `ModeDefault` (106) | Mode is the mode that the peer sends. 0 means ModeDefault. |
| `Flags` | `ipsc.Flags` | zero value | Flags is the flags that the peer sends. The zero Flags means voice and data calls. The Peer sets ipsc.FlagAuth with Auth. |
| `RegisterVersion` | `[4]byte` | not machine-readable; see the description | RegisterVersion is the protocol version that the peer sends in a register request or reply (0x90, 0x91, 0x94, 0x95). Zero means RegisterVersionDefault. |
| `AliveVersion` | `[4]byte` | not machine-readable; see the description | AliveVersion is the protocol version that the peer sends in a keepalive request or reply (0x96, 0x97). Zero means AliveVersionDefault. |
| `Master` | `Addr` | zero value | Master is the address of the master. |
| `Auth` | `bool` | zero value | Auth turns on the HMAC-SHA1 trailer with Key: the Peer drops a packet without a good trailer and signs what it sends. |
| `Key` | `[20]byte` | zero value | Key is the authentication key (see ipscauth.ParseKey). |
| `Alive` | `time.Duration` | not machine-readable; see the description | Alive is the keepalive period. 0 means AliveDefault. |
| `MaxMissed` | `int` | `MaxMissedDefault` (20) | MaxMissed is the number of keepalives without a reply after which the Peer takes a link as down. 0 means MaxMissedDefault. |
| `MaxPeers` | `int` | `MaxPeersDefault` (15) | MaxPeers is the most other peers that the Peer keeps from the peer list. 0 means MaxPeersDefault. |
<!-- /dmrgo:config -->

<!-- dmrgo:config pkg=transport/ipsc/ipscsession type=MasterConfig -->
| Field | Type | Default | Description |
|---|---|---|---|
| `ID` | `uint32` | zero value | ID is the peer ID of the master. |
| `Mode` | `ipsc.Mode` | `ModeDefault` (106) | Mode is the mode that the master sends. 0 means ModeDefault. |
| `Flags` | `ipsc.Flags` | zero value | Flags is the flags that the master sends. The zero Flags means voice and data calls. The Master sets ipsc.FlagMasterPeer, and ipsc.FlagAuth with Auth. |
| `RegisterVersion` | `[4]byte` | not machine-readable; see the description | RegisterVersion is the protocol version that the master sends in a register request or reply (0x90, 0x91, 0x94, 0x95). Zero means RegisterVersionDefault. |
| `AliveVersion` | `[4]byte` | not machine-readable; see the description | AliveVersion is the protocol version that the master sends in a keepalive request or reply (0x96, 0x97). Zero means AliveVersionDefault. |
| `Auth` | `bool` | zero value | Auth turns on the HMAC-SHA1 trailer with Key: the Master drops a packet without a good trailer and signs what it sends. |
| `Key` | `[20]byte` | zero value | Key is the authentication key (see ipscauth.ParseKey). |
| `Timeout` | `time.Duration` | not machine-readable; see the description | Timeout is the time without a packet from a peer after which the Master drops it. 0 means TimeoutDefault. |
| `MaxPeers` | `int` | `MaxPeersDefault` (15) | MaxPeers is the most peers that the Master registers. 0 means MaxPeersDefault. |
| `AdoptKnown` | `bool` | zero value | AdoptKnown makes Packet adopt a peer that is not registered, as Adopt does, when the trailer of its packet proves the key of the config or the key that the caller passed, so that a peer that registered with another Master needs no Adopt call. Without a key it does nothing. |
<!-- /dmrgo:config -->

## How it works

A peer sends a registration request to the master, and the master answers with a registration reply. The peer then requests the peer list, registers with each other peer of the list, and sends keepalives to the master and to each peer every `Alive`. It takes a link as down after `MaxMissed` keepalives without a reply. The master sends the peer list to every peer when one requests it and when a peer leaves, times out or is dropped. It drops a peer after `Timeout` without a packet. The defaults of these timers are those of DMRlink.

With authentication, each packet ends with a 10-octet trailer: the first 10 octets of the HMAC-SHA1 of the packet, with the 20-octet key. A side with `Auth` drops a packet without a good trailer and signs what it sends. A Master can serve repeaters with keys of their own: pass the key of the sending peer to `Master.Packet`.

The voice and data packets have an RTP header, and octet 17 holds the call info: the slot, the end of the call and the privacy bit. (DMRlink 1f50db2 dmrlink.py and ipsc_const.py; ipsc2hbrp; captures of a Motorola XPR 8400, 2026-10-15 to 2026-10-20)

The state machines: [ipscsession Peer](../machines/ipscsession-peer.md) and [ipscsession Master](../machines/ipscsession-master.md).

## Related guides

- [Connect to a network with the MMDVM network protocol or HBRP](connect-dmrd.md)
- [Connect to a Hytera repeater](connect-hytera.md)
- [Rewrite and relay a voice call](relay-voice-call.md)
- [Register radios and send text: ARS and TMS](register-radios-send-text.md)
