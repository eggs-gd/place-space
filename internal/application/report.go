package application

import (
	"time"

	"github.com/eggs-gd/place-space/internal/domain"
)

// JSONView is the CLI dump. Raw provider payloads stay in SQLite.
type JSONView struct {
	Watch WatchView  `json:"watch"`
	Runs  []RunView  `json:"runs"`
	Items []ItemView `json:"items"`
}

// WatchView is the human-facing watch.
type WatchView struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Enabled      bool           `json:"enabled"`
	Sources      []string       `json:"sources"`
	Query        domain.Query   `json:"query"`
	Filters      domain.Filters `json:"filters"`
	PollInterval string         `json:"pollInterval"`
}

// RunView is one source check.
type RunView struct {
	ID             string    `json:"id"`
	SourceID       string    `json:"sourceId"`
	SourceType     string    `json:"sourceType"`
	StartedAt      time.Time `json:"startedAt"`
	FinishedAt     time.Time `json:"finishedAt"`
	DurationMs     int64     `json:"durationMs"`
	Received       int       `json:"received"`
	New            int       `json:"new"`
	Changed        int       `json:"changed"`
	Matched        int       `json:"matched"`
	Rejected       int       `json:"rejected"`
	Duplicates     int       `json:"duplicates"`
	DuplicateLinks int       `json:"duplicateLinks"`
	Failed         int       `json:"failed"`
	Errors         []string  `json:"errors"`
}

// ItemView is one listing after filter and diff.
type ItemView struct {
	Outcome     string          `json:"outcome"`
	Changes     []domain.Change `json:"changes"`
	DuplicateOf string          `json:"duplicateOf,omitempty"`
	Match       MatchView       `json:"match"`
	Listing     ListingView     `json:"listing"`
}

// MatchView explains why a listing matched or was rejected.
type MatchView struct {
	Status  string          `json:"status"`
	Reasons []domain.Reason `json:"reasons"`
}

// ListingView is the canonical listing without the raw payload.
type ListingView struct {
	ID          string     `json:"id"`
	Source      string     `json:"source"`
	ExternalID  string     `json:"externalId"`
	URL         string     `json:"url"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	PriceAmount *int64     `json:"priceAmount"`
	Currency    string     `json:"currency"`
	Location    string     `json:"location"`
	Address     string     `json:"address"`
	Latitude    *float64   `json:"latitude"`
	Longitude   *float64   `json:"longitude"`
	Rooms       *int       `json:"rooms"`
	Area        *float64   `json:"area"`
	Floor       *int       `json:"floor"`
	TotalFloors *int       `json:"totalFloors"`
	Images      []string   `json:"images"`
	PublishedAt *time.Time `json:"publishedAt"`
	FirstSeenAt time.Time  `json:"firstSeenAt"`
	LastSeenAt  time.Time  `json:"lastSeenAt"`
}

// View converts a report into the JSON dump.
func View(report Report) JSONView {
	runs := make([]RunView, 0, len(report.Runs))
	for _, run := range report.Runs {
		errors := run.Errors
		if errors == nil {
			errors = []string{}
		}
		runs = append(runs, RunView{
			ID:             run.ID,
			SourceID:       run.SourceID,
			SourceType:     run.SourceType,
			StartedAt:      run.StartedAt,
			FinishedAt:     run.FinishedAt,
			DurationMs:     run.Duration().Milliseconds(),
			Received:       run.Received,
			New:            run.New,
			Changed:        run.Changed,
			Matched:        run.Matched,
			Rejected:       run.Rejected,
			Duplicates:     run.Duplicates,
			DuplicateLinks: run.DuplicateLinks,
			Failed:         run.Failed,
			Errors:         errors,
		})
	}
	items := make([]ItemView, 0, len(report.Items))
	for _, item := range report.Items {
		changes := item.Changes
		if changes == nil {
			changes = []domain.Change{}
		}
		reasons := item.Decision.Reasons
		if reasons == nil {
			reasons = []domain.Reason{}
		}
		images := item.Listing.Images
		if images == nil {
			images = []string{}
		}
		items = append(items, ItemView{
			Outcome:     item.Outcome,
			Changes:     changes,
			DuplicateOf: item.DuplicateOf,
			Match: MatchView{
				Status:  item.Decision.Status,
				Reasons: reasons,
			},
			Listing: ListingView{
				ID:          item.Listing.ID,
				Source:      item.Listing.Source,
				ExternalID:  item.Listing.ExternalID,
				URL:         item.Listing.URL,
				Title:       item.Listing.Title,
				Description: item.Listing.Description,
				PriceAmount: item.Listing.PriceAmount,
				Currency:    item.Listing.Currency,
				Location:    item.Listing.Location.Name,
				Address:     item.Listing.Location.Address,
				Latitude:    item.Listing.Location.Latitude,
				Longitude:   item.Listing.Location.Longitude,
				Rooms:       item.Listing.Rooms,
				Area:        item.Listing.Area,
				Floor:       item.Listing.Floor,
				TotalFloors: item.Listing.TotalFloors,
				Images:      images,
				PublishedAt: item.Listing.PublishedAt,
				FirstSeenAt: item.Listing.FirstSeenAt,
				LastSeenAt:  item.Listing.LastSeenAt,
			},
		})
	}
	sources := report.Watch.SourceIDs
	if sources == nil {
		sources = []string{}
	}
	return JSONView{
		Watch: WatchView{
			ID:           report.Watch.ID,
			Name:         report.Watch.Name,
			Enabled:      report.Watch.Enabled,
			Sources:      sources,
			Query:        report.Watch.Query,
			Filters:      report.Watch.Filters,
			PollInterval: report.Watch.PollInterval.String(),
		},
		Runs:  runs,
		Items: items,
	}
}

// HasErrors reports a fetch or normalize failure. A rejected listing is not an error.
func HasErrors(report Report) bool {
	for _, run := range report.Runs {
		if run.Failed > 0 || len(run.Errors) > 0 {
			return true
		}
	}
	return false
}
