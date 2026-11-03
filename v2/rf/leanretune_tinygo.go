//go:build tinygo

package rf

import "github.com/USA-RedDragon/dmrgo/v2/enums"

type lockRetune struct{}

func (lockRetune) wanted() bool { return false }

func (r *leanCore) dueLock() bool { return false }

func (r *leanCore) wantRetune(enums.SyncPattern, int, bool) bool { return false }

func (x *softExt) lockTune(*leanCore, enums.SyncPattern, int, bool) bool { return x.rt.wanted() }
