package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/sources"
	"github.com/eggs-gd/place-space/internal/storage/sqlite"
)

// stubAdapter records what it was asked for.
type stubAdapter struct {
	kind     string
	areas    []domain.Area
	areasErr error
	reads    int
	fetchErr error
	fetched  []domain.Query
}

func (s *stubAdapter) Type() string { return s.kind }

func (s *stubAdapter) Areas(context.Context, string) ([]domain.Area, error) {
	s.reads++
	return s.areas, s.areasErr
}

func (s *stubAdapter) Fetch(_ context.Context, query domain.Query) ([]sources.RawListing, error) {
	s.fetched = append(s.fetched, query)
	return nil, s.fetchErr
}

func (s *stubAdapter) Normalize(context.Context, sources.RawListing) (domain.Listing, error) {
	return domain.Listing{}, fmt.Errorf("unused")
}

func area(name, kind, path string) domain.Area {
	return domain.Area{Key: domain.AreaKey(name), Name: name, Kind: kind, Path: path}
}

func directoryFixture(t *testing.T) (*AreaDirectory, *stubAdapter, *stubAdapter, *time.Time) {
	t.Helper()
	lunStub := &stubAdapter{kind: "lun", areas: []domain.Area{
		area("Дарницький", domain.AreaDistrict, "/rent/kyiv/flats-darnytskyi-district"),
		area("Осокорки", domain.AreaMicrodistrict, "/rent/kyiv/flats-osokorky"),
	}}
	domStub := &stubAdapter{kind: "domria", areas: []domain.Area{
		area("Дарницький", domain.AreaMicrodistrict, "/uk/arenda-kvartir/kiev-rayon-darnytskyi/"),
		area("Троєщина", domain.AreaMicrodistrict, "/uk/arenda-kvartir/kiev-rayon-troeshchyna/"),
	}}
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	directory := &AreaDirectory{
		Store:    openStore(t),
		Adapters: map[string]sources.Adapter{"lun": lunStub, "domria": domStub},
		Now:      func() time.Time { return now },
	}
	return directory, lunStub, domStub, &now
}

func TestDirectoryMergesSourcesByName(t *testing.T) {
	directory, _, _, _ := directoryFixture(t)
	options, err := directory.List(context.Background(), "Київ")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]AreaOption{}
	for _, option := range options {
		got[option.Key] = option
	}
	if len(options) != 3 {
		t.Fatalf("options %+v", options)
	}
	darnytskyi := got["дарницький"]
	if darnytskyi.Kind != domain.AreaDistrict || strings.Join(darnytskyi.Sources, ",") != "domria,lun" {
		t.Fatalf("merged %+v", darnytskyi)
	}
	if strings.Join(got["троєщина"].Sources, ",") != "domria" {
		t.Fatalf("single source %+v", got["троєщина"])
	}
}

func TestDirectoryNestsOnlyUnderRealDistricts(t *testing.T) {
	directory, lunStub, domStub, _ := directoryFixture(t)
	nested := area("Осокорки", domain.AreaMicrodistrict, "/rent/kyiv/flats-osokorky")
	nested.Parent = "дарницький"
	lunStub.areas[1] = nested
	stray := area("Троєщина", domain.AreaMicrodistrict, "/uk/arenda-kvartir/kiev-rayon-troeshchyna/")
	stray.Parent = "десь"
	domStub.areas[1] = stray
	options, err := directory.List(context.Background(), "Київ")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, option := range options {
		got[option.Key] = option.Parent
	}
	if got["осокорки"] != "дарницький" || got["троєщина"] != "" || got["дарницький"] != "" {
		t.Fatalf("parents %v", got)
	}
}

func TestDirectoryKeepsParentWhenLaterScanHasNone(t *testing.T) {
	directory, lunStub, _, now := directoryFixture(t)
	ctx := context.Background()
	nested := area("Осокорки", domain.AreaMicrodistrict, "/rent/kyiv/flats-osokorky")
	nested.Parent = "дарницький"
	lunStub.areas[1] = nested
	if _, err := directory.List(ctx, "Київ"); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(8 * 24 * time.Hour)
	lunStub.areas[1].Parent = ""
	options, err := directory.List(ctx, "Київ")
	if err != nil {
		t.Fatal(err)
	}
	for _, option := range options {
		if option.Key == "осокорки" && option.Parent != "дарницький" {
			t.Fatalf("parent lost: %+v", option)
		}
	}
}

func TestDirectoryRereadsOnlyWhenOld(t *testing.T) {
	directory, lunStub, _, now := directoryFixture(t)
	ctx := context.Background()
	for range 2 {
		if _, err := directory.List(ctx, "Київ"); err != nil {
			t.Fatal(err)
		}
	}
	if lunStub.reads != 1 {
		t.Fatalf("reads %d", lunStub.reads)
	}
	*now = now.Add(8 * 24 * time.Hour)
	if _, err := directory.List(ctx, "Київ"); err != nil {
		t.Fatal(err)
	}
	if lunStub.reads != 2 {
		t.Fatalf("reads after a week %d", lunStub.reads)
	}
}

func TestDirectoryRemembersCitiesWithoutAreas(t *testing.T) {
	directory, lunStub, domStub, _ := directoryFixture(t)
	lunStub.areas, domStub.areas = nil, nil
	ctx := context.Background()
	for range 2 {
		options, err := directory.List(ctx, "Кам'янець-Подільський")
		if err != nil || len(options) != 0 {
			t.Fatalf("options %+v err %v", options, err)
		}
	}
	if lunStub.reads != 1 {
		t.Fatalf("reads %d", lunStub.reads)
	}
}

func TestDirectoryKeepsSavedAreasWhenRereadFails(t *testing.T) {
	directory, lunStub, domStub, now := directoryFixture(t)
	ctx := context.Background()
	if _, err := directory.List(ctx, "Київ"); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(8 * 24 * time.Hour)
	lunStub.areasErr, domStub.areasErr = errors.New("down"), errors.New("down")
	options, err := directory.List(ctx, "Київ")
	if err != nil || len(options) != 3 {
		t.Fatalf("options %+v err %v", options, err)
	}
}

func TestDirectoryKeepsAreasALaterScanDidNotSee(t *testing.T) {
	directory, lunStub, domStub, now := directoryFixture(t)
	ctx := context.Background()
	if _, err := directory.List(ctx, "Київ"); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(8 * 24 * time.Hour)
	lunStub.areas = lunStub.areas[:1]
	domStub.areas = domStub.areas[:1]
	paths, err := directory.Paths(ctx, "lun", "Київ", []domain.AreaRef{{Key: "осокорки"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(paths, ",") != "/rent/kyiv/flats-osokorky" {
		t.Fatalf("paths %v", paths)
	}
}

func TestDirectoryRetriesIncompleteScanSoon(t *testing.T) {
	directory, lunStub, _, now := directoryFixture(t)
	ctx := context.Background()
	lunStub.areasErr = fmt.Errorf("lun: %w", sources.ErrIncompleteAreas)
	options, err := directory.List(ctx, "Київ")
	if err != nil || len(options) != 3 {
		t.Fatalf("options %+v err %v", options, err)
	}
	if _, err := directory.List(ctx, "Київ"); err != nil {
		t.Fatal(err)
	}
	if lunStub.reads != 1 {
		t.Fatalf("reread at once: %d", lunStub.reads)
	}
	*now = now.Add(areaScanRetry + time.Minute)
	lunStub.areasErr = nil
	if _, err := directory.List(ctx, "Київ"); err != nil {
		t.Fatal(err)
	}
	if lunStub.reads != 2 {
		t.Fatalf("not retried: %d", lunStub.reads)
	}
}

func TestDirectoryFailsWhenNothingIsKnown(t *testing.T) {
	directory, lunStub, domStub, _ := directoryFixture(t)
	lunStub.areasErr, domStub.areasErr = errors.New("down"), errors.New("down")
	if _, err := directory.List(context.Background(), "Київ"); err == nil {
		t.Fatal("no error")
	}
}

func TestResolveChecksAreas(t *testing.T) {
	directory, _, _, _ := directoryFixture(t)
	ctx := context.Background()
	refs, err := directory.Resolve(ctx, "Київ", []string{"Дарницький район", "дарницький", "Осокорки"})
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 || refs[0] != (domain.AreaRef{Key: "дарницький", Name: "Дарницький"}) {
		t.Fatalf("refs %+v", refs)
	}
	var mistake *AreaError
	if _, err := directory.Resolve(ctx, "Київ", []string{"Марс"}); !errors.As(err, &mistake) {
		t.Fatalf("unknown area: %v", err)
	}
}

func TestResolveLimitsAreas(t *testing.T) {
	directory, lunStub, _, _ := directoryFixture(t)
	lunStub.areas = nil
	names := []string{"а", "б", "в", "г", "д", "е"}
	for _, name := range names {
		lunStub.areas = append(lunStub.areas, area(name, domain.AreaMicrodistrict, "/rent/kyiv/flats-"+name))
	}
	var mistake *AreaError
	if _, err := directory.Resolve(context.Background(), "Київ", names); !errors.As(err, &mistake) {
		t.Fatalf("err %v", err)
	}
}

func pollFixture(t *testing.T) (*Pipeline, *stubAdapter, *stubAdapter, domain.Watch, *sqlite.Store, *time.Time) {
	t.Helper()
	directory, lunStub, domStub, now := directoryFixture(t)
	ctx := context.Background()
	for _, id := range []string{"lun", "domria"} {
		if _, err := directory.Store.EnsureSource(ctx, domain.Source{
			ID: id, Type: id, Enabled: true, Status: domain.SourceUnknown, Configuration: json.RawMessage(`{}`),
		}); err != nil {
			t.Fatal(err)
		}
	}
	watch, err := directory.Store.CreateWatch(ctx, domain.Watch{
		Name: "Київ", Enabled: true, SourceIDs: []string{"lun", "domria"},
		Query:        domain.Query{City: "Київ", Deal: domain.DealRent, Areas: []domain.AreaRef{{Key: "осокорки", Name: "Осокорки"}}},
		PollInterval: 10 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	pipeline := &Pipeline{Store: directory.Store, Adapters: directory.Adapters, Areas: directory, Now: directory.Now}
	return pipeline, lunStub, domStub, watch, directory.Store, now
}

func TestPollSkipsSourceWithoutTheChosenArea(t *testing.T) {
	pipeline, lunStub, domStub, watch, _, _ := pollFixture(t)
	if _, err := pipeline.PollWatch(context.Background(), watch); err != nil {
		t.Fatal(err)
	}
	if len(lunStub.fetched) != 1 || strings.Join(lunStub.fetched[0].AreaPaths, ",") != "/rent/kyiv/flats-osokorky" {
		t.Fatalf("lun %+v", lunStub.fetched)
	}
	if len(domStub.fetched) != 0 {
		t.Fatalf("whole city was read: %+v", domStub.fetched)
	}
}

func TestBlockedSourceRestsThenReturns(t *testing.T) {
	pipeline, lunStub, _, watch, _, now := pollFixture(t)
	ctx := context.Background()
	lunStub.fetchErr = fmt.Errorf("lun: /x: status 429: %w", sources.ErrBlocked)
	for range 2 {
		if _, err := pipeline.PollWatch(ctx, watch); err != nil {
			t.Fatal(err)
		}
	}
	if len(lunStub.fetched) != 1 {
		t.Fatalf("fetches during cooldown %d", len(lunStub.fetched))
	}
	*now = now.Add(blockCooldown + time.Second)
	lunStub.fetchErr = nil
	if _, err := pipeline.PollWatch(ctx, watch); err != nil {
		t.Fatal(err)
	}
	if len(lunStub.fetched) != 2 {
		t.Fatalf("fetches after cooldown %d", len(lunStub.fetched))
	}
}
