// Package personio fetches open positions from a company's public Personio career feed,
// https://{company}.jobs.personio.de/xml (M7-02). Step 0 (docs/research/m7-sources.md):
// public, no login, robots.txt allows all; Personio's own help pages couldn't be read
// (OI-17: the adapter goes if they turn out to forbid it).
//
// The feed is XML; each <position> is stored as a JSON object of its fields, so the
// stored payload is JSON like every other source's (internal/job normalises it).
package personio

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/time/rate"

	"github.com/shinraxtensei/job-hunt-agent/internal/job"
	"github.com/shinraxtensei/job-hunt-agent/internal/source/fetch"
)

// URL is the production feed address; %s is the company's Personio slug.
const URL = "https://%s.jobs.personio.de/xml"

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// Client reads Personio feeds. HTTP and Backoff come from the embedded fetch.Client.
type Client struct {
	*fetch.Client
	url string // with one %s for the slug
}

// New returns a client for urlFormat (URL in production; tests pass their server's).
func New(urlFormat string) *Client {
	return &Client{Client: fetch.New(), url: urlFormat}
}

// Name is the board type this client serves.
func (c *Client) Name() string { return "personio" }

// Host is the host requests are limited by: every company's subdomain shares Personio's
// servers, so they share one limiter.
func (c *Client) Host() string {
	if u, err := url.Parse(fmt.Sprintf(c.url, "x")); err == nil && u.Host != "" {
		h := u.Hostname()
		if i := strings.Index(h, "."); i > 0 && strings.HasSuffix(h, "personio.de") {
			return h[i+1:]
		}
		return u.Host
	}
	return "jobs.personio.de"
}

// Fetch returns every position in the company's feed and how many were skipped as
// malformed. A bad position never fails the whole fetch.
func (c *Client) Fetch(ctx context.Context, slug string, limit *rate.Limiter) ([]job.Raw, job.Skipped, error) {
	if !slugRe.MatchString(slug) {
		return nil, job.Skipped{}, fmt.Errorf("personio: fetch %q: not a Personio company slug", slug)
	}
	body, err := c.Get(ctx, fmt.Sprintf(c.url, slug), limit)
	if err != nil {
		return nil, job.Skipped{}, fmt.Errorf("personio: fetch %s: %w", slug, err)
	}
	raws, skipped, err := parse(body, slug)
	if err != nil {
		return nil, job.Skipped{}, fmt.Errorf("personio: fetch %s: %w", slug, err)
	}
	return raws, skipped, nil
}

// position is one <position>; unknown elements are ignored.
type position struct {
	ID                 string   `xml:"id" json:"id"`
	Subcompany         string   `xml:"subcompany" json:"subcompany,omitempty"`
	Office             string   `xml:"office" json:"office,omitempty"`
	AdditionalOffices  []string `xml:"additionalOffices>office" json:"additionalOffices,omitempty"`
	Department         string   `xml:"department" json:"department,omitempty"`
	RecruitingCategory string   `xml:"recruitingCategory" json:"recruitingCategory,omitempty"`
	Name               string   `xml:"name" json:"name"`
	Descriptions       []struct {
		Name  string `xml:"name" json:"name"`
		Value string `xml:"value" json:"value"`
	} `xml:"jobDescriptions>jobDescription" json:"jobDescriptions,omitempty"`
	EmploymentType     string `xml:"employmentType" json:"employmentType,omitempty"`
	Seniority          string `xml:"seniority" json:"seniority,omitempty"`
	Schedule           string `xml:"schedule" json:"schedule,omitempty"`
	YearsOfExperience  string `xml:"yearsOfExperience" json:"yearsOfExperience,omitempty"`
	Keywords           string `xml:"keywords" json:"keywords,omitempty"`
	Occupation         string `xml:"occupation" json:"occupation,omitempty"`
	OccupationCategory string `xml:"occupationCategory" json:"occupationCategory,omitempty"`
	CreatedAt          string `xml:"createdAt" json:"createdAt,omitempty"`
	URL                string `xml:"-" json:"url"`
}

// parse reads a feed into raw records, one per position. Positions without an id or a
// name, or containing NUL (which Postgres refuses), are skipped and counted. A document
// that isn't a Personio feed is an error: the format changed, and an empty result would
// hide it. Go's XML decoder resolves no external entities.
func parse(body []byte, slug string) ([]job.Raw, job.Skipped, error) {
	dec := xml.NewDecoder(bytes.NewReader(body))
	raws := []job.Raw{}
	var skipped job.Skipped
	seenRoot := false
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, job.Skipped{}, fmt.Errorf("decoding feed: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "workzag-jobs":
			seenRoot = true
		case "position":
			if !seenRoot {
				return nil, job.Skipped{}, errors.New("decoding feed: a position outside <workzag-jobs>")
			}
			var p position
			if err := dec.DecodeElement(&p, &se); err != nil {
				return nil, job.Skipped{}, fmt.Errorf("decoding feed: %w", err)
			}
			p.ID, p.Name = strings.TrimSpace(p.ID), strings.TrimSpace(p.Name)
			if p.ID == "" || p.Name == "" || strings.ContainsRune(p.Name+p.Office, 0) {
				skipped.Add(p.ID)
				continue
			}
			for i := range p.Descriptions {
				p.Descriptions[i].Name = strings.TrimSpace(p.Descriptions[i].Name)
				p.Descriptions[i].Value = strings.TrimSpace(p.Descriptions[i].Value)
			}
			p.URL = fmt.Sprintf("https://%s.jobs.personio.de/job/%s", slug, url.PathEscape(p.ID))
			payload, err := json.Marshal(p)
			if err != nil || fetch.HasNUL(payload) {
				skipped.Add(p.ID)
				continue
			}
			raws = append(raws, job.Raw{Source: "personio", SourceID: p.ID, URL: p.URL, Title: p.Name, Payload: payload})
		}
	}
	if !seenRoot {
		return nil, job.Skipped{}, errors.New("decoding feed: no <workzag-jobs> root in the response")
	}
	return raws, skipped, nil
}
