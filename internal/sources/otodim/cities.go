package otodim

import "strings"

// cityLocations maps a normalized city name to its place in Otodim's location
// tree: oblast, raion, hromada, city. A shorter slug such as "kyiv" redirects
// to the whole of Ukraine.
var cityLocations = map[string]string{
	"київ":         "kyiv/kyiv/kyiv/kyiv",
	"kyiv":         "kyiv/kyiv/kyiv/kyiv",
	"львів":        "lvivska-oblast/lvivskyi-raion/lvivska-hromada/lviv",
	"lviv":         "lvivska-oblast/lvivskyi-raion/lvivska-hromada/lviv",
	"харків":       "kharkivska-oblast/kharkivskyi-raion/kharkivska-hromada/kharkiv",
	"kharkiv":      "kharkivska-oblast/kharkivskyi-raion/kharkivska-hromada/kharkiv",
	"одеса":        "odeska-oblast/odeskyi-raion/odeska-hromada/odesa",
	"odesa":        "odeska-oblast/odeskyi-raion/odeska-hromada/odesa",
	"дніпро":       "dnipropetrovska-oblast/dniprovskyi-raion/dniprovska-hromada/dnipro",
	"dnipro":       "dnipropetrovska-oblast/dniprovskyi-raion/dniprovska-hromada/dnipro",
	"хмельницький": "khmelnytska-oblast/khmelnytskyi-raion/khmelnytska-hromada/khmelnytskyi",
	"khmelnytskyi": "khmelnytska-oblast/khmelnytskyi-raion/khmelnytska-hromada/khmelnytskyi",
	"кам'янець-подільський": "khmelnytska-oblast/kamianets--podilskyi-raion/kamianets--podilska-hromada/kamianets--podilskyi",
	"кам'янець подільський": "khmelnytska-oblast/kamianets--podilskyi-raion/kamianets--podilska-hromada/kamianets--podilskyi",
	"kamianets-podilskyi":   "khmelnytska-oblast/kamianets--podilskyi-raion/kamianets--podilska-hromada/kamianets--podilskyi",
	"вінниця":               "vinnytska-oblast/vinnytskyi-raion/vinnytska-hromada/vinnytsia",
	"запоріжжя":             "zaporizka-oblast/zaporizkyi-raion/zaporizka-hromada/zaporizhzhia",
	"черкаси":               "cherkaska-oblast/cherkaskyi-raion/cherkaska-hromada/cherkasy",
	"чернігів":              "chernihivska-oblast/chernihivskyi-raion/chernihivska-hromada/chernihiv",
	"чернівці":              "chernivetska-oblast/chernivetskyi-raion/chernivetska-hromada/chernivtsi",
	"івано-франківськ":      "ivano--frankivska-oblast/ivano--frankivskyi-raion/ivano--frankivska-hromada/ivano--frankivsk",
	"миколаїв":              "mykolaivska-oblast/mykolaivskyi-raion/mykolaivska-hromada/mykolaiv",
	"полтава":               "poltavska-oblast/poltavskyi-raion/poltavska-hromada/poltava",
	"рівне":                 "rivnenska-oblast/rivnenskyi-raion/rivnenska-hromada/rivne",
	"суми":                  "sumska-oblast/sumskyi-raion/sumska-hromada/sumy",
	"тернопіль":             "ternopilska-oblast/ternopilskyi-raion/ternopilska-hromada/ternopil",
	"ужгород":               "zakarpatska-oblast/uzhhorodskyi-raion/uzhhorodska-hromada/uzhhorod",
	"луцьк":                 "volynska-oblast/lutskyi-raion/lutska-hromada/lutsk",
	"житомир":               "zhytomyrska-oblast/zhytomyrskyi-raion/zhytomyrska-hromada/zhytomyr",
	"херсон":                "khersonska-oblast/khersonskyi-raion/khersonska-hromada/kherson",
}

func normCity(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer("’", "'", "ʼ", "'", "`", "'", "´", "'").Replace(value)
	return strings.Join(strings.Fields(value), " ")
}
