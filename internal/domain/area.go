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
