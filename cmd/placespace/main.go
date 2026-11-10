package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/eggs-gd/place-space/internal/application"
	"github.com/eggs-gd/place-space/internal/core"
	"github.com/eggs-gd/place-space/internal/domain"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if len(os.Args) < 2 {
		if err := cmdServe([]string{"--open"}); err != nil {
			slog.Error(err.Error())
			os.Exit(1)
		}
		return
	}
	var err error
	switch os.Args[1] {
	case "poll":
		err = cmdPoll(os.Args[2:])
	case "run":
		err = cmdRun(os.Args[2:])
	case "status":
		err = cmdStatus(os.Args[2:])
	case "serve":
		err = cmdServe(os.Args[2:])
	case "help", "-h", "--help":
		usage()
		return
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `Place Space — моніторинг оголошень.

Використання:
  placespace poll --city "Кам'янець-Подільський" --price-max 20000 --rooms-min 2
  placespace serve
  placespace run
  placespace status

Без аргументів програма відкриває власне вікно і сама перевіряє пошуки.
poll створює або оновлює пошук і одразу перевіряє джерела.
serve робить те саме, що запуск без аргументів, але вікно не відкриває.
run перевіряє джерела без HTTP.
`)
}

func cmdPoll(args []string) error {
	fs := flag.NewFlagSet("poll", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dbPath := fs.String("db", "", dbFlagHelp)
	name := fs.String("name", "", "назва watch; за замовчуванням — місто")
	city := fs.String("city", "", "місто або шлях LUN, наприклад /rent/kyiv/flats")
	deal := fs.String("deal", domain.DealRent, "тип угоди")
	property := fs.String("property", domain.PropertyApartment, "apartment, house або обидва через кому")
	interval := fs.Duration("interval", 10*time.Minute, "інтервал наступних перевірок")
	pages := fs.Int("pages", 5, "максимум сторінок пошуку на кожен тип житла")
	var priceMax, priceMin optionalInt64
	var roomsMin optionalInt
	var areaMin optionalFloat
	fs.Var(&priceMax, "price-max", "максимальна ціна, у валюті оголошення")
	fs.Var(&priceMin, "price-min", "мінімальна ціна")
	fs.Var(&roomsMin, "rooms-min", "мінімум кімнат")
	fs.Var(&areaMin, "area-min", "мінімальна площа, м²")
	currency := fs.String("currency", "", "валюта фільтра ціни; з межею ціни типово UAH")
	areas := fs.String("areas", "", "райони міста через кому, наприклад Дарницький,Осокорки")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *city == "" {
		return errors.New("потрібен --city")
	}
	properties, err := parseProperties(*property)
	if err != nil {
		return err
	}
	if *name == "" {
		*name = *city
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, err := openCore(ctx, *dbPath, *pages)
	if err != nil {
		return err
	}
	defer app.Close()

	areaRefs, err := app.Pipeline.Areas.Resolve(ctx, *city, splitList(*areas))
	if err != nil {
		return err
	}
	watch := domain.Watch{
		Name:         *name,
		Enabled:      true,
		SourceIDs:    app.SourceIDs,
		Query:        domain.Query{City: *city, Deal: *deal, Properties: properties, Areas: areaRefs},
		Filters:      domain.Filters{PriceMin: priceMin.value, PriceMax: priceMax.value, Currency: *currency, RoomsMin: roomsMin.value, AreaMin: areaMin.value},
		PollInterval: *interval,
	}
	store := app.Store
	existing, found, err := store.FindWatchByName(ctx, watch.Name)
	if err != nil {
		return err
	}
	if found {
		watch.ID = existing.ID
		watch.CreatedAt = existing.CreatedAt
		if err := store.UpdateWatch(ctx, watch); err != nil {
			return err
		}
	} else {
		watch, err = store.CreateWatch(ctx, watch)
		if err != nil {
			return err
		}
	}

	report, pollErr := app.Poll(ctx, watch.ID)
	if pollErr != nil && !application.HasErrors(report) {
		return pollErr
	}
	if err := writeJSON(os.Stdout, application.View(report)); err != nil {
		return err
	}
	return pollErr
}

func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dbPath := fs.String("db", "", dbFlagHelp)
	pages := fs.Int("pages", 5, "максимум сторінок пошуку на кожен тип житла")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	app, err := openCore(ctx, *dbPath, *pages)
	if err != nil {
		return err
	}
	defer app.Close()
	return app.Run(ctx)
}

func openCore(ctx context.Context, explicit string, pages int) (*core.Core, error) {
	path, err := resolveDBPath(ctx, explicit)
	if err != nil {
		return nil, err
	}
	return core.Open(ctx, core.Options{DBPath: path, Pages: pages})
}

func cmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dbPath := fs.String("db", "", dbFlagHelp)
	limit := fs.Int("limit", 20, "скільки останніх запусків показати")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx := context.Background()
	app, err := openCore(ctx, *dbPath, 0)
	if err != nil {
		return err
	}
	defer app.Close()
	store := app.Store
	sources, err := store.ListSources(ctx)
	if err != nil {
		return err
	}
	watches, err := store.ListWatches(ctx)
	if err != nil {
		return err
	}
	runs, err := store.RecentRuns(ctx, *limit)
	if err != nil {
		return err
	}
	return writeJSON(os.Stdout, map[string]any{
		"sources": sourcesView(sources),
		"watches": watchesView(watches),
		"runs":    runsView(runs),
	})
}

func writeJSON(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func splitList(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseProperties(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return []string{domain.PropertyApartment}, nil
	}
	var out []string
	seen := map[string]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		property, err := normalizeProperty(part)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[property]; ok {
			continue
		}
		seen[property] = struct{}{}
		out = append(out, property)
	}
	return out, nil
}

func normalizeProperty(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case domain.PropertyApartment, "flat", "flats", "квартира", "квартири":
		return domain.PropertyApartment, nil
	case domain.PropertyHouse, "houses", "будинок", "будинки":
		return domain.PropertyHouse, nil
	default:
		return "", fmt.Errorf("невідомий тип житла %q", raw)
	}
}

func sourcesView(sources []domain.Source) []map[string]any {
	out := make([]map[string]any, 0, len(sources))
	for _, source := range sources {
		out = append(out, map[string]any{
			"id":            source.ID,
			"type":          source.Type,
			"enabled":       source.Enabled,
			"status":        source.Status,
			"lastSuccessAt": source.LastSuccessAt,
			"lastErrorAt":   source.LastErrorAt,
			"lastError":     source.LastError,
		})
	}
	return out
}

func watchesView(watches []domain.Watch) []map[string]any {
	out := make([]map[string]any, 0, len(watches))
	for _, watch := range watches {
		out = append(out, map[string]any{
			"id":           watch.ID,
			"name":         watch.Name,
			"enabled":      watch.Enabled,
			"sources":      watch.SourceIDs,
			"query":        watch.Query,
			"filters":      watch.Filters,
			"pollInterval": watch.PollInterval.String(),
		})
	}
	return out
}

func runsView(runs []domain.Run) []application.RunView {
	report := application.Report{Runs: runs}
	return application.View(report).Runs
}

type optionalInt64 struct{ value *int64 }

func (o *optionalInt64) String() string {
	if o.value == nil {
		return ""
	}
	return strconv.FormatInt(*o.value, 10)
}

func (o *optionalInt64) Set(raw string) error {
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return err
	}
	if n < 0 {
		return errors.New("ціна не може бути від'ємною")
	}
	o.value = &n
	return nil
}

type optionalInt struct{ value *int }

func (o *optionalInt) String() string {
	if o.value == nil {
		return ""
	}
	return strconv.Itoa(*o.value)
}

func (o *optionalInt) Set(raw string) error {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return err
	}
	if n < 1 {
		return errors.New("кількість кімнат має бути додатною")
	}
	o.value = &n
	return nil
}

type optionalFloat struct{ value *float64 }

func (o *optionalFloat) String() string {
	if o.value == nil {
		return ""
	}
	return strconv.FormatFloat(*o.value, 'f', -1, 64)
}

func (o *optionalFloat) Set(raw string) error {
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return err
	}
	if n < 0 {
		return errors.New("площа не може бути від'ємною")
	}
	o.value = &n
	return nil
}
