package domria

import "strings"

// defaultCityPaths maps a normalized city name to a DOM.RIA long-term apartment
// search. The slugs are the ones the site publishes; several big cities still
// use an older transliteration. A path (/uk/arenda-kvartir/...) bypasses this table.
var defaultCityPaths = map[string]string{
	"київ":         "/uk/arenda-kvartir/kiev/",
	"kyiv":         "/uk/arenda-kvartir/kiev/",
	"львів":        "/uk/arenda-kvartir/lvov/",
	"lviv":         "/uk/arenda-kvartir/lvov/",
	"харків":       "/uk/arenda-kvartir/kharkov/",
	"kharkiv":      "/uk/arenda-kvartir/kharkov/",
	"одеса":        "/uk/arenda-kvartir/odessa/",
	"odesa":        "/uk/arenda-kvartir/odessa/",
	"дніпро":       "/uk/arenda-kvartir/dnepr/",
	"dnipro":       "/uk/arenda-kvartir/dnepr/",
	"хмельницький": "/uk/arenda-kvartir/khmelnytskyi/",
	"khmelnytskyi": "/uk/arenda-kvartir/khmelnytskyi/",
	"кам'янець-подільський": "/uk/arenda-kvartir/kamenets-podolskyi/",
	"кам'янець подільський": "/uk/arenda-kvartir/kamenets-podolskyi/",
	"kamianets-podilskyi":   "/uk/arenda-kvartir/kamenets-podolskyi/",
	"вінниця":               "/uk/arenda-kvartir/vinnitsa/",
	"запоріжжя":             "/uk/arenda-kvartir/zaporozhye/",
	"черкаси":               "/uk/arenda-kvartir/cherkasy/",
	"чернігів":              "/uk/arenda-kvartir/chernigov/",
	"чернівці":              "/uk/arenda-kvartir/chernovtsy/",
	"івано-франківськ":      "/uk/arenda-kvartir/ivano-frankovsk/",
	"миколаїв":              "/uk/arenda-kvartir/nikolaev/",
	"полтава":               "/uk/arenda-kvartir/poltava/",
	"рівне":                 "/uk/arenda-kvartir/rovno/",
	"суми":                  "/uk/arenda-kvartir/sumy/",
	"тернопіль":             "/uk/arenda-kvartir/ternopol/",
	"ужгород":               "/uk/arenda-kvartir/uzhgorod/",
	"луцьк":                 "/uk/arenda-kvartir/lutsk/",
	"житомир":               "/uk/arenda-kvartir/zhitomir/",
	"херсон":                "/uk/arenda-kvartir/kherson/",
}

func normCity(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer("’", "'", "ʼ", "'", "`", "'", "´", "'").Replace(value)
	return strings.Join(strings.Fields(value), " ")
}
