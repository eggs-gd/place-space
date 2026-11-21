package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/storage/sqlite"
)

// profileCookie remembers who is using this browser. It is a choice, not a
// login: anyone who can reach the install may pick any profile.
const profileCookie = "place_space_profile"

const profileCookieAge = 365 * 24 * time.Hour

const maxProfileName = 40

type profileKey struct{}

type profileJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type profilesBody struct {
	// Enabled tells the interface to ask who is using it.
	Enabled  bool          `json:"enabled"`
	Current  *profileJSON  `json:"current"`
	Profiles []profileJSON `json:"profiles"`
}

type profileRequiredBody struct {
	Error           string `json:"error"`
	ProfileRequired bool   `json:"profileRequired"`
}

// withProfile lets a request through only with a known profile, and hands
// that profile to the handler. Without profiles every request goes through
// and sees every watch.
func (s *Server) withProfile(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.Profiles || strings.HasPrefix(r.URL.Path, "/api/profiles") {
			next.ServeHTTP(w, r)
			return
		}
		profile, ok, err := s.cookieProfile(r)
		if err != nil {
			writeError(w, err)
			return
		}
		if !ok {
			writeJSON(w, http.StatusUnauthorized, profileRequiredBody{Error: "оберіть профіль", ProfileRequired: true})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), profileKey{}, profile.ID)))
	})
}

func (s *Server) cookieProfile(r *http.Request) (sqlite.Profile, bool, error) {
	cookie, err := r.Cookie(profileCookie)
	if err != nil || cookie.Value == "" {
		return sqlite.Profile{}, false, nil
	}
	profile, err := s.Store.GetProfile(r.Context(), cookie.Value)
	if errors.Is(err, sqlite.ErrNotFound) {
		return sqlite.Profile{}, false, nil
	}
	if err != nil {
		return sqlite.Profile{}, false, err
	}
	return profile, true, nil
}

// requestProfile is the profile withProfile admitted, or "" without profiles.
func requestProfile(r *http.Request) string {
	id, _ := r.Context().Value(profileKey{}).(string)
	return id
}

// owns reports whether the request may see and change the watch.
func (s *Server) owns(r *http.Request, watch domain.Watch) bool {
	return !s.Profiles || watch.ProfileID == requestProfile(r)
}

func (s *Server) profiles(w http.ResponseWriter, r *http.Request) {
	body := profilesBody{Enabled: s.Profiles, Profiles: []profileJSON{}}
	if !s.Profiles {
		writeJSON(w, http.StatusOK, body)
		return
	}
	profiles, err := s.Store.ListProfiles(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	for _, profile := range profiles {
		body.Profiles = append(body.Profiles, profileJSON{ID: profile.ID, Name: profile.Name})
	}
	current, ok, err := s.cookieProfile(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if ok {
		body.Current = &profileJSON{ID: current.ID, Name: current.Name}
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) createProfile(w http.ResponseWriter, r *http.Request) {
	if !s.Profiles {
		writeJSON(w, http.StatusNotFound, errorBody{Error: "профілі вимкнено"})
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "некоректний запит"})
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "вкажіть ім’я"})
		return
	}
	if utf8.RuneCountInString(name) > maxProfileName {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "ім’я задовге"})
		return
	}
	profile, err := s.Store.CreateProfile(r.Context(), name)
	if errors.Is(err, sqlite.ErrProfileExists) {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "такий профіль уже є"})
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	setProfileCookie(w, profile.ID)
	writeJSON(w, http.StatusCreated, profileJSON{ID: profile.ID, Name: profile.Name})
}

func (s *Server) selectProfile(w http.ResponseWriter, r *http.Request) {
	if !s.Profiles {
		writeJSON(w, http.StatusNotFound, errorBody{Error: "профілі вимкнено"})
		return
	}
	profile, err := s.Store.GetProfile(r.Context(), r.PathValue("id"))
	if errors.Is(err, sqlite.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, errorBody{Error: "профіль не знайдено"})
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	setProfileCookie(w, profile.ID)
	writeJSON(w, http.StatusOK, profileJSON{ID: profile.ID, Name: profile.Name})
}

func (s *Server) leaveProfile(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: profileCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	w.WriteHeader(http.StatusNoContent)
}

func setProfileCookie(w http.ResponseWriter, profileID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     profileCookie,
		Value:    profileID,
		Path:     "/",
		MaxAge:   int(profileCookieAge.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
