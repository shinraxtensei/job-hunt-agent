package job_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/shinraxtensei/job-hunter/internal/job"
)

// AC2: role family and seniority from title, then team/department. Every title is a real
// one from the local database (2026-11-02), team and department as stored with it.
func TestClassifyRoleAndSeniority(t *testing.T) {
	for _, c := range []struct {
		title, team, dept string
		family, seniority string
	}{
		// Engineering
		{"Senior Backend Engineer - ClickPipes Platform", "ClickPipes", "Engineering", "backend", "senior"},
		{"(Senior) Backend Engineer (Golang) - Bank", "", "Backend", "backend", "senior"},
		{"Staff Backend Engineer, Database Automation (Go)", "", "Data Engineering", "backend", "staff_plus"},
		{"Senior / Staff Fullstack Engineer", "Engineering", "Product", "fullstack", "senior"},
		{"Lead Fullstack Developer (.NET/React)", "", "1S_TWAUS DEC Region", "fullstack", "lead_manager"},
		{"Développeur/se Fullstack - CDI Paris - Theodo GovTech", "", "", "fullstack", "unknown"},
		{"Healthcare Software Engineer Fullstack - CDI Paris - Theodo HealthTech", "Software Engineering", "Theodo HealthTech", "fullstack", "unknown"},
		{"Software Engineer Intern - Fullstack (x/f/m) - Berlin - March 2027", "", "Engineering", "fullstack", "intern"},
		{"Software Engineer Fullstack - Stage de fin d'études Paris - Theodo", "", "", "fullstack", "intern"},
		{"UI Engineer II", "Application", "Engineering", "frontend", "mid"},
		{"iOS Engineer - Design System", "", "Engineering", "mobile", "unknown"},
		{"Infrastructure Engineer", "", "1S_TWSGP DEC Region", "platform", "unknown"},
		{"Site Reliability Engineer II", "", "Product Development", "platform", "mid"},
		{"Lead SRE / DevOps - CDI Paris - Theodo Cloud", "", "", "platform", "lead_manager"},
		{"SRE / DevOps - Stage de fin d'études Paris -  Theodo Cloud", "", "", "platform", "intern"},
		{"Senior Platform Engineer - Container Services", "", "Infrastructure", "platform", "senior"},
		{"Senior Software Engineer - Dev Platform", "AI & Machine Learning", "Engineering", "platform", "senior"},
		{"Staff Engineer, Network Infrastructure", "", "Core Cloud", "platform", "staff_plus"},
		{"Architecte Cloud & DevOps - F/H", "", "", "platform", "unknown"},
		{"Graduate Systems Support Engineer", "", "1Z_TWBRA SL DAMO", "platform", "junior"},
		{"Data Platform Engineer", "", "Data Engineering", "data", "unknown"},
		{"Senior Data Engineer (San Francisco)", "Data Science", "Engineering", "data", "senior"},
		{"Data Analyst II", "", "Product Development", "data", "mid"},
		{"PhD Data Scientist, Intern", "", "5112 General University", "data", "intern"},
		{"Architecte Data (H/F)", "", "", "data", "unknown"},
		{"AI Engineer", "", "1155 SA Transformation", "ai_ml", "unknown"},
		{"Agentic AI Principal (m/f/d)", "", "1Y_TWDEU SL DATA", "ai_ml", "staff_plus"},
		{"Consultant Senior AI Advisory - CDI Paris - Theodo Data & AI", "Consultant", "Advisory AI", "ai_ml", "senior"},
		{"Security Engineer, Application Security", "Security", "Security", "security", "unknown"},
		{"Senior Information Security Infrastructure Engineer - Security Architecture - InfoSec", "", "InfoSec", "security", "senior"},
		{"Network Security Engineer", "", "Customer Support", "security", "unknown"},
		{"Architecte sécurité Cloud (H/F)", "", "", "security", "unknown"},
		{"SecOps Engineer - Stage Paris - Theodo Cloud", "", "", "security", "intern"},
		{"Intern, Frontend QA Engineering", "", "Engineering & Technology", "qa", "intern"},
		{"Software Engineering Manager - Global UX - Applications - Platform", "", "Platform - Cross Team", "eng_management", "lead_manager"},
		{"Machine Learning Engineering Manager, Credit & Underwriting", "", "Development", "eng_management", "lead_manager"},
		{"Engineering Leader -  Payments APAC", "", "Development", "eng_management", "lead_manager"},
		{"Engineering Lead, Growth", "Growth", "Engineering", "eng_management", "lead_manager"},
		{"CTO Cloud - CDI Paris - Theodo Cloud - H/F", "", "", "eng_management", "lead_manager"},
		// Generic engineering titles: no family is guessed.
		{"Senior Developer", "", "1S_TWCNO DEC Region", "unknown", "senior"},
		{"Associate (Graduate) Developer", "", "1S_TWCHL DEC Region", "unknown", "junior"},
		{"Principal Forward-Deployed Engineer", "", "14_TWESP PS - CXPD Service Development", "unknown", "staff_plus"},
		{"Principal Software Engineer - Postgres", "Engineering", "Engineering", "unknown", "staff_plus"},
		{"Integration Engineer", "", "1143 EMEA Delivery", "unknown", "unknown"},
		{"Senior Software Engineer I, Compute", "", "", "unknown", "senior"},
		// Department fallback when the title says nothing.
		{"Principal Software Engineer - Performance Tuning - Elasticsearch", "", "Platform - Elasticsearch", "platform", "staff_plus"},
		// Product and design
		{"Principal Product Manager (Consulting & Banking)", "", "1W_UKSTH SL CXPD", "product", "staff_plus"},
		{"Principal Product Manager, Security", "Product Management", "Product", "product", "staff_plus"},
		{"Product Manager, Billing (Senior Staff)", "Product", "All Cost Center", "product", "staff_plus"},
		{"Director of Product, ClickHouse Cloud", "Product Management", "Product", "product", "lead_manager"},
		{"SVP, Product - US-Based", "Executive", "Product Management", "product", "lead_manager"},
		{"Product Manager Intern (2027) - Lisbon, PT", "", "University", "product", "intern"},
		{"Associate Product Manager - Stage de fin d'études Paris - Theodo Healthtech", "", "", "product", "intern"},
		{"Senior Technical Program Manager, AI Platform", "Office of the CTO", "Office of the CTO", "operations", "senior"},
		{"Head of Product Design", "Design", "Design", "design", "lead_manager"},
		{"Lead Experience Designer", "", "1W_TWSGP SL CXPD", "design", "lead_manager"},
		{"Staff Product Designer, Console Experience", "", "Core Cloud", "design", "staff_plus"},
		// Sales, including Stripe cost-centre departments
		{"Sales Development Representative - Barcelona (Hybrid)", "", "0000: Pipeline", "sales", "unknown"},
		{"Manager, Sales Development", "", "1130 Global Sales Development Mgrs", "sales", "lead_manager"},
		{"Account Executive, Platforms (Hunter)", "", "1185 Account Executives (EMEA)", "sales", "unknown"},
		{"Account Executive, Commercial (Grower)", "", "1651 Velocity - Account Executives (NA)", "sales", "unknown"},
		{"Account Executive, AI Sales", "", "1654 Account Executives (AI)", "sales", "unknown"},
		{"Specialist Solutions Architect, Tax", "", "1150 Solutions Architecture", "sales", "unknown"},
		{"Head of Specialist Solutions Architecture, Revenue Suite (AMER)", "", "1154 Specialized Solutions Architecture", "sales", "lead_manager"},
		{"Solutions Engineer, AI Build Team", "", "1650 AI GTM Strategy & Solutions", "sales", "unknown"},
		{"Staff Solutions Engineer - Central", "Sales Eng", "All Cost Center", "sales", "staff_plus"},
		{"Security Sales Executive", "", "Sales - AMER - Security Specialists", "sales", "unknown"},
		{"Territory Account Executive, Qatar", "", "Mid Market", "sales", "unknown"},
		{"Regional Sales Director", "", "EMEA - Enterprise", "sales", "lead_manager"},
		{"Renewals Analyst", "", "1341 Renewals", "sales", "unknown"},
		{"Conseiller(ère) commercial(e) terrain expérimenté(e) indépendant", "", "Field Sales", "sales", "unknown"},
		{"Chargé(e) de comptes - Opticien / Audioprothésiste (x/f/m)", "", "Sales", "sales", "unknown"},
		// Customer success and support
		{"Customer Success Manager", "", "1340 Customer Success Management", "customer", "unknown"},
		{"Graduate Customer Success Manager", "", "Operations", "customer", "junior"},
		{"Customer Success Manager Grands Comptes - Cabinets comptables H/F (en télétravail depuis Nantes)", "", "", "customer", "unknown"},
		{"Spécialiste Support Pilotage et Comptabilité (Télétravail complet possible - Béziers)", "Customer Support", "Customer Success", "customer", "unknown"},
		{"Customer Solution Architect, Team Lead", "Success", "Success", "customer", "lead_manager"},
		{"Implementation Consultant (AUNZ)", "", "1142 APAC Delivery", "customer", "unknown"},
		{"Freelance - Consultant.e en Formation - Bordeaux", "Training", "Partner Success - Cabinets comptable", "customer", "unknown"},
		// Marketing, operations, finance, people
		{"Money Management Demand Generation Manager, Americas", "", "2311 Marketing - AMER", "marketing", "unknown"},
		{"Head of Marketing, Japan", "", "2313 Marketing - APAC", "marketing", "lead_manager"},
		{"Social Media Strategy Director", "", "Communications", "marketing", "lead_manager"},
		{"Junior Campaign Manager", "", "Marketing", "marketing", "junior"},
		{"Stage - Analyste Stratégie Marketing Patient (x/f/m) - janvier 2027", "", "Marketing", "marketing", "intern"},
		{"Program Manager, Professional Services", "", "1140 Professional Services", "operations", "unknown"},
		{"Staff Program Manager, Event Design", "", "8113 Brand Studio", "operations", "staff_plus"},
		{"Strategy & Ops, Enterprise", "", "1180 GEO Sales HQ (EMEA)", "operations", "unknown"},
		{"FP&A  Strategy Intern", "", "Finance", "finance", "intern"},
		{"Prudential Reporting & Banking Accountant H/F", "", "", "finance", "unknown"},
		{"Intern Talent Acquisition Coordinator (x/f/m)", "", "People", "people", "intern"},
		{"Head of People H/F", "", "", "people", "lead_manager"},
		{"HR Business Partner (H/F) - CDI Casablanca", "", "", "people", "unknown"},
		{"Chef de projets RH - Stage Paris - Theodo", "", "", "people", "intern"},
		// Spontaneous applications: only the words before "candidature spontanée" count,
		// and the team/department (a company unit) is not used.
		{"Profil Comptable - Candidature Spontanée", "Candidature spontanée", "Candidature spontanée", "finance", "unknown"},
		{"Candidature spontanée - Theodo Data & AI", "Other", "Theodo Data & IA", "unknown", "unknown"},
		// Nothing to go on
		{"Manager, Payments Performance (SEA and Greater China)", "", "1342 Payment Performance", "unknown", "lead_manager"},
		{"Technical Curriculum Developer, Technical Enablement", "", "1120 Training & Certification", "unknown", "unknown"},
		{"Business Value Consultant", "", "1530 GTM Industries", "unknown", "unknown"},
		{"Senior Manager, Customer Engineering, Central Eastern Europe & Israel - Hebrew Speaking", "", "", "unknown", "lead_manager"},
		{"Manager, Data Center Rack Integration", "", "", "platform", "lead_manager"},
		{"Software Engineer, New Grad", "", "", "unknown", "junior"},
	} {
		got := job.Classify(job.Record{Title: c.title, Team: c.team, Department: c.dept}, "")
		if got.RoleFamily != c.family || got.Seniority != c.seniority {
			t.Errorf("%q (%q, %q) = %s/%s, want %s/%s", c.title, c.team, c.dept,
				got.RoleFamily, got.Seniority, c.family, c.seniority)
		}
	}
}

// AC2: an explicit minimum, the lower bound of a range, the first one stated; traps and
// absurd numbers are ignored. Phrasings are from the local descriptions.
func TestClassifyExperience(t *testing.T) {
	for _, c := range []struct {
		desc string
		want int
	}{
		{"Have 5+ years of backend engineering experience building APIs", 5},
		{"You have at least 6 years of experience with Go", 6},
		{"At least 5+ years of relevant working experience", 5},
		{"Minimum 8+ years of product management experience", 8},
		{"Minimum of 12 years of Recruitment experience in a fast-paced firm", 12},
		{"Profil : minimum de 5 ans d'expérience en développement", 5},
		{"Vous avez au moins 3 ans d’expérience", 3},
		{"Vous justifiez de 4 ans d'expérience sur un poste similaire", 4},
		{"Expérience : 2+ ans en support client", 2},
		{"5–8 years of proven ability to build trust", 5},
		{"4-6+ years of experience in a client-facing role", 4},
		{"Proven experience (3-4 years) working in accounts payables", 3},
		{"You have 3-5 years of experience in a client-facing role", 3},
		{"de 2 à 4 ans d'expérience", 2},
		{"10 years of experience in consulting, strategy and operations", 10},
		{"3 years experience in managing teams", 3},
		{"7+ years of experience&lt;/li&gt;", 7},
		{"8+ years in software engineering and 3+ years managing engineers", 8},
		// traps
		{"Kraken has spent the last 15 years building one of the most modern exchanges", 0},
		{"We saw 5 years of growth in a row", 0},
		{"The company was founded 10 years ago", 0},
		{"Time off through our Break4Me program after 3 years at SumUp", 0},
		{"Our 2024 report covers 50 years of history", 0},
		{"0+ years of experience", 0},
		{"Monzo has accounts for 16-17 year olds", 0},
		{"For 30+ years, we’ve delivered extraordinary impact. You have 4+ years of Go", 4},
		{"", 0},
		// the trap comes first, the requirement later
		{"Founded 25 years ago. Requirements: 4+ years of Python", 4},
	} {
		if got := job.Classify(job.Record{}, c.desc).ExperienceMinYears; got != c.want {
			t.Errorf("%q: experience = %d, want %d", c.desc, got, c.want)
		}
	}
}

// AC1: every remote scope the locations name (ADR-0011), and the countries they name.
func TestClassifyRemoteScopesAndCountries(t *testing.T) {
	for _, c := range []struct {
		name      string
		rec       job.Record
		scopes    []string
		countries []string
	}{
		{"home based EMEA", job.Record{Workplace: job.Unknown, Locations: []string{"Home Based - EMEA"}},
			[]string{"emea"}, nil},
		{"several segments, one office", job.Record{Workplace: job.Unknown,
			Locations: []string{"Home Based - Americas; Home based - EMEA; Office Based - London, UK"}},
			[]string{"americas", "emea"}, []string{"GB"}},
		{"US", job.Record{Workplace: job.Remote, Locations: []string{"United States (Remote)", "USA (Remote)"}},
			[]string{"us_only"}, []string{"US"}},
		{"global", job.Record{Workplace: job.Remote, Locations: []string{"Distributed, Global"}},
			[]string{"worldwide"}, nil},
		{"worldwide", job.Record{Workplace: job.Unknown, Locations: []string{"Home based - Worldwide"}},
			[]string{"worldwide"}, nil},
		{"Morocco", job.Record{Workplace: job.Remote, Locations: []string{"Remote – Morocco"}},
			[]string{"country:MA"}, []string{"MA"}},
		{"Lever country code", job.Record{Workplace: job.Remote, Locations: []string{"Remote"}, CountryCodes: []string{"MA"}},
			[]string{"country:MA"}, []string{"MA"}},
		{"Lever US code", job.Record{Workplace: job.Remote, Locations: []string{"Remote"}, CountryCodes: []string{"US"}},
			[]string{"us_only"}, []string{"US"}},
		{"bare remote", job.Record{Workplace: job.Remote, Locations: []string{"Remote"}},
			[]string{"unknown"}, nil},
		{"remote, no location", job.Record{Workplace: job.Remote},
			[]string{"unknown"}, nil},
		{"or remote", job.Record{Workplace: job.Unknown, Locations: []string{"Cardiff, London or Remote (UK)"}},
			[]string{"country:GB"}, []string{"GB"}},
		{"two countries in one string", job.Record{Workplace: job.Remote, Locations: []string{"Remote, Canada; Remote, United States"}},
			[]string{"country:CA", "us_only"}, []string{"CA", "US"}},
		{"APAC", job.Record{Workplace: job.Unknown, Locations: []string{"Home Based - Asia Pacific"}},
			[]string{"apac"}, nil},
		{"NA", job.Record{Workplace: job.Remote, Locations: []string{"Remote - NA"}},
			[]string{"americas"}, nil},
		{"US state", job.Record{Workplace: job.Remote, Locations: []string{"Remote - Texas", "Seattle, Washington"}},
			[]string{"us_only"}, []string{"US"}},
		// Cities from the local data (bare or "XX - City"): onsite gets a country, remote
		// gets that country's scope.
		{"bare city, onsite", job.Record{Workplace: job.Onsite, Locations: []string{"Amsterdam", "Milan", "Lisbon"}},
			nil, []string{"NL", "IT", "PT"}},
		{"code-prefixed cities", job.Record{Workplace: job.Hybrid, Locations: []string{"KR - Seoul", "CN - Shanghai", "AU - Melbourne", "UK - London", "BZ - Sao Paulo"}},
			nil, []string{"KR", "CN", "AU", "GB", "BR"}},
		{"metro and office names", job.Record{Workplace: job.Onsite, Locations: []string{"Seattle Metro", "DigitalOcean Seattle Office", "Bay Area Metro"}},
			nil, []string{"US"}},
		{"remote from a city", job.Record{Workplace: job.Remote, Locations: []string{"Remote - Berlin"}},
			[]string{"country:DE"}, []string{"DE"}},
		{"europe", job.Record{Workplace: job.Remote, Locations: []string{"Remote - Europe"}},
			[]string{"europe"}, nil},
		{"French", job.Record{Workplace: job.Unknown, Locations: []string{"All France (remote)"}},
			[]string{"country:FR"}, []string{"FR"}},
		{"hybrid is not remote", job.Record{Workplace: job.Hybrid, Locations: []string{"Paris, France"}},
			nil, []string{"FR"}},
		{"onsite city only", job.Record{Workplace: job.Onsite, Locations: []string{"Casablanca"}},
			nil, []string{"MA"}},
		{"city not in the table", job.Record{Workplace: job.Onsite, Locations: []string{"Béziers"}},
			nil, nil},
		{"remote and an office", job.Record{Workplace: job.Unknown, Locations: []string{"Berlin, Germany", "Remote, Spain"}},
			[]string{"country:ES"}, []string{"DE", "ES"}},
	} {
		got := job.Classify(c.rec, "")
		if !slices.Equal(got.RemoteScopes, c.scopes) || !slices.Equal(got.Countries, c.countries) {
			t.Errorf("%s: scopes %v countries %v, want %v %v", c.name, got.RemoteScopes, got.Countries, c.scopes, c.countries)
		}
	}
}

// ADR-0011 owner decision: a real multi-country Ashby posting (Kraken, 14 countries,
// workplaceType Remote) gets one scope per country.
func TestClassifyMultiCountryAshby(t *testing.T) {
	p := payloads(t, "ashby", "normalise_cases.json")["8768d39b-2600-432a-bf09-54b6ba73133f"]
	if p == nil {
		t.Fatal("fixture missing")
	}
	rec, err := job.Normalise("ashby", p)
	if err != nil {
		t.Fatalf("Normalise: %v", err)
	}
	got := job.Classify(rec, job.Description("ashby", p))
	want := []string{"RO", "BR", "PE", "SE", "CO", "AR", "CY", "ZA", "CZ", "HU", "MX", "CL", "PL", "BG"}
	var scopes []string
	for _, c := range want {
		scopes = append(scopes, "country:"+c)
	}
	if !slices.Equal(got.Countries, want) || !slices.Equal(got.RemoteScopes, scopes) {
		t.Errorf("countries %v scopes %v, want %v %v", got.Countries, got.RemoteScopes, want, scopes)
	}
	if got.RoleFamily != "fullstack" || got.ExperienceMinYears != 3 {
		t.Errorf("family %s, experience %d; want fullstack, 3", got.RoleFamily, got.ExperienceMinYears)
	}
}

// Description assembles each board's requirement text as plain text.
func TestDescription(t *testing.T) {
	for _, c := range []struct{ board, file, id, want string }{
		{"lever", "spotify_postings.json", "2a55d6c9-c564-443a-be3e-a3851db5a93b", "What You'll Do"},
		{"greenhouse", "normalise_cases.json", "7528749003", "5+ years of experience in mobile engineering"},
		{"ashby", "linear_jobs.json", "d3bc1ced-3ce4-4086-a050-555055dbb1ff", "Linear"},
	} {
		d := job.Description(c.board, payloads(t, c.board, c.file)[c.id])
		if !strings.Contains(d, c.want) || strings.ContainsAny(d, "<>") || strings.Contains(d, "&lt;") {
			t.Errorf("%s: description lacks %q or still has markup: %.200q", c.board, c.want, d)
		}
	}
	if job.Description("lever", []byte("not json")) != "" {
		t.Error("description of a broken payload is not empty")
	}
}

// Facet values are always ones the jobs table CHECKs allow.
func FuzzClassify(f *testing.F) {
	f.Add("Senior Backend Engineer (H/F)", "Engineering", "Remote – Morocco", "5+ years of experience")
	f.Add("Candidature spontanée", "", "Home Based - EMEA; Office Based - London, UK", "minimum de 5 ans")
	f.Fuzz(func(t *testing.T, title, dept, loc, desc string) {
		got := job.Classify(job.Record{Title: title, Department: dept, Locations: []string{loc}, Workplace: job.Remote}, desc)
		if !slices.Contains(job.RoleFamilies, got.RoleFamily) || !slices.Contains(job.Seniorities, got.Seniority) {
			t.Fatalf("invalid facet %q/%q", got.RoleFamily, got.Seniority)
		}
		if got.ExperienceMinYears < 0 || got.ExperienceMinYears > 20 || len(got.RemoteScopes) == 0 {
			t.Fatalf("experience %d, scopes %v", got.ExperienceMinYears, got.RemoteScopes)
		}
	})
}

// Presets match cities as whole words, accents and case folded.
func TestNamesPlace(t *testing.T) {
	for loc, want := range map[string]bool{"Casablanca, Morocco": true, "CASABLANCA": true, "Rabatville": false, "": false} {
		if got := job.NamesPlace(loc, "Casablanca") || job.NamesPlace(loc, "Rabat"); got != want {
			t.Errorf("%q: %v, want %v", loc, got, want)
		}
	}
	if !job.NamesPlace("São Paulo, Brazil", "Sao Paulo") || job.NamesPlace("Paris", "") {
		t.Error("accent folding or empty place")
	}
}
