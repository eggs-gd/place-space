package domria

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/sources"
)

// districtPages is how many pages of a district are read to find what lies
// inside it, once a week per city.
const districtPages = 5

var (
	areaAnchorPattern = regexp.MustCompile(`(?s)<a\b[^>]*href="([^"]+-rayon-[^"]+)"[^>]*>(.*?)</a>`)
	tagPattern        = regexp.MustCompile(`<[^>]+>`)
)

// administrative recognizes a city district's name. DOM.RIA does not mark
// districts, so this picks which pages are worth opening to find what is
// inside them; a wrong guess costs one request and finds nothing.
var administrative = regexp.MustCompile(`(ський|цький)$`)

// Areas lists the areas linked from a city's catalog page. DOM.RIA keeps
// districts and microdistricts in one list and does not say which is which,
// so what lies inside a district is read from the listings on its page.
func (a *Adapter) Areas(ctx context.Context, city string) ([]domain.Area, error) {
	base, err := a.cityPath(city)
	if err != nil {
		return nil, err
	}
	endpoint, err := url.Parse(a.baseURL + base)
	if err != nil {
		return nil, fmt.Errorf("domria: build url: %w", err)
	}
	body, err := a.load(ctx, endpoint, false)
	if err != nil {
		return nil, err
	}
	areas := parseAreas(base, string(body))
	votes := domain.ParentVotes{}
	var incomplete error
	for _, area := range areas {
		if !administrative.MatchString(area.Key) {
			continue
		}
		if a.pause > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(a.pause):
			}
		}
		err := a.collectChildren(ctx, area, votes)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			incomplete = fmt.Errorf("domria: %w: %w", sources.ErrIncompleteAreas, err)
			if errors.Is(err, sources.ErrBlocked) {
				break
			}
		}
	}
	for i := range areas {
		areas[i].Parent = votes.Best(areas[i].Key)
	}
	return areas, incomplete
}

// collectChildren reads several pages of a district: small microdistricts
// rarely appear among the first twenty listings of a large one.
func (a *Adapter) collectChildren(ctx context.Context, area domain.Area, votes domain.ParentVotes) error {
	seen := map[int64]struct{}{}
	for page := 1; page <= districtPages; page++ {
		if page > 1 && a.pause > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(a.pause):
			}
		}
		cards, total, err := a.fetchPage(ctx, target{path: area.Path, strict: true}, page)
		if err != nil {
			return err
		}
		added := 0
		for _, raw := range cards {
			card, err := decodeCard(raw)
			if err != nil {
				continue
			}
			if _, ok := seen[card.ID]; ok {
				continue
			}
			seen[card.ID] = struct{}{}
			added++
			votes.Add(domain.AreaKey(card.District), area.Key)
		}
		if added == 0 || (total > 0 && len(seen) >= total) {
			return nil
		}
	}
	return nil
}

func parseAreas(base, page string) []domain.Area {
	prefix := strings.TrimRight(base, "/") + "-rayon-"
	byKey := map[string]domain.Area{}
	for _, match := range areaAnchorPattern.FindAllStringSubmatch(page, -1) {
		path := match[1]
		if !strings.HasPrefix(path, prefix) || !catalogPath.MatchString(path) {
			continue
		}
		name := strings.Join(strings.Fields(html.UnescapeString(tagPattern.ReplaceAllString(match[2], " "))), " ")
		key := domain.AreaKey(name)
		if key == "" {
			continue
		}
		if current, ok := byKey[key]; ok && current.Path < path {
			continue
		}
		byKey[key] = domain.Area{Key: key, Name: name, Kind: domain.AreaMicrodistrict, Path: path}
	}
	out := make([]domain.Area, 0, len(byKey))
	for _, area := range byKey {
		out = append(out, area)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
