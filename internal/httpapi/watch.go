package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/eggs-gd/place-space/internal/application"
	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/storage/sqlite"
)

const defaultPollInterval = 10 * time.Minute

type criteriaBody struct {
	City       string   `json:"city"`
	PriceMin   *int64   `json:"priceMin"`
	PriceMax   *int64   `json:"priceMax"`
	RoomsMin   *int     `json:"roomsMin"`
	AreaMin    *float64 `json:"areaMin"`
	Properties []string `json:"properties"`
	Enabled    *bool    `json:"enabled"`
}

func (s *Server) createWatch(w http.ResponseWriter, r *http.Request) {
	body, ok := readCriteria(w, r)
	if !ok {
		return
	}
	watch, err := applyCriteria(domain.Watch{
		Enabled:      true,
		Query:        domain.Query{Deal: domain.DealRent},
		PollInterval: defaultPollInterval,
	}, body, true)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: err.Error()})
		return
	}
	sourceIDs, err := application.EnsureDefaultSources(r.Context(), s.Store)
	if err != nil {
		writeError(w, err)
		return
	}
	watch.SourceIDs = sourceIDs
	watch, err = s.Store.CreateWatch(r.Context(), watch)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeJSON(w, http.StatusBadRequest, errorBody{Error: "такий пошук вже є"})
			return
		}
		writeError(w, err)
		return
	}
	s.refresh(watch)
	writeJSON(w, http.StatusCreated, watchBody(watch))
}

func (s *Server) updateWatch(w http.ResponseWriter, r *http.Request) {
	body, ok := readCriteria(w, r)
	if !ok {
		return
	}
	current, err := s.Store.GetWatch(r.Context(), r.PathValue("id"))
	if errors.Is(err, sqlite.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, errorBody{Error: "пошук не знайдено"})
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	updated, err := applyCriteria(current, body, false)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: err.Error()})
		return
	}
	if err := s.Store.UpdateWatch(r.Context(), updated); err != nil {
		writeError(w, err)
		return
	}
	if queryChanged(current.Query, updated.Query) {
		if err := s.Store.ClearMatches(r.Context(), updated.ID); err != nil {
			writeError(w, err)
			return
		}
	}
	s.refresh(updated)
	writeJSON(w, http.StatusOK, watchBody(updated))
}

func (s *Server) refresh(watch domain.Watch) {
	if s.Refresh != nil && watch.Enabled {
		s.Refresh(watch.ID)
	}
}

func readCriteria(w http.ResponseWriter, r *http.Request) (criteriaBody, bool) {
	var body criteriaBody
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "не вдалося прочитати критерії"})
		return criteriaBody{}, false
	}
	return body, true
}

func applyCriteria(watch domain.Watch, body criteriaBody, creating bool) (domain.Watch, error) {
	city := strings.TrimSpace(body.City)
	if city == "" {
		return domain.Watch{}, errors.New("вкажіть місто")
	}
	if body.PriceMin != nil && *body.PriceMin < 0 {
		return domain.Watch{}, errors.New("ціна не може бути від’ємною")
	}
	if body.PriceMax != nil && *body.PriceMax < 0 {
		return domain.Watch{}, errors.New("ціна не може бути від’ємною")
	}
	if body.PriceMin != nil && body.PriceMax != nil && *body.PriceMin > *body.PriceMax {
		return domain.Watch{}, errors.New("мінімальна ціна більша за максимальну")
	}
	if body.RoomsMin != nil && *body.RoomsMin < 1 {
		return domain.Watch{}, errors.New("кімнати мають бути від 1")
	}
	if body.AreaMin != nil && *body.AreaMin <= 0 {
		return domain.Watch{}, errors.New("площа має бути більшою за нуль")
	}
	properties, err := propertiesOf(body.Properties)
	if err != nil {
		return domain.Watch{}, err
	}
	if creating {
		if watch.Query.Deal == "" {
			watch.Query.Deal = domain.DealRent
		}
		if watch.PollInterval <= 0 {
			watch.PollInterval = defaultPollInterval
		}
		watch.Enabled = true
	}
	if creating || strings.TrimSpace(watch.Name) == "" || strings.TrimSpace(watch.Name) == strings.TrimSpace(watch.Query.City) {
		watch.Name = city
	}
	if body.Enabled != nil {
		watch.Enabled = *body.Enabled
	}
	watch.Query.City = city
	watch.Query.Properties = properties
	watch.Filters.PriceMin = body.PriceMin
	watch.Filters.PriceMax = body.PriceMax
	watch.Filters.RoomsMin = body.RoomsMin
	watch.Filters.AreaMin = body.AreaMin
	return watch, nil
}

func queryChanged(before, after domain.Query) bool {
	if strings.TrimSpace(before.City) != strings.TrimSpace(after.City) || before.Deal != after.Deal {
		return true
	}
	if len(before.Properties) != len(after.Properties) {
		return true
	}
	seen := make(map[string]int, len(before.Properties))
	for _, value := range before.Properties {
		seen[value]++
	}
	for _, value := range after.Properties {
		seen[value]--
		if seen[value] < 0 {
			return true
		}
	}
	return false
}

func propertiesOf(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, errors.New("оберіть квартиру, будинок або обидва")
	}
	seen := map[string]bool{}
	out := make([]string, 0, 2)
	for _, value := range values {
		if value != domain.PropertyApartment && value != domain.PropertyHouse {
			return nil, errors.New("оберіть квартиру, будинок або обидва")
		}
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out, nil
}

func watchBody(watch domain.Watch) watchJSON {
	properties := watch.Query.Properties
	if properties == nil {
		properties = []string{}
	}
	return watchJSON{
		ID:                  watch.ID,
		Name:                watch.Name,
		Enabled:             watch.Enabled,
		City:                watch.Query.City,
		Deal:                watch.Query.Deal,
		Properties:          properties,
		PriceMin:            watch.Filters.PriceMin,
		PriceMax:            watch.Filters.PriceMax,
		Currency:            watch.Filters.Currency,
		RoomsMin:            watch.Filters.RoomsMin,
		AreaMin:             watch.Filters.AreaMin,
		PollIntervalSeconds: int64(watch.PollInterval / time.Second),
	}
}
