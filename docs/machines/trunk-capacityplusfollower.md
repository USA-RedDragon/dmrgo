# trunk CapacityPlusFollower

This page covers the trunk CapacityPlusFollower, the state machine that follows the calls of a MOTOTRBO Capacity Plus or Linked Capacity Plus site without transmitting.

## What it does

A [CapacityPlusFollower](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#CapacityPlusFollower) follows a Capacity Plus or Linked Capacity Plus site as a scanner does. These sites have no TSCC. Every call starts on the rest channel, and the rest channel moves when a call takes it. The Follower finds the rest channel, moves with it, and follows the calls that its config selects: from the group or private voice grant in the Voice LC Header on the rest channel, or by late entry from the channel activity of a site status. Channel numbers name repeaters: repeater n carries logical slot number (LSN) 2n-1 on timeslot 1 and LSN 2n on timeslot 2. The Follower returns tune requests, and you tune the receiver. It does no I/O. No public spec describes the protocol; the decoding follows recordings of real systems.

## When to use it

Use a CapacityPlusFollower to listen to a Capacity Plus or Linked Capacity Plus site: a scanner, a recorder, or a monitor of site activity. For an ETSI Tier III site with a TSCC, use the [Follower](trunk-follower.md).

To decode the voice of a call, pass the call of the grant to [voice.Receiver.Expect](voice-receiver.md) and feed the bursts of the granted slot to that Receiver, as for a Tier III site.

## Config

Pass a [CapacityPlusConfig](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#CapacityPlusConfig) to [NewCapacityPlusFollower](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#NewCapacityPlusFollower). A nil config follows every talkgroup on the repeater that the receiver is on. Set a [ChannelPlan](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#ChannelPlan) with the frequencies of the repeaters, so the Follower can hunt and tune. Set Individual to follow private calls too. Set NoFollow for a receiver that cannot tune.

<!-- dmrgo:config pkg=trunk type=CapacityPlusConfig -->
<!-- /dmrgo:config -->

## States and transitions

[State](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#CapacityPlusFollower.State) returns the current state. The type is the [State](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#State) of the Tier III Follower, with the rest channel in place of the TSCC.

<!-- dmrgo:states pkg=trunk type=CapacityPlusFollower -->
<!-- /dmrgo:states -->

The Follower starts by hunting. With a plan, the first Tick or Burst requests the first repeater, and the Follower moves to the next repeater in number order when no valid Capacity Plus PDU comes within the no-signal time. A rest channel beacon or site status moves it to the rest channel, and it reports the site. When the site announces a new rest channel, the Follower moves with it. A voice grant or the activity of a site status for a selected call moves it to the payload slot of the call. The hang time after a Terminator with LC, the inactive time, or a lost signal returns it to the rest channel, or to the hunt when no rest channel is known. On the rest channel, the no-signal time without a valid PDU starts the hunt again.

## Inputs and outputs

Inputs:

- [Burst](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#CapacityPlusFollower.Burst) takes a raw burst, its [layer2.Hint](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#Hint), and the CACH field before it in transmit order. Hint.Slot, or the TDMA channel of the CACH, gives the timeslot of the burst.
- [Decoded](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#CapacityPlusFollower.Decoded) takes a burst that you decoded already.
- [Lost](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#CapacityPlusFollower.Lost) reports that the receiver lost the signal.
- [Tick](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#CapacityPlusFollower.Tick) runs the timers. Call it at the time [Deadline](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#CapacityPlusFollower.Deadline) returns.
- [Reset](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#CapacityPlusFollower.Reset) clears the site and the call and starts the hunt again.

Outputs are [Output](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#Output) values of the same kinds as for the Tier III Follower: tune requests, the site, the loss of the rest channel, grants and the end of a followed call. A tune request and a [Grant](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#Grant) name the repeater in Channel and the timeslot of the LSN in Slot. A grant from a site status has the site status opcode and LateEntry set; one from a Voice LC Header has opcode 0.

The accessors [RestLSN](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#CapacityPlusFollower.RestLSN), [SiteNumber](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#CapacityPlusFollower.SiteNumber) and [Call](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#CapacityPlusFollower.Call) return the rest channel, the Linked Capacity Plus site number and the followed call. Give each method a slice with capacity <!-- dmrgo:const pkg=trunk name=MaxOutputs --><!-- /dmrgo:const -->.

## Timers

- No-signal time: the time without a valid Capacity Plus PDU on the rest channel after which the Follower hunts again, and the time it stays on each repeater while it hunts. The default is <!-- dmrgo:const pkg=trunk name=CapacityPlusNoSignalDefault --><!-- /dmrgo:const -->, more than twice the beacon period of an idle rest channel.
- Inactive time: the time without a burst of the call after which the Follower returns to the rest channel. The default is <!-- dmrgo:const pkg=constants name=TVInactiveDefault --><!-- /dmrgo:const -->.
- Hang time: the time after a Terminator with LC in which the Follower waits for more voice. The default is <!-- dmrgo:const pkg=constants name=TVHangtimeDefault --><!-- /dmrgo:const -->.

A negative value in the config turns that timer off.

## Worked example

This example reads the rest channel beacons of a Linked Capacity Plus site from a recording: the time, the CACH and the burst of each line. The Follower reports the site with its rest LSN, holds the site number from the beacons, and hunts again one no-signal time after the last beacon.

<!-- dmrgo:code file=v2/trunk/example_test.go lines=167-211 -->
<!-- /dmrgo:code -->

Output:

<!-- dmrgo:output test=trunk.ExampleCapacityPlusFollower -->
<!-- /dmrgo:output -->

## Errors

The event methods return no errors. NewCapacityPlusFollower returns a configuration error for a talkgroup list out of range.

<!-- dmrgo:errors pkg=trunk type=CapacityPlusFollower -->
<!-- /dmrgo:errors -->

## Related machines and guides

Machines:

- [trunk Follower](trunk-follower.md): the same job for ETSI Tier III sites.
- [voice Receiver](voice-receiver.md): decodes the voice of a followed call.

Guides:

- Build and decode trunking messages
- Follow calls on both slots of a bit stream
- Receive with an SDR or an FM radio
