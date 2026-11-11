package application

import (
	"context"
	"sort"

	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/storage/sqlite"
)

const (
	// A point takes the areas of its nearest LUN listings: enough of them to
	// outvote a stray one, close enough to stay on the right side of a border.
	areaNeighbours  = 9
	districtReachKm = 1.5
	microReachKm    = 0.8
	minVotes        = 2
	// Districts within this margin of a circle may hold listings inside it.
	coverMarginKm = 0.5
	coverLimit    = 2000
)

// PlaceIndex maps points to the city's areas using LUN listings, which name
// their own district and microdistrict.
type PlaceIndex struct {
	Store *sqlite.Store
}

// AreasAt returns the district and microdistrict a point most likely belongs
// to. Either is empty when the nearby listings do not agree enough.
func (x *PlaceIndex) AreasAt(ctx context.Context, city string, lat, lng float64) (district, micro string, err error) {
	near, err := x.Store.AreaNeighbours(ctx, domain.AreaKey(city), lat, lng, areaNeighbours, districtReachKm)
	if err != nil {
		return "", "", err
	}
	districts := map[string]int{}
	micros := map[string]int{}
	for _, n := range near {
		if n.District != "" {
			districts[n.District]++
		}
		if n.Microdistrict != "" && n.DistanceKm <= microReachKm {
			micros[n.Microdistrict]++
		}
	}
	return winner(districts), winner(micros), nil
}

// DistrictsAround returns the districts that LUN listings place within the
// circle, with a margin.
func (x *PlaceIndex) DistrictsAround(ctx context.Context, city string, lat, lng, radiusKm float64) ([]string, error) {
	near, err := x.Store.AreaNeighbours(ctx, domain.AreaKey(city), lat, lng, coverLimit, radiusKm+coverMarginKm)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, n := range near {
		if n.District != "" && !seen[n.District] {
			seen[n.District] = true
			out = append(out, n.District)
		}
	}
	sort.Strings(out)
	return out, nil
}

func winner(votes map[string]int) string {
	best, count := "", 0
	for key, n := range votes {
		if n > count || (n == count && key < best) {
			best, count = key, n
		}
	}
	if count < minVotes {
		return ""
	}
	return best
}
