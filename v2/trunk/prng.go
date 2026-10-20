package trunk

import "github.com/USA-RedDragon/dmrgo/v2/internal/pcg"

func between(p *pcg.Source, n int) int {
	if n <= 1 {
		return 1
	}
	return 1 + int(uint64(p.Uint32())*uint64(n)>>32)
}
