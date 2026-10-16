// Package copies copies each library type that must not be copied after
// first use, so that TestVetReportsCopies can check that go vet reports
// every copy.
package copies

import (
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

// Copies copies a Burst, an Assembler, a Builder and a Receiver.
func Copies(b *layer2.Burst, a *packet.Assembler, bu *packet.Builder, r *voice.Receiver) {
	burst := *b
	assembler := *a
	builder := *bu
	receiver := *r
	_, _, _, _ = burst, assembler, builder, receiver
}
