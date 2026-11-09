package domain

import "strings"

const (
	AreaDistrict      = "district"
	AreaMicrodistrict = "microdistrict"
)

// Area is one part of a city as a single source publishes it. Path is that
// source's own catalog page for the area; sources share no ids or slugs, so
// the same place is recognized across them only by Key.
type Area struct {
	Key  string
	Name string
	Kind string
	Path string
	// Parent is the key of the district containing this area, when known.
	Parent string
}

// AreaRef is an area chosen on a watch.
type AreaRef struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// AreaKey reduces a place name to the form sources are compared by.
func AreaKey(name string) string {
	name = strings.NewReplacer("’", "'", "ʼ", "'", "`", "'", "´", "'").Replace(strings.ToLower(name))
	words := strings.Fields(name)
	kept := words[:0]
	for _, word := range words {
		if word != "район" && word != "р-н" {
			kept = append(kept, word)
		}
	}
	return strings.Join(kept, " ")
}

// ParentVotes counts how often listings place an area inside a district. A
// single listing can be mislabelled, and some names span several districts.
type ParentVotes map[string]map[string]int

// Add records one observation that child lies inside parent.
func (v ParentVotes) Add(child, parent string) {
	if child == "" || parent == "" || child == parent {
		return
	}
	if v[child] == nil {
		v[child] = map[string]int{}
	}
	v[child][parent]++
}

// Best returns the district seen most often for child, or "" when none was.
func (v ParentVotes) Best(child string) string {
	best, most := "", 0
	for parent, count := range v[child] {
		if count > most || (count == most && parent < best) {
			best, most = parent, count
		}
	}
	return best
}
