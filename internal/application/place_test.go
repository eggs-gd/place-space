package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/sources"
	"github.com/eggs-gd/place-space/internal/storage/sqlite"
)

// Two districts a few kilometres apart, as LUN files its listings.
var (
	darnytskyi = [2]float64{50.395, 30.616}
	obolonskyi = [2]float64{50.501, 30.498}
)

type geoListing struct {
	id       string
	lat, lng *float64
	district string
}

// geoAdapter serves fixed listings. Its listings carry LUN-style area links in
// the payload so that a "lun" instance seeds the place index.
type geoAdapter struct {
	kind     string
	near     bool
	listings []geoListing
	located  map[string][2]float64
	queries  []domain.Query
	locates  int
	blocked  bool
}

func (a *geoAdapter) Type() string { return a.kind }

func (a *geoAdapter) Fetch(_ context.Context, q domain.Query) ([]sources.RawListing, error) {
	a.queries = append(a.queries, q)
	out := []sources.RawListing{}
	for _, l := range a.listings {
		payload, _ := json.Marshal(map[string]any{"geoEntities": []map[string]any{
			{"type": "district", "realtyGeoUrl": map[string]string{"link": "/rent/kyiv/houses-" + l.district + "-district"}},
		}})
		out = append(out, sources.RawListing{ExternalID: l.id, Payload: payload})
	}
	return out, nil
}

func (a *geoAdapter) Normalize(_ context.Context, raw sources.RawListing) (domain.Listing, error) {
	for _, l := range a.listings {
		if l.id == raw.ExternalID {
			price := int64(10000)
			return domain.Listing{
				Source: a.kind, ExternalID: l.id, URL: "https://example.test/" + a.kind + "/" + l.id,
				Title: l.id, PriceAmount: &price, Currency: "UAH",
				Location: domain.Location{Latitude: l.lat, Longitude: l.lng},
				RawData:  raw.Payload,
			}, nil
		}
	}
	return domain.Listing{}, fmt.Errorf("unknown %s", raw.ExternalID)
}

func (a *geoAdapter) Locate(_ context.Context, listing domain.Listing) (float64, float64, error) {
	a.locates++
	if a.blocked {
		return 0, 0, fmt.Errorf("status 429: %w", sources.ErrBlocked)
	}
	point, ok := a.located[listing.ExternalID]
	if !ok {
		return 0, 0, fmt.Errorf("no coordinates")
	}
	return point[0], point[1], nil
}

type nearGeoAdapter struct{ *geoAdapter }

func (nearGeoAdapter) SearchesNear() {}

func at(point [2]float64, dLat float64) (*float64, *float64) {
	lat, lng := point[0]+dLat, point[1]
	return &lat, &lng
}

// seedLUN stores LUN listings around both districts and the directory rows
// that turn their links into areas.
func seedLUN(t *testing.T, store *sqlite.Store) {
	t.Helper()
	ctx := context.Background()
	for _, kind := range []string{"lun", "domria", "otodim"} {
		if _, err := store.EnsureSource(ctx, domain.Source{ID: kind, Type: kind, Enabled: true, Status: domain.SourceUnknown}); err != nil {
			t.Fatal(err)
		}
	}
	areas := []domain.Area{
		{Key: "дарницький", Name: "Дарницький", Kind: domain.AreaDistrict, Path: "/rent/kyiv/flats-darnytskyi-district"},
		{Key: "оболонський", Name: "Оболонський", Kind: domain.AreaDistrict, Path: "/rent/kyiv/flats-obolonskyi-district"},
	}
	if err := store.MergeAreas(ctx, "lun", "київ", areas, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := store.MergeAreas(ctx, "domria", "київ", []domain.Area{
		{Key: "дарницький", Name: "Дарницький", Kind: domain.AreaDistrict, Path: "/uk/arenda-kvartir/kiev-rayon-darnytskyi/"},
		{Key: "оболонський", Name: "Оболонський", Kind: domain.AreaDistrict, Path: "/uk/arenda-kvartir/kiev-rayon-obolonskyi/"},
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	lun := &geoAdapter{kind: "lun"}
	for i := 0; i < 5; i++ {
		d := float64(i) * 0.001
		lat, lng := at(darnytskyi, d)
		lun.listings = append(lun.listings, geoListing{id: fmt.Sprintf("d%d", i), lat: lat, lng: lng, district: "darnytskyi"})
		lat, lng = at(obolonskyi, d)
		lun.listings = append(lun.listings, geoListing{id: fmt.Sprintf("o%d", i), lat: lat, lng: lng, district: "obolonskyi"})
	}
	seedWatch, err := store.CreateWatch(ctx, domain.Watch{Name: "seed", Enabled: true, SourceIDs: []string{"lun"},
		Query: domain.Query{City: "Київ", Deal: domain.DealRent}, PollInterval: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	p := &Pipeline{Store: store, Adapters: map[string]sources.Adapter{"lun": lun}}
	if _, err := p.PollWatch(ctx, seedWatch); err != nil {
		t.Fatal(err)
	}
}

func decisions(report Report) map[string]domain.Decision {
	out := map[string]domain.Decision{}
	for _, item := range report.Items {
		out[item.Listing.ExternalID] = item.Decision
	}
	return out
}

func messages(d domain.Decision) string {
	var out []string
	for _, r := range d.Reasons {
		out = append(out, r.Message)
	}
	return strings.Join(out, "; ")
}

func TestAreasCheckSourceWithoutAreaPages(t *testing.T) {
	ctx := context.Background()
	store := openStore(t)
	seedLUN(t, store)
	otodim := &geoAdapter{kind: "otodim",
		listings: []geoListing{{id: "east"}, {id: "north"}, {id: "lost"}},
		located: map[string][2]float64{
			"east":  {darnytskyi[0] + 0.002, darnytskyi[1]},
			"north": {obolonskyi[0] + 0.002, obolonskyi[1]},
		},
	}
	p := &Pipeline{Store: store, Places: &PlaceIndex{Store: store},
		Adapters: map[string]sources.Adapter{"otodim": nearGeoAdapter{otodim}}}
	watch, err := store.CreateWatch(ctx, domain.Watch{Name: "Київ", Enabled: true, SourceIDs: []string{"otodim"},
		Query:        domain.Query{City: "Київ", Deal: domain.DealRent, Areas: []domain.AreaRef{{Key: "дарницький", Name: "Дарницький"}}},
		PollInterval: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	report, err := p.PollWatch(ctx, watch)
	if err != nil {
		t.Fatal(err)
	}
	got := decisions(report)
	if got["east"].Status != domain.MatchMatched {
		t.Fatalf("east: %s %s", got["east"].Status, messages(got["east"]))
	}
	if got["north"].Status != domain.MatchRejected || !strings.Contains(messages(got["north"]), "area: Оболонський") {
		t.Fatalf("north: %s %s", got["north"].Status, messages(got["north"]))
	}
	if got["lost"].Status != domain.MatchRejected || !strings.Contains(messages(got["lost"]), "area: unknown") {
		t.Fatalf("lost: %s %s", got["lost"].Status, messages(got["lost"]))
	}
	if len(otodim.queries) != 1 || len(otodim.queries[0].Areas) != 0 {
		t.Fatalf("otodim was asked for area pages: %+v", otodim.queries)
	}

	// Coordinates found once are kept: the next check reads no listing page
	// for them, and the stored point is not wiped by the catalog's empty one.
	before := otodim.locates
	if _, err := p.PollWatch(ctx, watch); err != nil {
		t.Fatal(err)
	}
	if otodim.locates-before != 1 {
		t.Fatalf("second check read %d listing pages, want 1 (only the one without coordinates)", otodim.locates-before)
	}
}

func TestNearChecksSourceWithoutDistanceSearch(t *testing.T) {
	ctx := context.Background()
	store := openStore(t)
	seedLUN(t, store)
	close1, close2 := at(darnytskyi, 0.004) // about 450 m north
	far1, far2 := at(darnytskyi, 0.03)      // about 3.3 km north
	domria := &geoAdapter{kind: "domria", listings: []geoListing{
		{id: "close", lat: close1, lng: close2}, {id: "far", lat: far1, lng: far2}, {id: "nowhere"},
	}}
	p := &Pipeline{Store: store, Places: &PlaceIndex{Store: store},
		Areas:    &AreaDirectory{Store: store},
		Adapters: map[string]sources.Adapter{"domria": domria}}
	lat, lng := darnytskyi[0], darnytskyi[1]
	watch, err := store.CreateWatch(ctx, domain.Watch{Name: "Київ", Enabled: true, SourceIDs: []string{"domria"},
		Query: domain.Query{City: "Київ", Deal: domain.DealRent,
			Near: &domain.Near{Name: "Осокорки", PlaceID: "ChIJtest", RadiusMeters: 1000, Latitude: &lat, Longitude: &lng}},
		PollInterval: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	report, err := p.PollWatch(ctx, watch)
	if err != nil {
		t.Fatal(err)
	}
	if len(domria.queries) != 1 || strings.Join(domria.queries[0].AreaPaths, ",") != "/uk/arenda-kvartir/kiev-rayon-darnytskyi/" {
		t.Fatalf("catalogs read: %+v", domria.queries)
	}
	got := decisions(report)
	if got["close"].Status != domain.MatchMatched {
		t.Fatalf("close: %s %s", got["close"].Status, messages(got["close"]))
	}
	if got["far"].Status != domain.MatchRejected || !strings.Contains(messages(got["far"]), "> 1.00") {
		t.Fatalf("far: %s %s", got["far"].Status, messages(got["far"]))
	}
	if got["nowhere"].Status != domain.MatchRejected || !strings.Contains(messages(got["nowhere"]), "distance: unknown") {
		t.Fatalf("nowhere: %s %s", got["nowhere"].Status, messages(got["nowhere"]))
	}
}

func TestDistanceReason(t *testing.T) {
	lat, lng := 50.0, 30.0
	near := domain.Near{RadiusMeters: 1000, Latitude: &lat, Longitude: &lng}
	inLat, outLat := 50.005, 50.02
	if r := domain.DistanceReason(near, domain.Listing{Location: domain.Location{Latitude: &inLat, Longitude: &lng}}); !r.Passed {
		t.Fatalf("inside: %+v", r)
	}
	if r := domain.DistanceReason(near, domain.Listing{Location: domain.Location{Latitude: &outLat, Longitude: &lng}}); r.Passed {
		t.Fatalf("outside: %+v", r)
	}
	if km := domain.DistanceKm(50.4501, 30.5234, 50.4547, 30.5238); km < 0.45 || km > 0.56 {
		t.Fatalf("distance %.3f km", km)
	}
}

func TestLocateSkipsListingsRejectedAnyway(t *testing.T) {
	ctx := context.Background()
	store := openStore(t)
	seedLUN(t, store)
	otodim := &geoAdapter{kind: "otodim", listings: []geoListing{{id: "dear"}},
		located: map[string][2]float64{"dear": darnytskyi}}
	p := &Pipeline{Store: store, Places: &PlaceIndex{Store: store},
		Adapters: map[string]sources.Adapter{"otodim": nearGeoAdapter{otodim}}}
	cheap := int64(5000)
	watch, err := store.CreateWatch(ctx, domain.Watch{Name: "Київ", Enabled: true, SourceIDs: []string{"otodim"},
		Query:        domain.Query{City: "Київ", Deal: domain.DealRent, Areas: []domain.AreaRef{{Key: "дарницький", Name: "Дарницький"}}},
		Filters:      domain.Filters{PriceMax: &cheap},
		PollInterval: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.PollWatch(ctx, watch); err != nil {
		t.Fatal(err)
	}
	if otodim.locates != 0 {
		t.Fatalf("read %d listing pages for a listing over the price limit", otodim.locates)
	}
}

func TestOtherWatchKeepsFoundCoordinates(t *testing.T) {
	ctx := context.Background()
	store := openStore(t)
	seedLUN(t, store)
	otodim := &geoAdapter{kind: "otodim", listings: []geoListing{{id: "east"}},
		located: map[string][2]float64{"east": darnytskyi}}
	p := &Pipeline{Store: store, Places: &PlaceIndex{Store: store},
		Adapters: map[string]sources.Adapter{"otodim": nearGeoAdapter{otodim}}}
	areaWatch, err := store.CreateWatch(ctx, domain.Watch{Name: "Дарницький", Enabled: true, SourceIDs: []string{"otodim"},
		Query:        domain.Query{City: "Київ", Deal: domain.DealRent, Areas: []domain.AreaRef{{Key: "дарницький", Name: "Дарницький"}}},
		PollInterval: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	cityWatch, err := store.CreateWatch(ctx, domain.Watch{Name: "Київ", Enabled: true, SourceIDs: []string{"otodim"},
		Query: domain.Query{City: "Київ", Deal: domain.DealRent}, PollInterval: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	for _, watch := range []domain.Watch{areaWatch, cityWatch, areaWatch} {
		if _, err := p.PollWatch(ctx, watch); err != nil {
			t.Fatal(err)
		}
	}
	if otodim.locates != 1 {
		t.Fatalf("listing page read %d times; a watch without areas wiped the coordinates", otodim.locates)
	}
	stored, err := store.ListingsByKeys(ctx, "otodim", []string{"east"}, nil)
	if err != nil || len(stored) != 1 || stored[0].Location.Latitude == nil {
		t.Fatalf("stored %+v err %v", stored, err)
	}
}

func TestRefusedListingPageRestsTheSource(t *testing.T) {
	ctx := context.Background()
	store := openStore(t)
	seedLUN(t, store)
	otodim := &geoAdapter{kind: "otodim", blocked: true, listings: []geoListing{{id: "a"}, {id: "b"}, {id: "c"}}}
	p := &Pipeline{Store: store, Places: &PlaceIndex{Store: store},
		Adapters: map[string]sources.Adapter{"otodim": nearGeoAdapter{otodim}}}
	watch, err := store.CreateWatch(ctx, domain.Watch{Name: "Київ", Enabled: true, SourceIDs: []string{"otodim"},
		Query:        domain.Query{City: "Київ", Deal: domain.DealRent, Areas: []domain.AreaRef{{Key: "дарницький", Name: "Дарницький"}}},
		PollInterval: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.PollWatch(ctx, watch); err != nil {
		t.Fatal(err)
	}
	if otodim.locates != 1 {
		t.Fatalf("kept reading after a refusal: %d requests", otodim.locates)
	}
	if !p.resting("otodim") {
		t.Fatal("refusal did not rest the source")
	}
}
