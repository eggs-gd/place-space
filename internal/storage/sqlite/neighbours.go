package sqlite

import (
	"context"
	"fmt"
	"math"
)

// Neighbour is a LUN listing near a point and the areas LUN files it under.
type Neighbour struct {
	DistanceKm    float64
	District      string
	Microdistrict string
}

// lunAreaLinks joins a LUN listing's own district and microdistrict links to
// the area directory. LUN links houses and flats to twin catalog pages; the
// directory keeps the flats one.
const lunAreaLinks = `
	SELECT near.id, near.d2, json_extract(g.value, '$.type'), a.area_key
	FROM near
	LEFT JOIN json_each(near.raw_data, '$.geoEntities') g
		ON json_extract(g.value, '$.type') IN ('district', 'microdistrict')
	LEFT JOIN areas a ON a.source_type = 'lun' AND a.city_key = ?
		AND a.path = replace(json_extract(g.value, '$.realtyGeoUrl.link'), '/houses-', '/flats-')
	ORDER BY near.d2, near.id`

// AreaNeighbours returns up to limit LUN listings within maxKm of the point,
// nearest first, with the areas they belong to. LUN listings name their own
// district and microdistrict, so together they map points to areas.
func (s *Store) AreaNeighbours(ctx context.Context, cityKey string, lat, lng float64, limit int, maxKm float64) ([]Neighbour, error) {
	kmPerLng := 111.32 * math.Cos(lat*math.Pi/180)
	rows, err := s.db.QueryContext(ctx, `WITH near AS (
		SELECT id, raw_data, d2 FROM (
			SELECT id, raw_data,
				((latitude - ?) * 110.57) * ((latitude - ?) * 110.57) +
				((longitude - ?) * ?) * ((longitude - ?) * ?) AS d2
			FROM listings WHERE source = 'lun' AND latitude IS NOT NULL AND longitude IS NOT NULL
		) WHERE d2 <= ? ORDER BY d2 LIMIT ?
	)`+lunAreaLinks,
		lat, lat, lng, kmPerLng, lng, kmPerLng, maxKm*maxKm, limit, cityKey)
	if err != nil {
		return nil, fmt.Errorf("area neighbours: %w", err)
	}
	defer rows.Close()
	var (
		out  []Neighbour
		last string
	)
	for rows.Next() {
		var (
			id      string
			d2      float64
			kind    *string
			areaKey *string
		)
		if err := rows.Scan(&id, &d2, &kind, &areaKey); err != nil {
			return nil, err
		}
		if id != last {
			out = append(out, Neighbour{DistanceKm: math.Sqrt(d2)})
			last = id
		}
		if kind == nil || areaKey == nil {
			continue
		}
		current := &out[len(out)-1]
		switch *kind {
		case "district":
			current.District = *areaKey
		case "microdistrict":
			current.Microdistrict = *areaKey
		}
	}
	return out, rows.Err()
}
