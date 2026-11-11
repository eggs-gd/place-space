package places

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type memory map[string]string

func (m memory) Setting(_ context.Context, key string) (string, error) { return m[key], nil }

func (m memory) SetSetting(_ context.Context, key, value string) error {
	if value == "" {
		delete(m, key)
	} else {
		m[key] = value
	}
	return nil
}

const goodKey = "AIzaGoodKey1234"

func google(t *testing.T, seen *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mask := r.Header.Get("X-Goog-FieldMask"); mask != "places.id" {
			t.Errorf("field mask %q would be billed", mask)
		}
		var body struct {
			TextQuery string `json:"textQuery"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		*seen = append(*seen, body.TextQuery)
		switch {
		case r.Header.Get("X-Goog-Api-Key") != goodKey:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"status":"INVALID_ARGUMENT","message":"API key not valid."}}`))
		case body.TextQuery == "нікуди":
			_, _ = w.Write([]byte(`{}`))
		default:
			_, _ = w.Write([]byte(`{"places":[{"id":"ChIJ_first"},{"id":"ChIJ_second"}]}`))
		}
	}))
}

func resolver(server *httptest.Server, settings memory, env string) *Resolver {
	return &Resolver{
		Settings: settings,
		HTTP:     server.Client(),
		Endpoint: server.URL,
		Env:      func(string) string { return env },
	}
}

func TestResolveUsesStoredKey(t *testing.T) {
	var seen []string
	server := google(t, &seen)
	defer server.Close()
	r := resolver(server, memory{SettingKey: goodKey}, "")
	id, err := r.Resolve(context.Background(), "Дніпровська набережна, Київ")
	if err != nil || id != "ChIJ_first" {
		t.Fatalf("id %q err %v", id, err)
	}
	if _, err := r.Resolve(context.Background(), "нікуди"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err %v", err)
	}
}

func TestResolveFallsBackToEnvironment(t *testing.T) {
	var seen []string
	server := google(t, &seen)
	defer server.Close()
	r := resolver(server, memory{}, goodKey)
	if _, err := r.Resolve(context.Background(), "Київ"); err != nil {
		t.Fatal(err)
	}
	state, err := r.State(context.Background())
	if err != nil || !state.Set || !state.FromEnv || state.Hint != "1234" {
		t.Fatalf("state %+v err %v", state, err)
	}
}

func TestResolveWithoutKey(t *testing.T) {
	var seen []string
	server := google(t, &seen)
	defer server.Close()
	if _, err := resolver(server, memory{}, "").Resolve(context.Background(), "Київ"); !errors.Is(err, ErrNoKey) {
		t.Fatalf("err %v", err)
	}
	if len(seen) != 0 {
		t.Fatalf("asked Google without a key: %v", seen)
	}
}

func TestSetKeyChecksBeforeStoring(t *testing.T) {
	var seen []string
	server := google(t, &seen)
	defer server.Close()
	settings := memory{}
	r := resolver(server, settings, "")
	if err := r.SetKey(context.Background(), "wrong"); !errors.Is(err, ErrKeyRejected) {
		t.Fatalf("err %v", err)
	}
	if _, ok := settings[SettingKey]; ok {
		t.Fatal("rejected key was stored")
	}
	if err := r.SetKey(context.Background(), "  "+goodKey+" "); err != nil {
		t.Fatal(err)
	}
	if settings[SettingKey] != goodKey {
		t.Fatalf("stored %q", settings[SettingKey])
	}
	if err := r.SetKey(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if _, ok := settings[SettingKey]; ok {
		t.Fatal("key was not removed")
	}
}
