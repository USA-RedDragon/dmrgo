package hytera

// Valid reports a position from a fix: Status 'A'.
func (g *GPSData) Valid() bool {
	if g == nil {
		return false
	}
	return g.Status == 'A'
}

// Latitude returns the latitude in degrees, north positive.
func (g *GPSData) Latitude() float64 {
	if g == nil {
		return 0
	}
	v := float64(g.LatitudeDegrees) + (float64(g.LatitudeMinutes)+float64(g.LatitudeFraction)/10000)/60
	if g.NorthSouth == 'S' {
		return -v
	}
	return v
}

// Longitude returns the longitude in degrees, east positive.
func (g *GPSData) Longitude() float64 {
	if g == nil {
		return 0
	}
	v := float64(g.LongitudeDegrees) + (float64(g.LongitudeMinutes)+float64(g.LongitudeFraction)/10000)/60
	if g.EastWest == 'W' {
		return -v
	}
	return v
}
