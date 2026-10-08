package domain

import "testing"

func TestCanonicalURL(t *testing.T) {
	got, err := CanonicalURL("http://www.OLX.ua/d/uk/obyavlenie/test-ID1.html?utm=1#photo")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://www.olx.ua/d/uk/obyavlenie/test-ID1.html"
	if got != want {
		t.Fatalf("got %s", got)
	}
}

func TestCanonicalURLRejectsEmpty(t *testing.T) {
	if _, err := CanonicalURL("  "); err == nil {
		t.Fatal("expected error")
	}
}
