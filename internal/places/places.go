// Package places turns a typed place name into a Google Places id.
//
// Only Text Search with the places.id field mask is used: Google bills that
// as Text Search Essentials (IDs Only), which has no charge. Asking for any
// other field would move the request to a paid tier.
package places

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultEndpoint = "https://places.googleapis.com/v1/places:searchText"
	// SettingKey is where the key is stored in the settings table.
	SettingKey = "google_places_key"
	// EnvKey supplies the key when none is stored.
	EnvKey  = "GAPI_KEY"
	maxBody = 1 << 20
)

var (
	// ErrNoKey means no Google key is stored or set in the environment.
	ErrNoKey = errors.New("no Google Places key")
	// ErrNotFound means Google found no place for the text.
	ErrNotFound = errors.New("place not found")
	// ErrKeyRejected means Google refused the key.
	ErrKeyRejected = errors.New("Google rejected the key")
)

// Settings is where the key is kept.
type Settings interface {
	Setting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
}

// Resolver looks places up with the stored key, or GAPI_KEY when none is stored.
type Resolver struct {
	Settings Settings
	HTTP     *http.Client
	Endpoint string
	// Env reads the environment; nil means os.Getenv.
	Env func(string) string
}

// KeyState describes the key without revealing it.
type KeyState struct {
	Set bool `json:"set"`
	// Hint is the last four characters, so a person can tell keys apart.
	Hint string `json:"hint"`
	// FromEnv is true when the key comes from GAPI_KEY, not from the settings.
	FromEnv bool `json:"fromEnv"`
}

// State reports which key is in use.
func (r *Resolver) State(ctx context.Context) (KeyState, error) {
	key, fromEnv, err := r.key(ctx)
	if err != nil || key == "" {
		return KeyState{}, err
	}
	return KeyState{Set: true, Hint: hint(key), FromEnv: fromEnv}, nil
}

// SetKey checks the key with one free lookup and stores it. An empty key
// removes the stored one.
func (r *Resolver) SetKey(ctx context.Context, key string) error {
	key = strings.TrimSpace(key)
	if key != "" {
		if _, err := r.lookup(ctx, key, "Київ"); err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
	}
	return r.Settings.SetSetting(ctx, SettingKey, key)
}

// Resolve returns the id of the best match for the text.
func (r *Resolver) Resolve(ctx context.Context, text string) (string, error) {
	key, _, err := r.key(ctx)
	if err != nil {
		return "", err
	}
	if key == "" {
		return "", ErrNoKey
	}
	return r.lookup(ctx, key, text)
}

func (r *Resolver) key(ctx context.Context) (string, bool, error) {
	stored, err := r.Settings.Setting(ctx, SettingKey)
	if err != nil {
		return "", false, err
	}
	if stored != "" {
		return stored, false, nil
	}
	env := r.Env
	if env == nil {
		env = os.Getenv
	}
	if key := strings.TrimSpace(env(EnvKey)); key != "" {
		return key, true, nil
	}
	return "", false, nil
}

func (r *Resolver) lookup(ctx context.Context, key, text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", ErrNotFound
	}
	payload, err := json.Marshal(map[string]string{"textQuery": text, "languageCode": "uk", "regionCode": "UA"})
	if err != nil {
		return "", err
	}
	endpoint := r.Endpoint
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Api-Key", key)
	// Anything beyond the id is billed.
	req.Header.Set("X-Goog-FieldMask", "places.id")
	client := r.HTTP
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("google places: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return "", fmt.Errorf("google places: %w", err)
	}
	var result struct {
		Places []struct {
			ID string `json:"id"`
		} `json:"places"`
		Error *struct {
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("google places: status %d", resp.StatusCode)
	}
	if result.Error != nil || resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
			return "", fmt.Errorf("%w: %s", ErrKeyRejected, googleMessage(result.Error))
		}
		return "", fmt.Errorf("google places: status %d: %s", resp.StatusCode, googleMessage(result.Error))
	}
	if len(result.Places) == 0 || result.Places[0].ID == "" {
		return "", ErrNotFound
	}
	return result.Places[0].ID, nil
}

func googleMessage(e *struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}) string {
	if e == nil {
		return "no details"
	}
	return strings.TrimSpace(e.Status + " " + e.Message)
}

func hint(key string) string {
	if len(key) <= 4 {
		return ""
	}
	return key[len(key)-4:]
}
