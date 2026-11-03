//go:build !tinygo

package rf

import (
	"strings"
	"testing"
)

func TestRxReplayStretch(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"s0.none.+4000.ms.voice.+500ppm.nom.cs16-48k.inf",
		"s0.none.-4000.ms.voice.-500ppm.nom.cs16-48k.inf",
	} {
		row := mxRowOf(t, name)
		x := mxRx{row.val(mxFormat), Tier3}
		res, err := mxRun(row, []mxRx{x})
		if err != nil {
			t.Fatal(err)
		}
		if r := res[x]; len(r.fail) > 0 {
			t.Errorf("%s/%s: %s", name, x.name(), strings.Join(r.detail, " | "))
		}
	}
}
