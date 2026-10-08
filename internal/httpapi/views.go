package httpapi

import (
	"time"

	"github.com/eggs-gd/place-space/internal/domain"
)

type errorBody struct {
	Error string `json:"error"`
}

type overviewBody struct {
	Watch   *watchJSON  `json:"watch"`
	Summary summaryBody `json:"summary"`
	Source  *sourceJSON `json:"source"`
	Run     *runJSON    `json:"run"`
}

type summaryBody struct {
	Places   int `json:"places"`
	New      int `json:"new"`
	Rejected int `json:"rejected"`
}

type watchJSON struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Enabled             bool     `json:"enabled"`
	City                string   `json:"city"`
	Deal                string   `json:"deal"`
	Properties          []string `json:"properties"`
	PriceMin            *int64   `json:"priceMin"`
	PriceMax            *int64   `json:"priceMax"`
	Currency            string   `json:"currency"`
	RoomsMin            *int     `json:"roomsMin"`
	AreaMin             *float64 `json:"areaMin"`
	PollIntervalSeconds int64    `json:"pollIntervalSeconds"`
}

type sourceJSON struct {
	ID            string     `json:"id"`
	Type          string     `json:"type"`
	Enabled       bool       `json:"enabled"`
	Status        string     `json:"status"`
	LastSuccessAt *time.Time `json:"lastSuccessAt"`
	LastErrorAt   *time.Time `json:"lastErrorAt"`
	LastError     string     `json:"lastError"`
}

type runJSON struct {
	SourceType string    `json:"sourceType"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
	DurationMs int64     `json:"durationMs"`
	Received   int       `json:"received"`
	New        int       `json:"new"`
	Changed    int       `json:"changed"`
	Matched    int       `json:"matched"`
	Rejected   int       `json:"rejected"`
	Duplicates int       `json:"duplicates"`
	Failed     int       `json:"failed"`
	Errors     []string  `json:"errors"`
}

type feedBody struct {
	Status string     `json:"status"`
	Items  []cardBody `json:"items"`
}

type cardBody struct {
	ID          string          `json:"id"`
	Source      string          `json:"source"`
	URL         string          `json:"url"`
	Title       string          `json:"title"`
	PriceAmount *int64          `json:"priceAmount"`
	Currency    string          `json:"currency"`
	Location    string          `json:"location"`
	Address     string          `json:"address"`
	Rooms       *int            `json:"rooms"`
	Area        *float64        `json:"area"`
	Floor       *int            `json:"floor"`
	TotalFloors *int            `json:"totalFloors"`
	Image       string          `json:"image"`
	PublishedAt *time.Time      `json:"publishedAt"`
	FirstSeenAt time.Time       `json:"firstSeenAt"`
	LastSeenAt  time.Time       `json:"lastSeenAt"`
	IsNew       bool            `json:"isNew"`
	Status      string          `json:"status"`
	Reasons     []domain.Reason `json:"reasons"`
}

type detailBody struct {
	cardBody
	Description string          `json:"description"`
	Images      []string        `json:"images"`
	History     []historyBody   `json:"history"`
	Duplicates  []duplicateBody `json:"duplicates"`
}

type historyBody struct {
	SeenAt      time.Time `json:"seenAt"`
	PriceAmount int64     `json:"priceAmount"`
	Currency    string    `json:"currency"`
}

type duplicateBody struct {
	ID          string `json:"id"`
	Source      string `json:"source"`
	URL         string `json:"url"`
	Title       string `json:"title"`
	PriceAmount *int64 `json:"priceAmount"`
	Currency    string `json:"currency"`
}
