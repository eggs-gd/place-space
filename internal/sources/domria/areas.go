package domria

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/eggs-gd/place-space/internal/domain"
)

var (
	areaAnchorPattern = regexp.MustCompile(`(?s)<a\b[^>]*href="([^"]+-rayon-[^"]+)"[^>]*>(.*?)</a>`)
	tagPattern        = regexp.MustCompile(`<[^>]+>`)
)

// Areas lists the areas linked from a city's catalog page. DOM.RIA keeps
// districts and microdistricts in one list and does not say which is which.
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
	return parseAreas(base, string(body)), nil
}

func parseAreas(base, page string) []domain.Area {
	prefix := strings.TrimRight(base, "/") + "-rayon-"
	byKey := map[string]domain.Area{}
	for _, match := range areaAnchorPattern.FindAllStringSubmatch(page, -1) {
		path := match[1]
		if !strings.HasPrefix(path, prefix) {
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
