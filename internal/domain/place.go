package domain

import (
	"fmt"
	"math"
)

const earthRadiusKm = 6371.0

// DistanceKm is the great-circle distance between two points.
func DistanceKm(lat1, lng1, lat2, lng2 float64) float64 {
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLng := (lng2 - lng1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthRadiusKm * math.Asin(math.Min(1, math.Sqrt(a)))
}

// HasPoint reports whether the place's point is known.
func (n *Near) HasPoint() bool {
	return n != nil && n.Latitude != nil && n.Longitude != nil
}

// DistanceReason checks a listing against the circle around the place.
func DistanceReason(near Near, listing Listing) Reason {
	limit := float64(near.RadiusMeters) / 1000
	if !near.HasPoint() || listing.Location.Latitude == nil || listing.Location.Longitude == nil {
		return Reason{Field: "distance", Message: "distance: unknown"}
	}
	km := DistanceKm(*near.Latitude, *near.Longitude, *listing.Location.Latitude, *listing.Location.Longitude)
	if km <= limit {
		return Reason{Field: "distance", Passed: true, Message: fmt.Sprintf("distance: %.2f <= %.2f", km, limit)}
	}
	return Reason{Field: "distance", Message: fmt.Sprintf("distance: %.2f > %.2f", km, limit)}
}

// With adds reasons to the decision. Any failed reason rejects the listing.
func (d Decision) With(reasons ...Reason) Decision {
	d.Reasons = append(append([]Reason{}, d.Reasons...), reasons...)
	for _, reason := range reasons {
		if !reason.Passed {
			d.Status = MatchRejected
		}
	}
	return d
}
