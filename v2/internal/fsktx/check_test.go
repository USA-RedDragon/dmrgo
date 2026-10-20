package fsktx

import "github.com/USA-RedDragon/dmrgo/v2/rf"

func check(_ []rf.Burst, err error) {
	if err != nil {
		panic(err)
	}
}
