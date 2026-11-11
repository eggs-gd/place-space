package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/places"
)

// PlaceResolver turns place names into Google Places ids and keeps the key.
type PlaceResolver interface {
	State(ctx context.Context) (places.KeyState, error)
	SetKey(ctx context.Context, key string) error
	Resolve(ctx context.Context, text string) (string, error)
}

const (
	defaultRadiusMeters = 1000
	minRadiusMeters     = 100
	maxRadiusMeters     = 20_000
)

type settingsJSON struct {
	GoogleKey places.KeyState `json:"googleKey"`
}

type settingsBody struct {
	GoogleKey *string `json:"googleKey"`
}

type nearBody struct {
	Name         string `json:"name"`
	RadiusMeters int    `json:"radiusMeters"`
}

type nearJSON struct {
	Name         string `json:"name"`
	RadiusMeters int    `json:"radiusMeters"`
}

func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	if s.Places == nil {
		writeJSON(w, http.StatusOK, settingsJSON{})
		return
	}
	state, err := s.Places.State(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settingsJSON{GoogleKey: state})
}

// The key is never sent back: the response only says whether one is set.
func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	if s.Places == nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "налаштування недоступні"})
		return
	}
	var body settingsBody
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || body.GoogleKey == nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "не вдалося прочитати налаштування"})
		return
	}
	if err := s.Places.SetKey(r.Context(), *body.GoogleKey); err != nil {
		if errors.Is(err, places.ErrKeyRejected) {
			writeJSON(w, http.StatusBadRequest, errorBody{Error: "Google не прийняв цей ключ. Перевірте, що для нього ввімкнено Places API (New)."})
			return
		}
		writeJSON(w, http.StatusBadGateway, errorBody{Error: "не вдалося перевірити ключ у Google"})
		return
	}
	s.settings(w, r)
}

// resolveNear fills the watch's place and reports whether it may be saved.
// A place whose name and city did not change keeps its id, so only a new
// place costs a lookup: the same street name in another city is another street.
func (s *Server) resolveNear(w http.ResponseWriter, r *http.Request, watch *domain.Watch, requested *nearBody, previous *domain.Near, previousCity string) bool {
	if requested == nil || strings.TrimSpace(requested.Name) == "" {
		watch.Query.Near = nil
		return true
	}
	name := strings.Join(strings.Fields(requested.Name), " ")
	radius := requested.RadiusMeters
	if radius == 0 {
		radius = defaultRadiusMeters
	}
	if radius < minRadiusMeters || radius > maxRadiusMeters {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "радіус має бути від 100 м до 20 км"})
		return false
	}
	if previous != nil && previous.PlaceID != "" && strings.EqualFold(previous.Name, name) && watch.Query.City == previousCity {
		watch.Query.Near = &domain.Near{Name: name, PlaceID: previous.PlaceID, RadiusMeters: radius}
		return true
	}
	if s.Places == nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "пошук біля адреси недоступний"})
		return false
	}
	id, err := s.Places.Resolve(r.Context(), placeQuery(name, watch.Query.City))
	switch {
	case errors.Is(err, places.ErrNoKey):
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "щоб шукати біля адреси, вкажіть ключ Google нижче на цій сторінці"})
		return false
	case errors.Is(err, places.ErrNotFound):
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "Google не знайшов «" + name + "»"})
		return false
	case errors.Is(err, places.ErrKeyRejected):
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "Google не прийняв ключ. Перевірте його нижче на цій сторінці."})
		return false
	case err != nil:
		writeJSON(w, http.StatusBadGateway, errorBody{Error: "не вдалося знайти адресу в Google"})
		return false
	}
	watch.Query.Near = &domain.Near{Name: name, PlaceID: id, RadiusMeters: radius}
	return true
}

// placeQuery adds the city, so a street name finds the street in that city.
func placeQuery(name, city string) string {
	city = strings.TrimSpace(city)
	if city == "" || strings.HasPrefix(city, "/") || strings.Contains(strings.ToLower(name), strings.ToLower(city)) {
		return name
	}
	return name + ", " + city
}

func nearView(near *domain.Near) *nearJSON {
	if near == nil {
		return nil
	}
	return &nearJSON{Name: near.Name, RadiusMeters: near.RadiusMeters}
}
