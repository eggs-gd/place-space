package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/storage/sqlite"
)

type profileClient struct {
	t    *testing.T
	base string
	http *http.Client
}

func newProfileClient(t *testing.T, base string) *profileClient {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &profileClient{t: t, base: base, http: &http.Client{Jar: jar}}
}

func (c *profileClient) do(method, path string, body any, out any) int {
	c.t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		c.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil && res.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			c.t.Fatalf("%s %s: %v", method, path, err)
		}
	}
	return res.StatusCode
}

func openProfileStore(t *testing.T) *sqlite.Store {
	t.Helper()
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "place.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestProfilesKeepWatchesApart(t *testing.T) {
	ctx := context.Background()
	store := openProfileStore(t)
	existing, err := store.CreateWatch(ctx, domain.Watch{
		Name: "Київ", Enabled: true, PollInterval: 10 * time.Minute,
		Query: domain.Query{City: "Київ", Deal: domain.DealRent},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer((&Server{Store: store, Profiles: true}).Handler())
	t.Cleanup(server.Close)

	olia := newProfileClient(t, server.URL)
	var required profileRequiredBody
	if status := olia.do("GET", "/api/overview", nil, &required); status != http.StatusUnauthorized || !required.ProfileRequired {
		t.Fatalf("overview without a profile = %d %+v", status, required)
	}
	var list profilesBody
	olia.do("GET", "/api/profiles", nil, &list)
	if !list.Enabled || list.Current != nil || len(list.Profiles) != 0 {
		t.Fatalf("profiles before any = %+v", list)
	}
	var created profileJSON
	if status := olia.do("POST", "/api/profiles", map[string]string{"name": "Оля"}, &created); status != http.StatusCreated {
		t.Fatalf("create = %d", status)
	}
	var overview overviewBody
	if status := olia.do("GET", "/api/overview", nil, &overview); status != http.StatusOK || overview.Watch == nil || overview.Watch.ID != existing.ID {
		t.Fatalf("first profile does not see the existing watch: %d %+v", status, overview.Watch)
	}

	sasha := newProfileClient(t, server.URL)
	var other profileJSON
	if status := sasha.do("POST", "/api/profiles", map[string]string{"name": "Саша"}, &other); status != http.StatusCreated {
		t.Fatalf("create second = %d", status)
	}
	overview = overviewBody{}
	sasha.do("GET", "/api/overview", nil, &overview)
	if overview.Watch != nil {
		t.Fatalf("second profile sees someone else's watch: %+v", overview.Watch)
	}
	if status := sasha.do("GET", "/api/overview?watch="+existing.ID, nil, &overview); status != http.StatusOK || overview.Watch != nil {
		t.Fatalf("second profile opened someone else's watch by id: %+v", overview.Watch)
	}
	var failure errorBody
	if status := sasha.do("PATCH", "/api/watches/"+existing.ID, map[string]any{"city": "Львів"}, &failure); status != http.StatusNotFound {
		t.Fatalf("second profile changed someone else's watch: %d", status)
	}
	var own watchJSON
	raw := map[string]any{}
	if status := sasha.do("POST", "/api/watches", map[string]any{"city": "Київ", "properties": []string{"apartment"}}, &raw); status != http.StatusCreated {
		t.Fatalf("second profile cannot watch the same city: %d %v", status, raw)
	}
	if encoded, err := json.Marshal(raw); err != nil || json.Unmarshal(encoded, &own) != nil {
		t.Fatal("watch body")
	}
	stored, err := store.GetWatch(ctx, own.ID)
	if err != nil || stored.ProfileID != other.ID {
		t.Fatalf("new watch belongs to %q, %v", stored.ProfileID, err)
	}

	list = profilesBody{}
	sasha.do("GET", "/api/profiles", nil, &list)
	if list.Current == nil || list.Current.ID != other.ID || len(list.Profiles) != 2 {
		t.Fatalf("profiles = %+v", list)
	}
	if status := sasha.do("POST", "/api/profiles/"+created.ID+"/select", nil, nil); status != http.StatusOK {
		t.Fatalf("select = %d", status)
	}
	overview = overviewBody{}
	sasha.do("GET", "/api/overview", nil, &overview)
	if overview.Watch == nil || overview.Watch.ID != existing.ID {
		t.Fatalf("switching profile did not switch the watch: %+v", overview.Watch)
	}
	if status := sasha.do("DELETE", "/api/profiles/current", nil, nil); status != http.StatusNoContent {
		t.Fatalf("leave = %d", status)
	}
	if status := sasha.do("GET", "/api/overview", nil, &required); status != http.StatusUnauthorized {
		t.Fatalf("overview after leaving = %d", status)
	}
	if status := sasha.do("POST", "/api/profiles", map[string]string{"name": "Оля"}, &failure); status != http.StatusBadRequest {
		t.Fatalf("duplicate profile = %d", status)
	}
	if status := sasha.do("POST", "/api/profiles/missing/select", nil, &failure); status != http.StatusNotFound {
		t.Fatalf("select missing = %d", status)
	}
}

func TestWithoutProfilesEverythingIsShared(t *testing.T) {
	ctx := context.Background()
	store := openProfileStore(t)
	if _, err := store.CreateWatch(ctx, domain.Watch{
		Name: "Київ", Enabled: true, PollInterval: 10 * time.Minute,
		Query: domain.Query{City: "Київ", Deal: domain.DealRent},
	}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer((&Server{Store: store}).Handler())
	t.Cleanup(server.Close)
	client := newProfileClient(t, server.URL)
	var list profilesBody
	client.do("GET", "/api/profiles", nil, &list)
	if list.Enabled {
		t.Fatal("profiles reported on without the option")
	}
	var overview overviewBody
	if status := client.do("GET", "/api/overview", nil, &overview); status != http.StatusOK || overview.Watch == nil {
		t.Fatalf("overview = %d %+v", status, overview.Watch)
	}
	var failure errorBody
	if status := client.do("POST", "/api/profiles", map[string]string{"name": "Оля"}, &failure); status != http.StatusNotFound {
		t.Fatalf("create without the option = %d", status)
	}
}

func TestProfileCookieSecureOnlyOverHTTPS(t *testing.T) {
	store := openProfileStore(t)
	handler := (&Server{Store: store, Profiles: true}).Handler()
	cases := []struct {
		name   string
		proto  string
		secure bool
	}{
		{"plain HTTP", "", false},
		{"behind an HTTPS proxy", "https", true},
	}
	for i, c := range cases {
		req := httptest.NewRequest("POST", "/api/profiles", bytes.NewReader([]byte(`{"name":"`+c.name+`"}`)))
		if c.proto != "" {
			req.Header.Set("X-Forwarded-Proto", c.proto)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		cookies := rec.Result().Cookies()
		if rec.Code != http.StatusCreated || len(cookies) != 1 {
			t.Fatalf("case %d: %d %v", i, rec.Code, cookies)
		}
		if cookies[0].Secure != c.secure {
			t.Fatalf("%s: Secure = %v", c.name, cookies[0].Secure)
		}
	}
}
