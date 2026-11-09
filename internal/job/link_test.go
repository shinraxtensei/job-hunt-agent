package job_test

import (
	"testing"

	"github.com/shinraxtensei/job-hunt-agent/internal/job"
)

const (
	leverID = "2a55d6c9-c564-443a-be3e-a3851db5a93b"
	ashbyID = "8768d39b-2600-432a-bf09-54b6ba73133f"
)

// M2-07 AC3: only exact known board hosts, over https, with the board's posting path,
// are treated as boards (and so may be fetched, through the adapter's own API). Every
// other link is manual: stored, never fetched.
func TestParseBoardLink(t *testing.T) {
	for _, c := range []struct {
		raw  string
		want job.BoardLink // zero: not a board link
	}{
		// valid board links
		{"https://jobs.lever.co/spotify/" + leverID, job.BoardLink{Source: "lever", Board: "spotify", ID: leverID}},
		{"https://jobs.lever.co/spotify/" + leverID + "/apply", job.BoardLink{Source: "lever", Board: "spotify", ID: leverID}},
		{"https://jobs.lever.co/spotify/" + leverID + "?lever-origin=applied&ref=x#top", job.BoardLink{Source: "lever", Board: "spotify", ID: leverID}},
		{"https://JOBS.LEVER.CO/spotify/" + leverID, job.BoardLink{Source: "lever", Board: "spotify", ID: leverID}},
		{"https://boards.greenhouse.io/cloudflare/jobs/8336674", job.BoardLink{Source: "greenhouse", Board: "cloudflare", ID: "8336674"}},
		{"https://job-boards.greenhouse.io/stripe/jobs/7123456?gh_jid=7123456", job.BoardLink{Source: "greenhouse", Board: "stripe", ID: "7123456"}},
		{"https://jobs.ashbyhq.com/kraken.com/" + ashbyID, job.BoardLink{Source: "ashby", Board: "kraken.com", ID: ashbyID}},
		{"https://jobs.ashbyhq.com/linear/" + ashbyID + "/application", job.BoardLink{Source: "ashby", Board: "linear", ID: ashbyID}},

		// look-alike hosts
		{"https://jobs.lever.co.evil.com/spotify/" + leverID, job.BoardLink{}},
		{"https://evil.com/jobs.lever.co/spotify/" + leverID, job.BoardLink{}},
		{"https://evil-jobs.lever.co/spotify/" + leverID, job.BoardLink{}},
		{"https://xjobs.lever.co/spotify/" + leverID, job.BoardLink{}},
		{"https://lever.co/spotify/" + leverID, job.BoardLink{}},
		{"https://jobs.lever.co./spotify/" + leverID, job.BoardLink{}}, // trailing dot
		{"https://api.lever.co/v0/postings/spotify/" + leverID, job.BoardLink{}},
		{"https://boards.greenhouse.io.evil.com/x/jobs/1", job.BoardLink{}},
		{"https://jobs.ashbyhq.com.evil.com/x/" + ashbyID, job.BoardLink{}},
		{"https://jobs.lever.co%2eevil.com/spotify/" + leverID, job.BoardLink{}},

		// userinfo tricks, ports
		{"https://jobs.lever.co@evil.com/spotify/" + leverID, job.BoardLink{}},
		{"https://user:pass@jobs.lever.co/spotify/" + leverID, job.BoardLink{}},
		{"https://evil.com\\@jobs.lever.co/spotify/" + leverID, job.BoardLink{}},
		{"https://jobs.lever.co:443/spotify/" + leverID, job.BoardLink{}},
		{"https://jobs.lever.co:8443/spotify/" + leverID, job.BoardLink{}},

		// schemes
		{"http://jobs.lever.co/spotify/" + leverID, job.BoardLink{}},
		{"javascript:alert(1)//jobs.lever.co/spotify/" + leverID, job.BoardLink{}},
		{"file:///jobs.lever.co/spotify/" + leverID, job.BoardLink{}},
		{"ftp://jobs.lever.co/spotify/" + leverID, job.BoardLink{}},
		{"//jobs.lever.co/spotify/" + leverID, job.BoardLink{}},
		{"jobs.lever.co/spotify/" + leverID, job.BoardLink{}},
		{"data:text/html,jobs.lever.co", job.BoardLink{}},

		// IP addresses and internal names
		{"https://127.0.0.1/spotify/" + leverID, job.BoardLink{}},
		{"https://[::1]/spotify/" + leverID, job.BoardLink{}},
		{"https://169.254.169.254/latest/meta-data/", job.BoardLink{}},
		{"https://0x7f000001/spotify/" + leverID, job.BoardLink{}},
		{"https://localhost/spotify/" + leverID, job.BoardLink{}},

		// right host, wrong path: never "fix" a path into a board posting
		{"https://jobs.lever.co/spotify", job.BoardLink{}},
		{"https://jobs.lever.co/spotify/not-a-uuid", job.BoardLink{}},
		{"https://jobs.lever.co/spotify/" + leverID + "/apply/extra", job.BoardLink{}},
		{"https://jobs.lever.co/../spotify/" + leverID, job.BoardLink{}},
		{"https://jobs.lever.co/spot%2Fify/" + leverID, job.BoardLink{}},
		{"https://jobs.lever.co/spot ify/" + leverID, job.BoardLink{}},
		{"https://boards.greenhouse.io/cloudflare/jobs/abc", job.BoardLink{}},
		{"https://boards.greenhouse.io/embed/job_app?for=cloudflare&token=8336674", job.BoardLink{}},
		{"https://jobs.ashbyhq.com/linear", job.BoardLink{}},
		{"", job.BoardLink{}},
		{"not a url at all", job.BoardLink{}},
	} {
		got, ok := job.ParseBoardLink(c.raw)
		if got != c.want || ok != (c.want != job.BoardLink{}) {
			t.Errorf("ParseBoardLink(%q) = %+v, %v; want %+v", c.raw, got, ok, c.want)
		}
	}
}

// M2-07 AC2/AC5: manual links are stored by a normal form, so the same job pasted twice
// is one posting; only absolute http(s) links without user info are accepted.
func TestNormaliseLink(t *testing.T) {
	for raw, want := range map[string]string{
		"https://www.linkedin.com/jobs/view/4012345678/":                               "https://www.linkedin.com/jobs/view/4012345678",
		"HTTPS://WWW.LinkedIn.com/jobs/view/4012345678?trk=public_jobs&refId=abc#frag": "https://www.linkedin.com/jobs/view/4012345678",
		"https://careers.example.ma/offre?id=42&utm_source=x&utm_medium=y":             "https://careers.example.ma/offre?id=42",
		"https://careers.example.ma/offre?b=2&a=1":                                     "https://careers.example.ma/offre?a=1&b=2",
		"http://rekrute.com/offre-emploi-123.html":                                     "http://rekrute.com/offre-emploi-123.html",
		"https://jobs.lever.co/spotify/" + leverID + "/":                               "https://jobs.lever.co/spotify/" + leverID,
	} {
		got, err := job.NormaliseLink(raw)
		if err != nil || got != want {
			t.Errorf("NormaliseLink(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
	for _, raw := range []string{"", "javascript:alert(1)", "file:///etc/passwd", "ftp://x.com/a",
		"https://user:pass@example.com/job", "//example.com/job", "example.com/job", "https://", "data:text/html,x"} {
		if got, err := job.NormaliseLink(raw); err == nil {
			t.Errorf("NormaliseLink(%q) = %q, want an error", raw, got)
		}
	}
}

// The parser never panics, and anything it accepts is a well-formed board link whose
// parts rebuild a link it accepts again.
func FuzzParseBoardLink(f *testing.F) {
	for _, s := range []string{"https://jobs.lever.co/spotify/" + leverID, "https://boards.greenhouse.io/x/jobs/1",
		"https://jobs.ashbyhq.com/kraken.com/" + ashbyID, "https://jobs.lever.co@evil.com/x/" + leverID, "https://[::1]:80/a"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		b, ok := job.ParseBoardLink(raw)
		if !ok {
			if b != (job.BoardLink{}) {
				t.Fatalf("rejected link returned %+v", b)
			}
			return
		}
		if b.Source == "" || b.Board == "" || b.ID == "" {
			t.Fatalf("accepted %q with empty parts %+v", raw, b)
		}
		if again, ok := job.ParseBoardLink(b.CanonicalURL()); !ok || again != b {
			t.Fatalf("canonical %q of %q does not round-trip: %+v %v", b.CanonicalURL(), raw, again, ok)
		}
	})
}

// An http:// link on a board host is refused (not stored as a manual duplicate of the
// board posting): the owner is told to use the https:// link.
func TestInsecureBoardLink(t *testing.T) {
	for raw, want := range map[string]bool{
		"http://jobs.lever.co/spotify/" + leverID:          true,
		"HTTP://JOBS.LEVER.CO/spotify/" + leverID:          true,
		"http://boards.greenhouse.io/x/jobs/1":             true,
		"http://job-boards.greenhouse.io/x/jobs/1":         true,
		"http://jobs.ashbyhq.com/linear/" + ashbyID:        true,
		"http://jobs.lever.co/careers":                     true, // any path on a board host
		"https://jobs.lever.co/spotify/" + leverID:         false,
		"http://jobs.lever.co.evil.com/spotify/" + leverID: false,
		"http://rekrute.com/offre-1.html":                  false,
		"http://jobs.lever.co:8080/x/" + leverID:           false, // a port: not the board host
		"not a url":                                        false,
	} {
		if got := job.InsecureBoardLink(raw); got != want {
			t.Errorf("InsecureBoardLink(%q) = %v, want %v", raw, got, want)
		}
	}
}
