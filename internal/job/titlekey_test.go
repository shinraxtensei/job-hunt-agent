package job_test

import (
	"strings"
	"testing"

	"github.com/shinraxtensei/job-hunt-agent/internal/job"
)

type side struct {
	title string
	locs  []string
}

// AC1 + AC3: pairs of titles from the same company, grouped or kept apart. "real" marks
// pairs taken from the local database on 2026-11-02 (company in the name).
var pairs = []struct {
	name string
	a, b side
	same bool
}{
	// Same role, posted once per place: the suffix is a place in the record's locations.
	{"real ClickHouse: country suffixes", side{"Solutions Architect - China", []string{"China"}}, side{"Solutions Architect - Germany", []string{"Germany"}}, true},
	{"real ClickHouse: 'The Netherlands' location", side{"Solutions Architect - Netherlands", []string{"The Netherlands"}}, side{"Solutions Architect - Spain", []string{"Spain"}}, true},
	{"real ClickHouse: city location", side{"Solutions Architect - Mexico", []string{"Mexico City"}}, side{"Solutions Architect - UAE", []string{"UAE"}}, true},
	{"real Elastic: bare vs country", side{"Mid Market Account Executive", []string{"Germany"}}, side{"Mid Market Account Executive - Japan", []string{"Tokyo, Japan"}}, true},
	{"real Kong: bare vs country", side{"Enterprise Account Executive", []string{"Dubai, United Arab Emirates"}}, side{"Enterprise Account Executive - Italy", []string{"Italy"}}, true},
	{"real Kong: region words", side{"Enterprise Account Executive, France", []string{"France"}}, side{"Enterprise Account Executive - LATAM", []string{"Texas, United States"}}, true},
	{"real Kong: Benelux region", side{"Enterprise Account Executive, Benelux", []string{"Netherlands"}}, side{"Enterprise Account Executive", []string{"London, England, United Kingdom"}}, true},
	{"real Supabase: regions in parentheses", side{"Support Engineer (AMER)", nil}, side{"Support Engineer (APAC)", nil}, true},
	{"story: work-mode suffixes", side{"Backend Engineer – Berlin", []string{"Berlin, Germany"}}, side{"Backend Engineer | Hybrid", []string{"Berlin, Germany"}}, true},
	{"story: place in parentheses", side{"Backend Engineer (Berlin)", []string{"Berlin"}}, side{"Backend Engineer – Remote", []string{"Remote (EU)"}}, true},
	{"real Pennylane FR: remote phrase + city", side{"Spécialiste Support Pilotage et Comptabilité (Télétravail complet possible - Tours)", []string{"Tours"}}, side{"Spécialiste Support Pilotage et Comptabilité (Télétravail complet possible depuis Paimpol)", []string{"Paimpol"}}, true},
	{"real Pennylane FR: city suffix", side{"Freelance - Consultant.e en Formation - Bordeaux", []string{"Bordeaux"}}, side{"Freelance - Consultant.e en Formation - Nantes", []string{"Nantes"}}, true},
	{"real Pennylane: remote region and emoji", side{"Product Designer (Confirmed/Senior) - Remote Europe 🇪🇺", []string{"Paris"}}, side{"Product Designer (Confirmed/Senior)", []string{"Madrid"}}, true},
	{"real Ippon FR: gender marker", side{"Architecte d'Entreprise (H/F)", nil}, side{"Architecte d'Entreprise", nil}, true},
	{"real Ippon FR: gender markers differ", side{"Architecte SI H/F", nil}, side{"Architecte SI (F/H)", nil}, true},
	{"FR: accents folded", side{"Ingénieur Logiciel Sénior (Paris)", []string{"Paris"}}, side{"ingenieur logiciel senior", []string{"Lyon"}}, true},
	{"FR: inclusive writing", side{"Développeur·se Backend (F/H) - Télétravail", nil}, side{"Developpeur.se backend", nil}, true},
	{"DE: m/w/d", side{"Backend Engineer (m/w/d) – Remote", nil}, side{"Backend Engineer", []string{"Berlin"}}, true},
	{"exact repeat, different city", side{"Data Engineer", []string{"Paris"}}, side{"Data Engineer", []string{"Lisbon"}}, true},

	// Different roles: seniority, stack, team, territory, specialisation.
	{"story: seniority kept", side{"Senior Backend Engineer", nil}, side{"Backend Engineer", nil}, false},
	{"seniority in parentheses kept", side{"(Senior) iOS Engineer", nil}, side{"iOS Engineer", nil}, false},
	{"story: stack", side{"Backend Engineer, Go", nil}, side{"Backend Engineer, Java", nil}, false},
	{"real Adyen: team", side{"Engineering Manager - Android", []string{"Amsterdam"}}, side{"Engineering Manager - Bank Payments", []string{"Amsterdam"}}, false},
	{"real Stripe: team after comma", side{"Staff Engineer, Compute Foundation", []string{"Seattle"}}, side{"Staff Engineer, Core Infrastructure", []string{"Seattle"}}, false},
	{"real Theodo FR: entity", side{"Senior Key Account Manager - CDI Paris - Theodo Data & IA", []string{"Paris"}}, side{"Senior Key Account Manager - CDI Paris - Theodo FinTech", []string{"Paris"}}, false},
	{"real Theodo FR: entity vs bare entity", side{"Directeur de programme - CDI Paris - Theodo", []string{"Paris"}}, side{"Directeur de programme - CDI Paris - Theodo RetailTech", []string{"Paris"}}, false},
	{"real Elastic: segment", side{"Mid Market Account Executive", []string{"United States"}}, side{"Mid Market Account Executive, Pursuit", []string{"United States"}}, false},
	{"real Elastic: territory", side{"Mid Market Account Executive - West", []string{"United States"}}, side{"Mid Market Account Executive", []string{"United States"}}, false},
	{"real Kong: public sector", side{"Enterprise Account Executive - Public Sector", []string{"Washington D.C., United States"}}, side{"Enterprise Account Executive", []string{"Italy"}}, false},
	{"real Airwallex: specialisation", side{"Solutions Architect - Payments", []string{"Singapore"}}, side{"Solutions Architect - Payments (pre-sales)", []string{"Singapore"}}, false},
	{"real SumUp: product area", side{"Senior Product Designer", []string{"Berlin"}}, side{"Senior Product Designer - Sell Online", []string{"Berlin"}}, false},
	{"real Supabase: weekend shift", side{"Support Engineer (EMEA)", nil}, side{"Support Engineer (EMEA - Weekends)", nil}, false},
	{"real Pennylane FR: different role", side{"Spécialiste Support Pilotage et Comptabilité (Télétravail complet possible - Tours)", []string{"Tours"}}, side{"Spécialiste Support Comptable (Télétravail complet possible depuis Amiens)", []string{"Amiens"}}, false},
	{"short token not matched inside a word", side{"Product Engineer - AI", []string{"Dubai"}}, side{"Product Engineer", []string{"Dubai"}}, false},

	// Known misses (ADR-0009): a place not in the record's own locations is kept.
	{"real Kong: city not in locations", side{"Enterprise Account Executive - Chicago", []string{"Illinois, United States"}}, side{"Enterprise Account Executive", []string{"Italy"}}, false},
	{"real ClickHouse: region not in locations", side{"Solutions Architect - Pacific Northwest", []string{"Seattle"}}, side{"Solutions Architect - China", []string{"China"}}, false},
}

func TestTitleKeyPairs(t *testing.T) {
	if len(pairs) < 25 {
		t.Fatalf("pair table has %d pairs, the story asks for at least 25", len(pairs))
	}
	for _, p := range pairs {
		t.Run(p.name, func(t *testing.T) {
			ka, kb := job.TitleKey(p.a.title, p.a.locs), job.TitleKey(p.b.title, p.b.locs)
			if (ka == kb) != p.same {
				t.Errorf("%q → %q\n%q → %q\nwant same=%v", p.a.title, ka, p.b.title, kb, p.same)
			}
			if ka == "" || kb == "" {
				t.Errorf("empty key: %q → %q, %q → %q", p.a.title, ka, p.b.title, kb)
			}
		})
	}
}

// Exact keys for a few titles, so the normal form itself is pinned.
func TestTitleKeyForm(t *testing.T) {
	for title, want := range map[string]string{
		"Senior  Backend Engineer, Go!": "senior backend engineer go",
		"Ingénieur·e Données (H/F)":     "ingenieur e donnees",
		"Solutions Architect - Germany": "solutions architect germany", // no locations given: kept
		"Support Engineer (AMER)":       "support engineer",
		"Remote":                        "remote", // a title is never stripped to nothing
		"  ":                            "",
		"Œuvre d'art – Hybride":         "oeuvre d art",
		"Staff Engineer | Remote — EMEA / Worldwide": "staff engineer",
	} {
		if got := job.TitleKey(title, nil); got != want {
			t.Errorf("TitleKey(%q) = %q, want %q", title, got, want)
		}
	}
}

// Untrusted titles: regex metacharacters are ordinary text, nothing panics.
func FuzzTitleKey(f *testing.F) {
	for _, p := range pairs {
		f.Add(p.a.title, strings.Join(p.a.locs, "\n"))
	}
	f.Add(`(.*)+[a-z]{999}\\`, `.*`)
	f.Fuzz(func(t *testing.T, title, locs string) {
		k := job.TitleKey(title, strings.Split(locs, "\n"))
		if k != strings.TrimSpace(k) || strings.Contains(k, "  ") {
			t.Fatalf("key %q is not collapsed", k)
		}
		if k == "" && strings.TrimSpace(title) != "" && hasAlnum(title) {
			t.Fatalf("title %q with letters gave an empty key", title)
		}
	})
}

func hasAlnum(s string) bool {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return true
		}
	}
	return false
}
