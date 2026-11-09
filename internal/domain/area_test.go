package domain

import "testing"

func TestAreaKeyMatchesSourceSpellings(t *testing.T) {
	cases := map[string]string{
		"Деснянський район": "деснянський",
		"Деснянський":       "деснянський",
		"Солом’янський":     "солом'янський",
		"Солом'янський":     "солом'янський",
		"Лісовий  Масив":    "лісовий масив",
		" Р-н Позняки ":     "позняки",
	}
	for input, want := range cases {
		if got := AreaKey(input); got != want {
			t.Errorf("%q: %q, want %q", input, got, want)
		}
	}
}

func TestParentVotesPickTheCommonestDistrict(t *testing.T) {
	votes := ParentVotes{}
	votes.Add("нова забудова", "печерський")
	votes.Add("нова забудова", "печерський")
	votes.Add("нова забудова", "голосіївський")
	votes.Add("сам", "сам")
	votes.Add("тай", "б")
	votes.Add("тай", "а")
	if got := votes.Best("нова забудова"); got != "печерський" {
		t.Errorf("best %q", got)
	}
	if got := votes.Best("сам"); got != "" {
		t.Errorf("own parent %q", got)
	}
	if got := votes.Best("тай"); got != "а" {
		t.Errorf("tie %q", got)
	}
	if got := votes.Best("нема"); got != "" {
		t.Errorf("unknown %q", got)
	}
}
