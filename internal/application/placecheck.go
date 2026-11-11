package application

import (
	"context"
	"errors"
	"time"

	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/sources"
)

// maxLocatePerPoll caps listing pages read for coordinates in one check. Only
// listings without known coordinates are read, so the cap is reached only on
// a first check; the rest are read on the next ones.
const maxLocatePerPoll = 25

// placeCheck is what is checked per listing after a source's catalog: its
// area, its distance from a place, or both.
type placeCheck struct {
	city  string
	areas map[string]bool
	// names shows areas by name in reasons.
	names map[string]string
	near  *domain.Near
}

func (c placeCheck) needsPoint() bool {
	return c.areas != nil || c.near != nil
}

func areaSet(refs []domain.AreaRef) map[string]bool {
	out := make(map[string]bool, len(refs))
	for _, ref := range refs {
		out[ref.Key] = true
	}
	return out
}

// locate returns coordinates for listings whose catalog omitted them. Stored
// ones are always returned: the listing row is shared by every watch, and a
// catalog without coordinates would otherwise wipe what another watch found.
// Listing pages are read only when this watch checks place, and only for
// listings that pass the other limits: the rest are rejected wherever they
// are. A refusal stops the reading and rests the source.
func (p *Pipeline) locate(ctx context.Context, check placeCheck, source domain.Source, adapter sources.Adapter, filters domain.Filters, rates domain.Rates, listings []domain.Listing, existing []domain.Listing) map[string][2]float64 {
	points := map[string][2]float64{}
	for _, listing := range existing {
		if listing.Location.Latitude != nil && listing.Location.Longitude != nil {
			points[listing.ExternalID] = [2]float64{*listing.Location.Latitude, *listing.Location.Longitude}
		}
	}
	if !check.needsPoint() {
		return points
	}
	locator, ok := adapter.(sources.Locator)
	if !ok {
		return points
	}
	read := 0
	for _, listing := range listings {
		if listing.Location.Latitude != nil {
			continue
		}
		if _, ok := points[listing.ExternalID]; ok {
			continue
		}
		if domain.Apply(filters, listing, rates).Status != domain.MatchMatched {
			continue
		}
		if read >= maxLocatePerPoll || ctx.Err() != nil {
			break
		}
		if read > 0 {
			timer := time.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return points
			case <-timer.C:
			}
		}
		read++
		lat, lng, err := locator.Locate(ctx, listing)
		if errors.Is(err, sources.ErrBlocked) {
			p.log().Warn("locate listing refused", "source", source.ID, "err", err)
			p.rest(source.ID)
			return points
		}
		if err != nil {
			p.log().Warn("locate listing", "source", source.ID, "listing", listing.ExternalID, "err", err)
			continue
		}
		points[listing.ExternalID] = [2]float64{lat, lng}
	}
	return points
}

// placeReasons checks one listing's area and distance.
func (p *Pipeline) placeReasons(ctx context.Context, check placeCheck, listing domain.Listing) []domain.Reason {
	var reasons []domain.Reason
	if check.near != nil {
		reasons = append(reasons, domain.DistanceReason(*check.near, listing))
	}
	if check.areas != nil {
		reasons = append(reasons, p.areaReason(ctx, check, listing))
	}
	return reasons
}

func (p *Pipeline) areaReason(ctx context.Context, check placeCheck, listing domain.Listing) domain.Reason {
	unknown := domain.Reason{Field: "place", Message: "area: unknown"}
	if p.Places == nil || listing.Location.Latitude == nil || listing.Location.Longitude == nil {
		return unknown
	}
	district, micro, err := p.Places.AreasAt(ctx, check.city, *listing.Location.Latitude, *listing.Location.Longitude)
	if err != nil {
		p.log().Warn("area of listing", "listing", listing.ExternalID, "err", err)
		return unknown
	}
	if district == "" && micro == "" {
		return unknown
	}
	for _, key := range []string{micro, district} {
		if key != "" && check.areas[key] {
			return domain.Reason{Field: "place", Passed: true, Message: "area: " + check.name(key)}
		}
	}
	shown := micro
	if shown == "" {
		shown = district
	}
	return domain.Reason{Field: "place", Message: "area: " + check.name(shown)}
}

func (c placeCheck) name(key string) string {
	if name := c.names[key]; name != "" {
		return name
	}
	return key
}

// areaNames maps the city's area keys to the names people see.
func (p *Pipeline) areaNames(ctx context.Context, city string) map[string]string {
	names := map[string]string{}
	rows, err := p.Store.AreasForCity(ctx, domain.AreaKey(city))
	if err != nil {
		p.log().Warn("area names", "city", city, "err", err)
		return names
	}
	for _, row := range rows {
		if names[row.Key] == "" {
			names[row.Key] = row.Name
		}
	}
	return names
}
