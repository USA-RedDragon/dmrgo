package nmea_test

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/nmea"
)

func ExampleParseRMC() {
	r, err := nmea.ParseRMC([]byte("$GPRMC,200037.000,A,3412.7300,N,10850.0000,E,0.00,121.00,021225,,,A*60\r\n"))
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("%d-%02d-%02d %02d:%02d:%02d valid=%t %.5f %.5f %.1f kn %.0f deg\n",
		r.Year, r.Month, r.Day, r.Hour, r.Minute, r.Second, r.Valid, r.Latitude, r.Longitude, r.Speed, r.Course)
	_, err = nmea.ParseRMC([]byte("$GPRMC,200037.000,A,3412.7300,N,10850.0000,E,0.00,121.00,021225,,,A*61\r\n"))
	fmt.Println(err)
	// Output:
	// 2025-12-02 20:00:37 valid=true 34.21217 108.83333 0.0 kn 121 deg
	// NMEA checksum is missing or does not match
}
