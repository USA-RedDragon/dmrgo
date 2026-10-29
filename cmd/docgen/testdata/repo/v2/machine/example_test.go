package machine_test

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/machine"
)

func ExampleLamp() {
	l, _ := machine.NewLamp(machine.Config{})
	_ = l.Start(0)
	fmt.Println(l.State() == machine.LampOn)
	// Output: true
}
