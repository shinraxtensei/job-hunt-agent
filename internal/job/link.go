package job

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

// BoardLink is a posting on a supported board, read from a pasted link (M2-07, ADR-0015).
type BoardLink struct {
	Source string // "lever", "greenhouse" or "ashby"
	Board  string // the board's slug
	ID     string // the posting's id on that board
}

// boardHosts are the only hosts whose links are boards, compared exactly after
// lower-casing. A pasted link is never fetched: the adapter calls its own API endpoint
// built from Board and ID.
var boardHosts = map[string]string{
	"jobs.lever.co":            "lever",
	"boards.greenhouse.io":     "greenhouse",
	"job-boards.greenhouse.io": "greenhouse",
	"jobs.ashbyhq.com":         "ashby",
}

var (
	slugRe   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)
	uuidRe   = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	digitsRe = regexp.MustCompile(`^[0-9]{1,20}$`)
)

// ParseBoardLink reports whether raw is a posting on a supported board, and which. It
// accepts only https links with no user info, no port and no percent-encoded path, whose
// lower-cased host is exactly a board host, and whose path is that board's posting shape:
//
//	jobs.lever.co/<site>/<uuid>[/apply]
//	boards.greenhouse.io/<board>/jobs/<digits>  (and job-boards.greenhouse.io)
//	jobs.ashbyhq.com/<board>/<uuid>[/application]
//
// Anything else, look-alike hosts, IP addresses and other schemes included, is not a board.
func ParseBoardLink(raw string) (BoardLink, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Opaque != "" || u.User != nil || u.RawPath != "" || u.Port() != "" {
		return BoardLink{}, false
	}
	source, ok := boardHosts[strings.ToLower(u.Host)]
	if !ok {
		return BoardLink{}, false
	}
	seg := strings.Split(strings.TrimPrefix(strings.TrimSuffix(u.Path, "/"), "/"), "/")
	var b BoardLink
	switch source {
	case "lever", "ashby":
		suffix := map[string]string{"lever": "apply", "ashby": "application"}[source]
		if len(seg) == 3 && seg[2] == suffix {
			seg = seg[:2]
		}
		if len(seg) != 2 || !uuidRe.MatchString(seg[1]) {
			return BoardLink{}, false
		}
		b = BoardLink{Source: source, Board: seg[0], ID: seg[1]}
	case "greenhouse":
		if len(seg) != 3 || seg[1] != "jobs" || !digitsRe.MatchString(seg[2]) {
			return BoardLink{}, false
		}
		b = BoardLink{Source: source, Board: seg[0], ID: seg[2]}
	}
	if !slugRe.MatchString(b.Board) {
		return BoardLink{}, false
	}
	return b, true
}

// CanonicalURL is the board's public link to the posting.
func (b BoardLink) CanonicalURL() string {
	switch b.Source {
	case "lever":
		return "https://jobs.lever.co/" + b.Board + "/" + b.ID
	case "greenhouse":
		return "https://boards.greenhouse.io/" + b.Board + "/jobs/" + b.ID
	case "ashby":
		return "https://jobs.ashbyhq.com/" + b.Board + "/" + b.ID
	}
	return ""
}

// tracking are query parameters that identify the click, not the job: dropped when links
// are compared.
var tracking = map[string]bool{"trk": true, "ref": true, "refid": true, "trackingid": true}

// NormaliseLink returns the form a manual link is stored and compared by (so the same job
// pasted twice is one posting): lower-case scheme and host, no fragment, no trailing
// slash, tracking parameters removed, the rest sorted. Only absolute http(s) links
// without user info are accepted. The link is never fetched.
func NormaliseLink(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", errors.New("not a link")
	}
	scheme := strings.ToLower(u.Scheme)
	switch {
	case scheme != "http" && scheme != "https":
		return "", errors.New("only http and https links can be added")
	case u.Opaque != "" || u.Host == "":
		return "", errors.New("the link has no host")
	case u.User != nil:
		return "", errors.New("links with a user name or password are not accepted")
	}
	q := u.Query()
	for k := range q {
		if lk := strings.ToLower(k); tracking[lk] || strings.HasPrefix(lk, "utm_") {
			q.Del(k)
		}
	}
	s := scheme + "://" + strings.ToLower(u.Host) + strings.TrimRight(u.EscapedPath(), "/")
	if enc := q.Encode(); enc != "" { // Encode sorts by key
		s += "?" + enc
	}
	return s, nil
}

// InsecureBoardLink reports whether raw is an http:// link on a board host. `collector add`
// refuses it (asking for the https:// link) rather than storing a manual duplicate of a
// board posting. Nothing is fetched either way.
func InsecureBoardLink(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "http") || u.Port() != "" {
		return false
	}
	_, ok := boardHosts[strings.ToLower(u.Host)]
	return ok
}
