//go:build tinygo

package rf

import "github.com/USA-RedDragon/dmrgo/v2/enums"

func (r *leanCore) dueLock() bool { return false }

func (r *leanCore) wantRetune(enums.SyncPattern, int, bool) bool { return false }

func (x *softExt) lockTune(*leanCore, enums.SyncPattern, int, bool) bool { return false }
