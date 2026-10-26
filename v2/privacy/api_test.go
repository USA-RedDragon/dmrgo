package privacy_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/privacy"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func TestAlgorithmString(t *testing.T) {
	t.Parallel()

	for a, want := range map[privacy.Algorithm]string{
		privacy.AlgorithmBasic: "Basic", privacy.AlgorithmARC4: "ARC4", privacy.AlgorithmDES: "DES",
		privacy.AlgorithmAES128: "AES-128", privacy.AlgorithmAES256: "AES-256", 0x68: "Algorithm(0x68)",
	} {
		if got := a.String(); got != want {
			t.Errorf("%d: %q, want %q", uint8(a), got, want)
		}
	}
}

func TestKeys(t *testing.T) {
	t.Parallel()

	for _, n := range []int{0, 5, 15, 17, 24, 31, 33} {
		if _, err := privacy.NewAESKey(1, make([]byte, n)); !errors.As(err, new(*dmrgo.ConfigError)) {
			t.Errorf("NewAESKey of %d octets: %v", n, err)
		}
	}
	for n, alg := range map[int]privacy.Algorithm{16: privacy.AlgorithmAES128, 32: privacy.AlgorithmAES256} {
		k, err := privacy.NewAESKey(7, make([]byte, n))
		if err != nil || k.Algorithm() != alg || k.ID() != 7 {
			t.Errorf("NewAESKey of %d octets: %v, %v", n, k, err)
		}
	}
	var nilKey *privacy.Key
	if nilKey.Algorithm() != privacy.AlgorithmBasic || nilKey.ID() != 0 {
		t.Error("nil Key")
	}
	if k := privacy.NewBasicKey(1); k.Algorithm() != privacy.AlgorithmBasic || k.ID() != 0 {
		t.Errorf("NewBasicKey: %v %d", k.Algorithm(), k.ID())
	}
}

func TestReceiverKeys(t *testing.T) {
	t.Parallel()

	r := privacy.NewReceiver()
	if err := r.AddKey(nil); !errors.Is(err, dmrgo.ErrNilArgument) {
		t.Errorf("AddKey(nil): %v", err)
	}
	for i := range privacy.MaxKeys {
		if err := r.AddKey(privacy.NewARC4Key(uint8(i), [5]byte{})); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.AddKey(privacy.NewARC4Key(3, [5]byte{1})); err != nil {
		t.Errorf("replacing a key: %v", err)
	}
	if err := r.AddKey(privacy.NewBasicKey(9)); err != nil {
		t.Errorf("a Basic key: %v", err)
	}
	if err := r.AddKey(privacy.NewARC4Key(privacy.MaxKeys, [5]byte{})); !errors.Is(err, privacy.ErrKeysFull) {
		t.Errorf("one key too many: %v", err)
	}
	var nr *privacy.Receiver
	if err := nr.AddKey(privacy.NewBasicKey(1)); !errors.Is(err, dmrgo.ErrNilReceiver) {
		t.Errorf("nil Receiver: %v", err)
	}
	nr.Reset()
	if nr.Info() != (privacy.Info{}) || nr.Event(&voice.Event{}, nil) != (privacy.Info{}) {
		t.Error("nil Receiver Info")
	}
	if r.Event(nil, nil) != (privacy.Info{}) {
		t.Error("Event(nil)")
	}
}

func TestFromPIHeader(t *testing.T) {
	t.Parallel()

	if privacy.FromPIHeader(nil) != (privacy.Info{}) {
		t.Error("nil")
	}
	h := pdu.PIHeader{AlgorithmID: 0x02, MFID: 0x68, KeyID: 3, MessageIndicator: 0x01020304}
	want := privacy.Info{Encrypted: true, Header: true, MFID: 0x68, Algorithm: 0x02, KeyID: 3, MI: 0x01020304}
	if got := privacy.FromPIHeader(&h); got != want {
		t.Errorf("%+v, want %+v", got, want)
	}
	h.FEC.Uncorrectable = true
	if privacy.FromPIHeader(&h) != (privacy.Info{}) {
		t.Error("a PI header whose CRC failed")
	}
}

func TestTransmitterErrors(t *testing.T) {
	t.Parallel()

	if _, err := privacy.NewTransmitter(nil); !errors.Is(err, dmrgo.ErrNilArgument) {
		t.Errorf("nil config: %v", err)
	}
	if _, err := privacy.NewTransmitter(&privacy.TransmitterConfig{Service: privacy.ServiceLandMobile}); !errors.Is(err, dmrgo.ErrNilArgument) {
		t.Errorf("nil key: %v", err)
	}
	if _, err := privacy.NewTransmitter(&privacy.TransmitterConfig{Key: &privacy.Key{}, Service: privacy.ServiceLandMobile}); err != nil {
		t.Errorf("zero Key, a Basic key of 0: %v", err)
	}
	tx, err := privacy.NewTransmitter(&privacy.TransmitterConfig{Key: privacy.NewARC4Key(1, [5]byte{}), Service: privacy.ServiceLandMobile})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tx.PIHeader(1); ok {
		t.Error("PIHeader before Start")
	}
	if err := tx.Output(&voice.Output{}); !errors.Is(err, privacy.ErrNotStarted) {
		t.Errorf("Output before Start: %v", err)
	}
	tx.Start(5)
	if err := tx.Output(nil); !errors.Is(err, dmrgo.ErrNilArgument) {
		t.Errorf("Output(nil): %v", err)
	}
	if err := tx.Output(&voice.Output{Kind: voice.OutputEnded}); err != nil {
		t.Errorf("OutputEnded: %v", err)
	}
	h, ok := tx.PIHeader(9)
	if !ok || h.AlgorithmID != 0x21 || h.MFID != enums.FIDMotorola || h.KeyID != 1 || h.MessageIndicator != 5 || h.Destination != 9 {
		t.Errorf("PIHeader: %+v %v", h, ok)
	}
	var nt *privacy.Transmitter
	nt.Start(1)
	if _, ok := nt.PIHeader(1); ok {
		t.Error("nil Transmitter PIHeader")
	}
	if err := nt.Output(&voice.Output{}); !errors.Is(err, dmrgo.ErrNilReceiver) {
		t.Errorf("nil Transmitter: %v", err)
	}
}

func TestServiceString(t *testing.T) {
	t.Parallel()

	for s, want := range map[privacy.Service]string{
		privacy.ServiceUnknown: "Unknown", privacy.ServiceAmateur: "Amateur", privacy.ServiceLandMobile: "LandMobile",
		privacy.ServiceLicenseFree: "LicenseFree", 9: "Service(9)",
	} {
		if got := s.String(); got != want {
			t.Errorf("%d: %q, want %q", uint8(s), got, want)
		}
	}
}

func transmitKeys(t *testing.T) []*privacy.Key {
	t.Helper()
	k128, err := privacy.NewAESKey(1, make([]byte, 16))
	if err != nil {
		t.Fatal(err)
	}
	k256, err := privacy.NewAESKey(2, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	return []*privacy.Key{privacy.NewBasicKey(0x4562), privacy.NewARC4Key(3, [5]byte{1, 2, 3, 4, 5}), k128, k256}
}

func TestTransmitterRefusesAmateurService(t *testing.T) {
	t.Parallel()

	for _, k := range transmitKeys(t) {
		tx, err := privacy.NewTransmitter(&privacy.TransmitterConfig{Key: k, Service: privacy.ServiceAmateur})
		if !errors.Is(err, privacy.ErrAmateur) || tx != nil {
			t.Errorf("%v key in the amateur service: made %v, %v", k.Algorithm(), tx != nil, err)
		}
		if _, err := privacy.NewTransmitter(&privacy.TransmitterConfig{Service: privacy.ServiceAmateur}); !errors.Is(err, privacy.ErrAmateur) {
			t.Errorf("amateur service without a key: %v", err)
		}
		for _, s := range []privacy.Service{privacy.ServiceUnknown, privacy.ServiceLicenseFree + 1, 255} {
			tx, err := privacy.NewTransmitter(&privacy.TransmitterConfig{Key: k, Service: s})
			if !errors.As(err, new(*dmrgo.ConfigError)) || tx != nil {
				t.Errorf("%v key in service %v: made %v, %v", k.Algorithm(), s, tx != nil, err)
			}
		}
	}
}

func TestTransmitterAcceptsOtherServices(t *testing.T) {
	t.Parallel()

	for _, k := range transmitKeys(t) {
		for _, s := range []privacy.Service{privacy.ServiceLandMobile, privacy.ServiceLicenseFree} {
			tx, err := privacy.NewTransmitter(&privacy.TransmitterConfig{Key: k, Service: s})
			if err != nil || tx == nil {
				t.Errorf("%v key in service %v: %v", k.Algorithm(), s, err)
				continue
			}
			tx.Start(1)
			if err := tx.Output(&voice.Output{}); err != nil {
				t.Errorf("%v key in service %v: Output: %v", k.Algorithm(), s, err)
			}
		}
	}
}
