package lun

import "strings"

// defaultCityPaths maps a normalized city name to a LUN long-term rent search.
// Secondary cities are not /rent/{city}/flats; Kamianets-Podilskyi lives under
// the Khmelnytskyi search. A path (/rent/...) bypasses this table.
var defaultCityPaths = map[string]string{
	"київ":         "/rent/kyiv/flats",
	"kyiv":         "/rent/kyiv/flats",
	"львів":        "/rent/lviv/flats",
	"lviv":         "/rent/lviv/flats",
	"харків":       "/rent/kharkiv/flats",
	"kharkiv":      "/rent/kharkiv/flats",
	"одеса":        "/rent/odesa/flats",
	"odesa":        "/rent/odesa/flats",
	"дніпро":       "/rent/dnipro/flats",
	"dnipro":       "/rent/dnipro/flats",
	"хмельницький": "/rent/khmelnytskyi/flats",
	"khmelnytskyi": "/rent/khmelnytskyi/flats",
	"кам'янець-подільський": "/rent/khmelnytskyi/flats-kamianets-podilskyi",
	"кам'янець подільський": "/rent/khmelnytskyi/flats-kamianets-podilskyi",
	"kamianets-podilskyi":   "/rent/khmelnytskyi/flats-kamianets-podilskyi",
	"вінниця":               "/rent/vinnytsia/flats",
	"запоріжжя":             "/rent/zp/flats",
	"черкаси":               "/rent/cherkasy/flats",
	"чернігів":              "/rent/chernihiv/flats",
	"чернівці":              "/rent/chernivtsi/flats",
	"івано-франківськ":      "/rent/if/flats",
	"миколаїв":              "/rent/mykolaiv/flats",
	"полтава":               "/rent/poltava/flats",
	"рівне":                 "/rent/rivne/flats",
	"суми":                  "/rent/sumy/flats",
	"тернопіль":             "/rent/ternopil/flats",
	"ужгород":               "/rent/uz/flats",
	"луцьк":                 "/rent/volyn/flats",
	"житомир":               "/rent/zhytomyr/flats",
	"херсон":                "/rent/kherson/flats",
	"кривий ріг":            "/rent/kr/flats",
}

func normCity(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer("’", "'", "ʼ", "'", "`", "'", "´", "'").Replace(value)
	return strings.Join(strings.Fields(value), " ")
}
