package application

import (
	"context"
	"testing"

	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/sources"
)

type plainAdapter struct{}

func (plainAdapter) Type() string { return "plain" }
func (plainAdapter) Fetch(context.Context, domain.Query) ([]sources.RawListing, error) {
	return nil, nil
}
func (plainAdapter) Normalize(context.Context, sources.RawListing) (domain.Listing, error) {
	return domain.Listing{}, nil
}

type nearAdapter struct{ plainAdapter }

func (nearAdapter) SearchesNear() {}

func TestScopeNearAPlace(t *testing.T) {
	p := &Pipeline{}
	near := &domain.Near{Name: "Поділ", PlaceID: "id", RadiusMeters: 1000}
	watch := domain.Watch{Query: domain.Query{City: "Київ", Near: near}}
	source := domain.Source{ID: "s"}

	if _, _, ok := p.scope(context.Background(), watch, source, plainAdapter{}); ok {
		t.Fatal("a source without a place index read the whole city unchecked")
	}
	scoped, _, ok := p.scope(context.Background(), watch, source, nearAdapter{})
	if !ok || scoped.Query.Near != near {
		t.Fatalf("near source: ok=%v query %+v", ok, scoped.Query)
	}

	watch.Query.Areas = []domain.AreaRef{{Key: "поділ", Name: "Поділ"}}
	scoped, _, ok = p.scope(context.Background(), watch, source, nearAdapter{})
	if ok {
		t.Fatalf("near source checked areas without a place index: %+v", scoped.Query)
	}
}
