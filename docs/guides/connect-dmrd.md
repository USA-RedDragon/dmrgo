# Connect to a network with the MMDVM network protocol or HBRP

This guide shows how to log a repeater or hotspot into a master with the MMDVM network protocol or the HomeBrew Repeater Protocol (HBRP), how to run the master side, and how to pass the DMRD packets of a call.

## Goal

Run the repeater side (Peer) and the master side (Master) of a session over UDP, log in with the password of the master, and send the bursts of a call in DMRD packets.

## What you need

- Go and a checkout of dmrgo. The example runs offline: both sides use UDP on the loopback interface, and it contacts no network.
- The packages [transport/dmrd](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd) and [transport/dmrd/dmrdsession](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession).
- The dialect of your master. Package dmrd covers four protocols built around the same DMRD packet. They differ in the login, keepalive and other control messages:
  - [dmrd.DialectMMDVM](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd#Dialect): the MMDVM network protocol of MMDVMHost and DMRGateway. BrandMeister, DMR+, HBlink, FreeDMR and DMRHub masters accept it. It has binary repeater IDs, RPTACK, RPTPING and MSTPONG.
  - `dmrd.DialectHBRP`: the HomeBrew Repeater Protocol, in the form of its 2015 specification and of the Homebrew port of BrandMeister. It has repeater IDs, salt and token as hex text, MSTACK, MSTPING and RPTPONG, and no RPTO.
  - `dmrd.DialectLocal`: the link between MMDVMHost and DMRGateway on one host, without a login.
  - OpenBridge, the link between two masters. A [dmrdsession.Bridge](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#Bridge) checks its signatures.

The MMDVM network protocol and HBRP are different protocols. Pick the dialect that your master speaks.

Run the example from the top of the repository:

```sh
go test -C v2 -run ExamplePeer_loopback -v ./transport/dmrd/dmrdsession
```

## Steps

### 1. Make the RPTC configuration

The repeater sends its configuration in an RPTC message during the login. [dmrd.MMDVMConfig](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd#MMDVMConfig) is the RPTC of the MMDVM network protocol and [dmrd.HBRPConfig](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd#HBRPConfig) that of HBRP. `Reset` fills the fixed octets, and the setters write the repeater ID, callsign, frequencies, color code and the other fields in place.

<!-- dmrgo:code file=v2/transport/dmrd/dmrdsession/udp_example_test.go symbol=rptcConfig -->
```go
func rptcConfig(dialect dmrd.Dialect) []byte {
	if dialect == dmrd.DialectMMDVM {
		var c dmrd.MMDVMConfig
		c.Reset()
		c.SetID(3149999)
		return c[:]
	}
	var c dmrd.HBRPConfig
	c.Reset()
	c.SetID(3149999)
	return c[:]
}
```
<!-- /dmrgo:code -->

### 2. Make the Peer and the Master

[dmrdsession.NewPeer](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#NewPeer) takes the dialect, the repeater ID, the password of the master and the RPTC. `PeerConfig.Options` is the RPTO options text, which the Peer sends after the RPTC is acknowledged. [dmrdsession.NewMaster](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession#NewMaster) makes the master side of one repeater. Keep one Master for each address that sends you packets.

<!-- dmrgo:code file=v2/transport/dmrd/dmrdsession/udp_example_test.go symbol=newMachines -->
```go
func newMachines(dialect dmrd.Dialect, rptc []byte) (*dmrdsession.Peer, *dmrdsession.Master, error) {
	peer, err := dmrdsession.NewPeer(&dmrdsession.PeerConfig{Dialect: dialect, ID: 3149999, Password: []byte("passw0rd"), Config: rptc})
	if err != nil {
		return nil, nil, err
	}
	master, err := dmrdsession.NewMaster(&dmrdsession.MasterConfig{Dialect: dialect})
	if err != nil {
		return nil, nil, err
	}
	return peer, master, nil
}
```
<!-- /dmrgo:code -->

### 3. Pass packets between the socket and the machines

The machines do no I/O. Read each UDP packet and give it to `Packet` of the machine. Send the `Packet` of each `OutputSend` output to the other side. Call `Tick` at the time that `Deadline` returns: the first `Tick` of a Peer sends the RPTL, and later ticks send the retries and the pings.

At the Master, `OutputLogin` reports an RPTL. Answer it with `Admit` and a random salt and the password of the repeater, or with `Refuse`. `OutputConfig` reports the RPTC. Check it and call `Accept` or `Refuse`. The Peer returns `OutputUp` when the login is complete.

<!-- dmrgo:code file=v2/transport/dmrd/dmrdsession/udp_example_test.go symbol=passPackets -->
```go
func passPackets(dialect dmrd.Dialect, call []byte, masterConn, peerConn *net.UDPConn, peer *dmrdsession.Peer, master *dmrdsession.Master) error {
	start := time.Now()
	toMaster, toPeer := 0, 0
	send := func(conn *net.UDPConn, to net.Addr, n *int, outs []dmrdsession.Output) {
		for _, o := range outs {
			if o.Kind == dmrdsession.OutputSend {
				if _, err := conn.WriteTo(o.Packet, to); err == nil {
					*n++
				}
			}
		}
	}
	read := func(conn *net.UDPConn, buf []byte) ([]byte, net.Addr, error) {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, from, err := conn.ReadFrom(buf)
		return buf[:n], from, err
	}
	send(peerConn, masterConn.LocalAddr(), &toMaster, peer.Tick(time.Since(start), nil))

	fmt.Println(dialect)
	buf := make([]byte, 1500)
	for toMaster+toPeer > 0 {
		for ; toMaster > 0; toMaster-- {
			p, from, err := read(masterConn, buf)
			if err != nil {
				return err
			}
			fmt.Println("  repeater sends", dmrd.Classify(p, dialect))
			for _, o := range master.Packet(time.Since(start), p, nil) {
				switch o.Kind {
				case dmrdsession.OutputSend:
					send(masterConn, from, &toPeer, []dmrdsession.Output{o})
				case dmrdsession.OutputLogin:
					var salt [4]byte
					if _, err := rand.Read(salt[:]); err != nil {
						return err
					}
					send(masterConn, from, &toPeer, master.Admit(time.Since(start), binary.BigEndian.Uint32(salt[:]), []byte("passw0rd"), nil))
				case dmrdsession.OutputConfig:
					send(masterConn, from, &toPeer, master.Accept(time.Since(start), nil))
				case dmrdsession.OutputData:
					d, err := dmrd.DecodeData(o.Packet)
					if err != nil {
						return err
					}
					fmt.Println("  master gets a call from", d.Source, "to", d.Destination, "on", d.Slot)
				default:
				}
			}
		}
		for ; toPeer > 0; toPeer-- {
			p, _, err := read(peerConn, buf)
			if err != nil {
				return err
			}
			fmt.Println("  master sends", dmrd.Classify(p, dialect))
			for _, o := range peer.Packet(time.Since(start), p, nil) {
				send(peerConn, masterConn.LocalAddr(), &toMaster, []dmrdsession.Output{o})
				if o.Kind == dmrdsession.OutputUp {
					fmt.Println("  repeater is", peer.State())
					if _, err := peerConn.WriteTo(call, masterConn.LocalAddr()); err == nil {
						toMaster++
					}
				}
			}
		}
	}
	return nil
}
```
<!-- /dmrgo:code -->

### 4. Send and receive the bursts of a call

Once the Peer is up, send each burst in a DMRD packet on the same socket. The Master returns each DMRD packet that it gets from a logged-in repeater as `OutputData`. [dmrd.DecodeData](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd#DecodeData) decodes it, and [Data.Hint](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd#Data.Hint) gives the `layer2.Hint` for the voice and data machines. To build a DMRD packet from the output of a machine, such as a [voice.Transmitter](../machines/voice-transmitter.md), set the call and burst kind with [Data.SetCallHint](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd#Data.SetCallHint) and encode it with `Data.AppendBinary`.

<!-- dmrgo:code file=v2/transport/dmrd/example_test.go symbol=ExampleDecodeData -->
```go
func ExampleDecodeData() {
	p, _ := hex.DecodeString("444d52440030b43c00005b003010afa1506290d020580b4015ec0f6871000d005c4d5d7f77fd757361a416203d52352421818502770000")
	fmt.Println(dmrd.Classify(p, dmrd.DialectMMDVM))
	d, err := dmrd.DecodeData(p)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(d.Source, d.Destination, d.Repeater, d.Slot, d.Group, d.Frame, d.Size)
	h := d.Hint(0)
	fmt.Println(h.Kind, h.DataType)
}
```
<!-- /dmrgo:code -->

## Output

The login in each dialect, then the voice LC header of a call at the master:

<!-- dmrgo:output test=dmrdsession.ExamplePeer_loopback -->
```text
MMDVM
  repeater sends RPTL
  master sends RPTACK
  repeater sends RPTK
  master sends RPTACK
  repeater sends RPTC
  master sends RPTACK
  repeater is Running
  repeater sends DMRD
  master gets a call from 3191868 to 91 on TS2
HBRP
  repeater sends RPTL
  master sends MSTACK
  repeater sends RPTK
  master sends MSTACK
  repeater sends RPTC
  master sends MSTACK
  repeater is Running
  repeater sends DMRD
  master gets a call from 3191868 to 91 on TS2
```
<!-- /dmrgo:output -->

The DMRD packet and the hint of its burst:

<!-- dmrgo:output test=dmrd.ExampleDecodeData -->
```text
DMRD
3191868 91 3149999 TS2 true Data Sync 55
Data Voice LC Header
```
<!-- /dmrgo:output -->

<!-- dmrgo:config pkg=transport/dmrd/dmrdsession type=PeerConfig -->
| Field | Type | Default | Description |
|---|---|---|---|
| `Dialect` | `dmrd.Dialect` | zero value | Dialect is dmrd.DialectMMDVM or dmrd.DialectHBRP. |
| `ID` | `uint32` | zero value | ID is the repeater ID. |
| `Password` | `[]byte` | zero value | Password is the password of the master. The Peer copies it. |
| `Config` | `[]byte` | zero value | Config is the RPTC message: a dmrd.MMDVMConfig (302 octets) for DialectMMDVM, a dmrd.HBRPConfig (306 octets) for DialectHBRP. The Peer copies it. |
| `Options` | `[]byte` | zero value | Options is the RPTO options text, at most MaxOptions octets, which the Peer sends after the RPTC is acknowledged. Empty sends no RPTO. DialectHBRP has no RPTO. |
| `Retry` | `time.Duration` | not machine-readable; see the description | Retry is the time between two sends of a login step, between two pings, and before a new login after the session ended. 0 means RetryDefault. |
| `Timeout` | `time.Duration` | not machine-readable; see the description | Timeout is the time without an acknowledgement or a pong after which the Peer ends the session and logs in again. 0 means TimeoutDefault. |
| `Backoff` | `time.Duration` | zero value | Backoff is the longest wait before a new login. Each session that ends before the login completes doubles the wait from Retry up to Backoff. 0, or a value up to Retry, waits Retry each time, as DMRGateway does. |
<!-- /dmrgo:config -->

<!-- dmrgo:config pkg=transport/dmrd/dmrdsession type=MasterConfig -->
| Field | Type | Default | Description |
|---|---|---|---|
| `Dialect` | `dmrd.Dialect` | zero value | Dialect is dmrd.DialectMMDVM or dmrd.DialectHBRP. |
| `Timeout` | `time.Duration` | not machine-readable; see the description | Timeout is the time without a packet from the repeater after which the Master ends the session. 0 means TimeoutDefault. |
| `LoginTimeout` | `time.Duration` | not machine-readable; see the description | LoginTimeout is the time after the first RPTL of a login in which the repeater must finish its login, or the Master ends the session. A repeated RPTL or RPTK does not restart it. 0 means LoginTimeoutDefault. |
<!-- /dmrgo:config -->

## How it works

The Peer follows DMRGateway. It sends RPTL with its repeater ID, and the master answers with a salt. It then sends RPTK with the SHA-256 token of the salt and the password, then RPTC and, when it has options, RPTO, each after the acknowledgement of the one before. It sends a step again every `Retry` when no answer comes, pings every `Retry` when it is logged in, and logs in again after `Timeout` without a reply, after MSTNAK and after MSTCL. Package [dmrdauth](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdauth) computes the token.

The Master answers each step and sends `OutputLogin` and `OutputConfig` to the caller before it answers the RPTL and the RPTC. It answers a packet from a repeater that is not logged in with MSTNAK. It ends the session after `LoginTimeout` without a packet during the login, and after `Timeout` after it.

The DMRD packet is the same in every dialect: the sequence number, the source and destination IDs, the repeater ID, the slot, the call type, the frame type and voice position or data type, the stream ID and the 33-octet burst. The 55-octet form adds the bit error count and the RSSI, and the 71-octet form of DMRGateway adds a trunking UUID. `Data.Size` sets the form; its zero value sends 55 octets, as MMDVMHost and DMRGateway do. DecodeData and Data.AppendBinary take no dialect. (DL5DI, G4KLX and DG1HT, IPSC Protocol Specs for homebrew DMR repeater, 2015-07-26; MMDVMHost 9384d68 and DMRGateway ee79d38)

The state machines: [dmrdsession Peer](../machines/dmrdsession-peer.md), [dmrdsession Master](../machines/dmrdsession-master.md) and [dmrdsession Bridge](../machines/dmrdsession-bridge.md).

## Related guides

- [Connect to an IPSC system](connect-ipsc.md)
- [Connect to a Hytera repeater](connect-hytera.md)
- [Use an MMDVM modem](mmdvm-modem.md)
- [Rewrite and relay a voice call](relay-voice-call.md)
- [Clean up bursts for a hotspot](clean-up-bursts.md)
