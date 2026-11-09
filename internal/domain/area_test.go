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
