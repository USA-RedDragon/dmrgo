//go:build tinygo

package rf

import "github.com/USA-RedDragon/dmrgo/v2/layer2"

type slcHook struct{}

func (s *burstSink) attachShortLC(on func([]ShortLC), _ Tier, _ bool, err error) error {
	if on != nil {
		return err
	}
	return nil
}

func (s *burstSink) noteLean(*leanCore, *Burst) {}

func (s *burstSink) noteLLR(float64, [3]byte, *[layer2.CACHBits]int16) {}

func (s *burstSink) noteCoh(float64, [3]byte, *[layer2.CACHBits]int16) {}

func (s *burstSink) seeShortLC(*Burst) { _ = s.slcHook }
