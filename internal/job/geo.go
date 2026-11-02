package job

import (
	"bytes"
	_ "embed" // the country tables are checked-in CSVs (M2-09)
	"encoding/csv"
	"slices"
	"strings"
)

// Every ISO 3166-1 alpha-2 code with its English name (generated once by
// geo/generate.mjs from CLDR and reviewed), and reviewed aliases. Read at start-up; nothing
// is downloaded.
var (
	//go:embed geo/iso3166-1.csv
	isoCSV []byte
	//go:embed geo/aliases.csv
	aliasCSV []byte
)

// ambiguous names are never matched: "Georgia" is a country (GE) and a US state.
var ambiguous = map[string]bool{"georgia": true}

var countryNames = func() map[string]string {
	m := map[string]string{}
	for _, r := range readCSV(isoCSV) {
		m[r[0]] = r[1]
	}
	return m
}()

// countryPhrases are geo entries ("c:<code>") for every country name and alias, plus "and"
// and "Saint" variants of names written with "&" and "St.".
var countryPhrases = func() []phrase {
	byCode := map[string][]string{}
	add := func(code, name string) {
		and := strings.ReplaceAll(name, "&", " and ")
		for _, v := range []string{name, and, strings.Replace(name, "St. ", "Saint ", 1), strings.Replace(and, "St. ", "Saint ", 1)} {
			if !ambiguous[strings.Join(tokens(v), " ")] {
				byCode["c:"+code] = append(byCode["c:"+code], v)
			}
		}
	}
	for code, name := range countryNames {
		add(code, name)
	}
	for _, r := range readCSV(aliasCSV) {
		add(r[0], r[1])
	}
	return table(byCode)
}()

// readCSV returns the code,value rows of a checked-in table, skipping comments, the header
// and rows whose code or value is invalid. It stops at the first row the CSV reader
// rejects (wrong field count, broken quoting), so everything after a malformed row would
// be lost: TestCountryTablesAreStrict fails on any malformed row, unknown code or empty
// alias, so that can't ship.
func readCSV(b []byte) [][2]string {
	r := csv.NewReader(bytes.NewReader(b))
	r.Comment = '#'
	var out [][2]string
	for {
		rec, err := r.Read()
		if err != nil {
			break // io.EOF, or a malformed row: ends the table (see above)
		}
		if len(rec) == 2 && isCountryCode(rec[0]) && rec[1] != "" {
			out = append(out, [2]string{rec[0], rec[1]})
		}
	}
	return out
}

// CountryCodes returns every ISO 3166-1 alpha-2 code, sorted.
func CountryCodes() []string {
	codes := make([]string, 0, len(countryNames))
	for c := range countryNames {
		codes = append(codes, c)
	}
	slices.Sort(codes)
	return codes
}

// CountryName is code's English name, or "".
func CountryName(code string) string { return countryNames[code] }

// regions lists the member countries of the remote regions presets can name as country
// aliases (LATAM, AMERICAS), so a "countries" rule can accept a whole region.
var regions = map[string][]string{
	"latam": strings.Fields("AR BO BR CL CO CR CU DO EC GT HN HT MX NI PA PE PR PY SV UY VE"),
	"americas": strings.Fields(`AR BO BR CL CO CR CU DO EC GT HN HT MX NI PA PE PR PY SV UY VE
		US CA AG AI AW BB BL BM BQ BS BZ CW DM FK GD GF GL GP GY JM KN KY LC MF MQ MS PM SR SX TC TT UM VC VG VI`),
}

// RegionCountries returns the member country codes of a region ("latam", "americas"), or
// nil for an unknown one.
func RegionCountries(region string) []string {
	return slices.Clone(regions[region])
}

// Unresolved reports whether a location names no country, city or region the classifier
// knows (`collector classify --report-unknown` lists them, so gaps are visible).
func Unresolved(location string) bool {
	f := Classify(Record{Workplace: Remote, Locations: []string{location}}, "")
	return len(f.Countries) == 0 && slices.Equal(f.RemoteScopes, []string{"unknown"})
}
