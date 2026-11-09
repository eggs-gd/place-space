package lun

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

var (
	anchorPattern = regexp.MustCompile(`(?s)<a\b([^>]*)>(.*?)</a>`)
	hrefPattern   = regexp.MustCompile(`href="([^"]+)"`)
	labelPattern  = regexp.MustCompile(`data-event-label="([^"]*)"`)
	actionPattern = regexp.MustCompile(`data-event-action="([^"]*)"`)
	tagPattern    = regexp.MustCompile(`<[^>]+>`)
)

// Areas lists a city's districts and microdistricts. The city page names all
// districts; each district's page and its cards name the microdistricts.
func (a *Adapter) Areas(ctx context.Context, city string) ([]domain.Area, error) {
	base, err := a.cityPath(city)
	if err != nil {
		return nil, err
	}
	found := map[string]domain.Area{}
	body, err := a.loadAreaPage(ctx, base)
	if err != nil {
		return nil, err
	}
	collectAreas(found, base, string(body))

	districts := make([]string, 0, len(found))
	for _, area := range found {
		if area.Kind == domain.AreaDistrict {
			districts = append(districts, area.Path)
		}
	}
	sort.Strings(districts)
	for _, path := range districts {
		if a.pause > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(a.pause):
			}
		}
		body, err := a.loadAreaPage(ctx, path)
		if errors.Is(err, sources.ErrBlocked) || ctx.Err() != nil {
			break
		}
		if err != nil {
			continue
		}
		collectAreas(found, base, string(body))
	}

	byKey := map[string]domain.Area{}
	for _, area := range found {
		// A district outranks a microdistrict of the same name.
		if current, ok := byKey[area.Key]; ok && (current.Kind == domain.AreaDistrict || current.Path < area.Path) {
			continue
		}
		byKey[area.Key] = area
	}
	out := make([]domain.Area, 0, len(byKey))
	for _, area := range byKey {
		out = append(out, area)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func (a *Adapter) loadAreaPage(ctx context.Context, path string) ([]byte, error) {
	endpoint, err := url.Parse(a.baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("lun: build url: %w", err)
	}
	return a.load(ctx, endpoint, false)
}

func collectAreas(found map[string]domain.Area, base, page string) {
	prefix := base + "-"
	add := func(path, name, kind string) {
		if !strings.HasPrefix(path, prefix) || strings.TrimSpace(name) == "" {
			return
		}
		key := domain.AreaKey(name)
		if key == "" {
			return
		}
		if _, ok := found[path]; ok {
			return
		}
		found[path] = domain.Area{Key: key, Name: displayName(name), Kind: kind, Path: path}
	}
	for _, match := range anchorPattern.FindAllStringSubmatch(page, -1) {
		href := hrefPattern.FindStringSubmatch(match[1])
		if href == nil {
			continue
		}
		kind := anchorKind(attr(labelPattern, match[1]), attr(actionPattern, match[1]))
		if kind == "" {
			continue
		}
		add(href[1], html.UnescapeString(tagPattern.ReplaceAllString(match[2], " ")), kind)
	}
	payload, err := extractRealties(page)
	if err != nil {
		return
	}
	for _, raw := range payload.Cards {
		card, err := decodeCard(raw)
		if err != nil {
			continue
		}
		for _, entity := range card.GeoEntities {
			if entity.URL == nil || (entity.Type != domain.AreaDistrict && entity.Type != domain.AreaMicrodistrict) {
				continue
			}
			add(entity.URL.Link, entity.Name, entity.Type)
		}
	}
}

// anchorKind reads LUN's own analytics labels, the only marker separating a
// district link from the metro stations and room filters next to it.
func anchorKind(label, action string) string {
	switch {
	case label == "district_click" || action == "internal_linkdistrict":
		return domain.AreaDistrict
	case label == "microdistrict_click":
		return domain.AreaMicrodistrict
	default:
		return ""
	}
}

func attr(pattern *regexp.Regexp, attrs string) string {
	match := pattern.FindStringSubmatch(attrs)
	if match == nil {
		return ""
	}
	return match[1]
}

func displayName(name string) string {
	name = strings.Join(strings.Fields(name), " ")
	return strings.TrimSpace(strings.TrimSuffix(name, " район"))
}
