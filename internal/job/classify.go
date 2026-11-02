package job

import (
	"encoding/json"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// ClassifierVersion is stored with every job's facets (ADR-0011). Bump it whenever
// Classify's output for the same record changes; `collector classify` then rebuilds older
// rows without re-normalising them.
const ClassifierVersion = 1

// RoleFamilies and Seniorities are every value the jobs table CHECKs allow.
var (
	RoleFamilies = []string{"backend", "frontend", "fullstack", "mobile", "platform", "data", "ai_ml",
		"security", "qa", "eng_management", "product", "design", "sales", "marketing", "customer",
		"operations", "finance", "people", "legal", "other", "unknown"}
	Seniorities = []string{"intern", "junior", "mid", "senior", "staff_plus", "lead_manager", "unknown"}
)

// Facets are what a job is filtered on (ADR-0011). "unknown" is a normal answer: nothing
// is guessed.
type Facets struct {
	RoleFamily         string
	Seniority          string
	ExperienceMinYears int      // the first explicit minimum in the description; 0 when none
	RemoteScopes       []string // every remote scope the locations name; empty when not remote
	Countries          []string // ISO 3166-1 alpha-2: the board's codes, then names in locations
}

// Classify derives a record's facets from its title, team, department and locations, and
// the posting's description text. It is pure. Titles and descriptions are untrusted: they
// are only split into words and compared with fixed tables, or scanned by fixed,
// precompiled patterns.
func Classify(r Record, description string) Facets {
	f := Facets{
		RoleFamily:         roleFamily(r),
		Seniority:          seniority(r),
		ExperienceMinYears: experience(description),
	}
	f.RemoteScopes, f.Countries = places(r)
	return f
}

// words is tokens with "/" kept as a word boundary ("Senior / Staff" is not "Senior Staff").
func words(s string) []string { return tokens(strings.ReplaceAll(s, "/", " or ")) }

// phrase is a run of words that, found in a text, means value.
type phrase struct {
	words []string
	value string
}

// table turns {value: [phrase, ...]} into phrases.
func table(m map[string][]string) []phrase {
	var out []phrase
	for v, ps := range m {
		for _, p := range ps {
			out = append(out, phrase{tokens(p), v})
		}
	}
	return out
}

// best returns the value of the longest phrase found in t; between equally long ones, the
// one found last (in English titles the head noun comes last: "Security Sales Executive").
func best(t []string, ps []phrase) (string, bool) {
	value, n, at := "", 0, -1
	for _, p := range ps {
		for i := 0; i+len(p.words) <= len(t); i++ {
			if equal(t[i:i+len(p.words)], p.words) && (len(p.words) > n || len(p.words) == n && i > at) {
				value, n, at = p.value, len(p.words), i
			}
		}
	}
	return value, n > 0
}

// has reports whether any phrase is found in t.
func has(t []string, ps []phrase) bool {
	_, ok := best(t, ps)
	return ok
}

// families maps title words to role families, built from the most common titles, teams
// and departments in the local data (2026-11-02). A generic "Software Engineer" or
// "Developer" names no family: it stays unknown.
var families = table(map[string][]string{
	"backend":   {"backend", "back end", "backend engineer", "backend developer", "back end engineer", "back end developer", "api engineer"},
	"frontend":  {"frontend", "front end", "frontend engineer", "frontend developer", "front end engineer", "front end developer", "ui engineer", "ui developer"},
	"fullstack": {"fullstack", "full stack", "fullstack engineer", "fullstack developer", "full stack engineer", "full stack developer"},
	"mobile": {"mobile", "ios", "android", "flutter", "react native", "mobile engineer", "mobile developer",
		"ios engineer", "ios developer", "android engineer", "android developer"},
	"platform": {"platform", "platform engineer", "devops", "devops engineer", "sre", "site reliability",
		"reliability engineer", "infrastructure", "data center", "cloud engineer", "network engineer", "systems support engineer", "architecte cloud"},
	"data": {"data", "data engineer", "data engineering", "data scientist", "data science", "data analyst", "analytics",
		"analytics engineer", "business intelligence", "data platform", "data platform engineer", "architecte data"},
	"ai_ml": {"ai", "ia", "ml", "mlops", "machine learning", "deep learning", "llm", "nlp", "computer vision",
		"ai engineer", "ml engineer", "machine learning engineer", "applied scientist", "research scientist"},
	"security": {"security", "securite", "infosec", "appsec", "secops", "cybersecurity", "cybersecurite",
		"security engineer", "information security", "penetration", "pentester"},
	"qa": {"qa", "qa engineer", "quality assurance", "test engineer", "test automation", "sdet"},
	"eng_management": {"engineering manager", "software engineering manager", "engineering director", "director of engineering",
		"head of engineering", "vp engineering", "vp of engineering", "engineering leader", "engineering lead", "cto", "chief technology officer"},
	"product": {"product manager", "product owner", "product management", "product lead", "director of product",
		"head of product", "vp product", "vp of product", "svp product", "chief product officer", "chef de produit"},
	// "of product design" outweighs "head of product" in "Head of Product Design".
	"design": {"design", "designer", "product design", "of product design", "product designer", "ux", "ui ux",
		"ux designer", "user experience", "experience designer"},
	"sales": {"sales", "sales development", "account executive", "account executives", "account manager", "account director",
		"business development", "business developer", "sdr", "bdr", "solutions architect", "solution architect",
		"solutions architecture", "solutions engineer", "solution engineer", "sales engineer", "pre sales", "presales",
		"deal desk", "renewals", "partnerships", "partner manager", "channel", "alliances", "key account", "territory",
		"commercial", "commerciale", "de comptes"},
	"customer": {"customer success", "customer support", "customer experience", "customer service", "support",
		"specialiste support", "technical support", "support engineer", "implementation", "implementation consultant",
		"onboarding", "customer solution architect", "partner success", "service client", "relation client"},
	"marketing": {"marketing", "product marketing", "growth marketing", "content marketing", "demand generation",
		"campaign manager", "brand", "communication", "communications", "comms", "social media", "seo", "events", "evenementiel"},
	"operations": {"operations", "ops", "business operations", "revenue operations", "program manager", "programme manager",
		"project manager", "strategy", "strategie", "chief of staff", "procurement", "office manager", "workplace",
		"supply chain", "logistics"},
	"finance": {"finance", "financial", "accounting", "accountant", "comptable", "comptabilite", "fp a", "controller",
		"controlling", "treasury", "audit", "auditor", "corporate development"},
	"people": {"people", "hr", "rh", "human resources", "ressources humaines", "hr business partner", "people partner",
		"recruiter", "recruiting", "recruitment", "talent", "talent acquisition", "payroll", "learning and development"},
	"legal": {"legal", "counsel", "lawyer", "paralegal", "juriste", "avocat", "compliance", "conformite", "privacy"},
})

// spontaneous marks an open application: only the words before it name a role, and the
// team or department (a company unit) is not used.
var spontaneous = table(map[string][]string{"": {"candidature spontanee", "spontaneous application", "open application"}})

func roleFamily(r Record) string {
	t := words(r.Title)
	for i := range t {
		if has(t[i:i+min(2, len(t)-i)], spontaneous) {
			if f, ok := best(t[:i], families); ok {
				return f
			}
			return "unknown"
		}
	}
	for _, s := range []string{r.Title, r.Team, r.Department} {
		if f, ok := best(words(s), families); ok {
			return f
		}
	}
	return "unknown"
}

var (
	interns = table(map[string][]string{"": {"intern", "interns", "internship", "stage", "stagiaire", "alternance",
		"alternant", "alternante", "apprenti", "apprentie", "apprentice", "apprenticeship", "trainee",
		"working student", "werkstudent", "praktikum", "co op"}})
	// People managers and leads. A bare "manager" counts only first ("Manager, Sales
	// Development"): "Product Manager" or "Customer Success Manager" says nothing of seniority.
	leads = table(map[string][]string{"": {"lead", "leader", "head", "director", "directeur", "directrice", "responsable",
		"vp", "svp", "evp", "vice president", "chief", "cto", "ceo", "cfo", "coo", "cpo", "ciso",
		"senior manager", "engineering manager", "sales manager", "general manager", "country manager", "managing director"}})
	staffRuns = table(map[string][]string{"": {"senior staff", "senior principal"}})
	// The individual-contributor ladder, lowest first; a title naming two ("Senior / Staff")
	// is open at the lower one.
	ladder = []struct {
		level string
		words []phrase
	}{
		{"junior", table(map[string][]string{"": {"junior", "jr", "graduate", "grad", "entry level", "debutant", "debutante"}})},
		{"mid", table(map[string][]string{"": {"mid", "mid level", "intermediate", "medior"}})},
		{"senior", table(map[string][]string{"": {"senior", "sr", "snr"}})},
		{"staff_plus", table(map[string][]string{"": {"staff", "principal", "distinguished"}})},
	}
	// Level numerals ("Engineer II") count only when no word does.
	numerals = table(map[string][]string{"mid": {"ii"}, "senior": {"iii"}})
	// Teams and departments that say a seniority ("University", "Early Careers").
	units = table(map[string][]string{"intern": {"university", "interns", "internships", "campus"},
		"junior": {"early careers", "early career", "graduates"}})
)

func seniority(r Record) string {
	t := words(r.Title)
	switch {
	case has(t, interns):
		return "intern"
	case has(t, leads) || len(t) > 0 && t[0] == "manager":
		return "lead_manager"
	case has(t, staffRuns):
		return "staff_plus"
	}
	for _, l := range ladder {
		if has(t, l.words) {
			return l.level
		}
	}
	if v, ok := best(t, numerals); ok {
		return v
	}
	for _, s := range []string{r.Team, r.Department} {
		if v, ok := best(words(s), units); ok {
			return v
		}
	}
	return "unknown"
}

// experienceRe finds an explicit minimum: "5+ years", "5-8 years" (the lower bound), "at
// least 6 years", "minimum de 5 ans", "10 years of experience", "3 ans d'expérience".
// Bare "N years" is not enough ("founded 10 years ago", "5 years of growth"), and the
// singular is not a requirement ("accounts for 16-17 year olds").
var experienceRe = regexp.MustCompile(`(?i)` +
	`\b(\d{1,2})\s*\+\s*(?:years|yrs|ans)\b` +
	`|\b(\d{1,2})\s*(?:-|–|—|to|à)\s*\d{1,2}\s*\+?\s*(?:years|yrs|ans)\b` +
	`|\b(?:at least|minimum(?: of| de)?|au moins)\s+(\d{1,2})\s*(?:years?|yrs?|ans)\b` +
	`|\b(\d{1,2})\s*(?:years?|yrs?)\s+(?:of\s+)?(?:[\w-]+\s+){0,3}experience` +
	`|\b(\d{1,2})\s*ans\s+d['’]\s*exp[ée]rience`)

// experience returns the first explicit minimum (descriptions state the main requirement
// first), ignoring 0 and anything over 20 (company age: "For 30+ years, we've delivered");
// 0 when there is none.
func experience(desc string) int {
	for _, m := range experienceRe.FindAllStringSubmatch(desc, -1) {
		for _, g := range m[1:] {
			if n, err := strconv.Atoi(g); err == nil && n >= 1 && n <= 20 {
				return n
			}
		}
	}
	return 0
}

// remoteWords mark a location as remote.
var remoteWords = table(map[string][]string{"": {"remote", "remotely", "anywhere", "distributed", "home based",
	"homebased", "work from home", "wfh", "teletravail", "worldwide"}})

// geo maps region words to "r:<scope>" and country names to "c:<code>" (a fixed table:
// English and French names). Cities are not here: presets match them in locations.
var geo = table(map[string][]string{
	"r:worldwide": {"worldwide", "global", "anywhere"},
	"r:emea":      {"emea", "emear"},
	"r:europe":    {"europe", "european", "eu"},
	"r:africa":    {"africa", "afrique"},
	"r:americas":  {"americas", "amer", "latam", "north america", "south america", "latin america", "na", "namer"},
	"r:apac":      {"apac", "asia pacific", "apj", "apjc", "anz", "aunz", "asia"},
	"c:US": {"united states", "united states of america", "usa", "us", "etats unis",
		// states (not Georgia: also a country), so "Remote - Texas" is US-only
		"alabama", "alaska", "arizona", "arkansas", "california", "colorado", "connecticut", "delaware",
		"florida", "hawaii", "idaho", "illinois", "indiana", "iowa", "kansas", "kentucky", "louisiana", "maine",
		"maryland", "massachusetts", "michigan", "minnesota", "mississippi", "missouri", "montana", "nebraska",
		"nevada", "new hampshire", "new jersey", "new mexico", "new york", "north carolina", "north dakota",
		"ohio", "oklahoma", "oregon", "pennsylvania", "rhode island", "south carolina", "south dakota",
		"tennessee", "texas", "utah", "vermont", "virginia", "washington", "west virginia", "wisconsin", "wyoming"},
	"c:CA": {"canada"}, "c:MX": {"mexico", "mexique"}, "c:BR": {"brazil", "brasil", "bresil"},
	"c:AR": {"argentina", "argentine"}, "c:CO": {"colombia", "colombie"}, "c:CL": {"chile", "chili"},
	"c:PE": {"peru", "perou"}, "c:UY": {"uruguay"}, "c:CR": {"costa rica"},
	"c:GB": {"united kingdom", "uk", "great britain", "england", "scotland", "wales", "northern ireland", "royaume uni"},
	"c:IE": {"ireland", "republic of ireland", "irlande"}, "c:FR": {"france"},
	"c:DE": {"germany", "deutschland", "allemagne"}, "c:ES": {"spain", "espana", "espagne"},
	"c:PT": {"portugal"}, "c:IT": {"italy", "italia", "italie"},
	"c:NL": {"netherlands", "the netherlands", "pays bas", "holland"}, "c:BE": {"belgium", "belgique", "belgie"},
	"c:LU": {"luxembourg"}, "c:CH": {"switzerland", "suisse", "schweiz"}, "c:AT": {"austria", "osterreich", "autriche"},
	"c:DK": {"denmark", "danemark"}, "c:SE": {"sweden", "suede", "sverige"}, "c:NO": {"norway", "norvege"},
	"c:FI": {"finland", "finlande"}, "c:IS": {"iceland", "islande"}, "c:LI": {"liechtenstein"},
	"c:PL": {"poland", "pologne", "polska"}, "c:CZ": {"czech republic", "czechia"}, "c:SK": {"slovakia"},
	"c:SI": {"slovenia"}, "c:HU": {"hungary", "hongrie"}, "c:RO": {"romania", "roumanie"},
	"c:BG": {"bulgaria", "bulgarie"}, "c:HR": {"croatia", "croatie"}, "c:GR": {"greece", "grece"},
	"c:CY": {"cyprus", "chypre"}, "c:MT": {"malta", "malte"}, "c:EE": {"estonia", "estonie"},
	"c:LV": {"latvia", "lettonie"}, "c:LT": {"lithuania", "lituanie"}, "c:RS": {"serbia", "serbie"},
	"c:UA": {"ukraine"}, "c:TR": {"turkey", "turkiye", "turquie"},
	"c:MA": {"morocco", "maroc"}, "c:TN": {"tunisia", "tunisie"}, "c:DZ": {"algeria", "algerie"},
	"c:EG": {"egypt", "egypte"}, "c:ZA": {"south africa", "afrique du sud"}, "c:NG": {"nigeria"},
	"c:KE": {"kenya"}, "c:GH": {"ghana"},
	"c:AE": {"united arab emirates", "uae", "emirats arabes unis"}, "c:SA": {"saudi arabia", "ksa", "arabie saoudite"},
	"c:QA": {"qatar"}, "c:IL": {"israel"},
	"c:IN": {"india", "inde"}, "c:SG": {"singapore", "singapour"}, "c:JP": {"japan", "japon"},
	"c:CN": {"china", "chine"}, "c:HK": {"hong kong"}, "c:TW": {"taiwan"}, "c:KR": {"south korea", "korea", "coree du sud"},
	"c:AU": {"australia", "australie"}, "c:NZ": {"new zealand", "nouvelle zelande"}, "c:TH": {"thailand", "thailande"},
	"c:VN": {"vietnam", "viet nam"}, "c:ID": {"indonesia", "indonesie"}, "c:MY": {"malaysia", "malaisie"},
	"c:PH": {"philippines"}, "c:PK": {"pakistan"},
})

// geoIn returns the geo values found in t, left to right, the longest phrase at each word.
func geoIn(t []string) []string {
	var out []string
	for i := 0; i < len(t); {
		v, n := "", 0
		for _, p := range geo {
			if len(p.words) > n && i+len(p.words) <= len(t) && equal(t[i:i+len(p.words)], p.words) {
				v, n = p.value, len(p.words)
			}
		}
		if n == 0 {
			i++
			continue
		}
		out = append(out, v)
		i += n
	}
	return out
}

// countryScope is the remote scope of a country code: the US is "us_only".
func countryScope(code string) string {
	if code == "US" {
		return "us_only"
	}
	return "country:" + code
}

// places returns the remote scopes and countries the record's locations name. Every
// location of a remote job is a remote location; otherwise only "; "-separated segments
// with a remote word are ("Home Based - EMEA; Office Based - London, UK").
func places(r Record) (scopes, countries []string) {
	for _, c := range r.CountryCodes {
		countries = addLocation(countries, c)
	}
	for _, loc := range r.Locations {
		for _, seg := range strings.Split(loc, ";") {
			t := words(seg)
			remote := r.Workplace == Remote || has(t, remoteWords)
			var segScopes []string
			for _, v := range geoIn(t) {
				switch v[:2] {
				case "c:":
					countries = addLocation(countries, v[2:])
					segScopes = append(segScopes, countryScope(v[2:]))
				case "r:":
					segScopes = append(segScopes, v[2:])
				}
			}
			if !remote {
				continue
			}
			if len(segScopes) == 0 {
				segScopes = []string{"unknown"}
				if len(r.CountryCodes) == 1 { // Lever: "Remote" with the posting's country
					segScopes = []string{countryScope(r.CountryCodes[0])}
				}
			}
			for _, s := range segScopes {
				scopes = addLocation(scopes, s)
			}
		}
	}
	if r.Workplace == Remote && len(scopes) == 0 {
		scopes = []string{"unknown"}
		if len(r.CountryCodes) == 1 {
			scopes = []string{countryScope(r.CountryCodes[0])}
		}
	}
	return scopes, countries
}

// Description returns a posting's requirement text as plain text: Lever's description,
// lists and closing text; Greenhouse's HTML-escaped content; Ashby's descriptionPlain. ""
// when the payload is unreadable. Never logged.
func Description(source string, payload []byte) string {
	var m fields
	if err := json.Unmarshal(payload, &m); err != nil || m == nil {
		return ""
	}
	switch source {
	case "lever":
		parts := []string{text(m, "descriptionPlain")}
		for _, e := range m.arr("lists") {
			l := elemObj(e)
			parts = append(parts, text(l, "text"), plain(text(l, "content")))
		}
		return strings.Join(append(parts, text(m, "additionalPlain")), "\n")
	case "greenhouse":
		return plain(plain(text(m, "content")))
	case "ashby":
		return text(m, "descriptionPlain")
	}
	return ""
}

// text returns m[k] as a string, uncapped (unlike str: descriptions are long).
func text(m fields, k string) string {
	var s string
	_ = json.Unmarshal(m[k], &s) // wrong type or missing: empty
	return s
}

var tagRe = regexp.MustCompile(`<[^>]*(?:>|$)`) // an unclosed tag runs to the end

// plain unescapes HTML entities and replaces tags with spaces.
func plain(s string) string {
	return tagRe.ReplaceAllString(html.UnescapeString(s), " ")
}
