package httpapi

import (
	"net/http"
	"net/url"
	"time"

	"github.com/shinraxtensei/job-hunt-agent/internal/store"
)

// VisitGap is how long a visit lasts after the page was last opened (OI-20): opening Today
// again within 30 minutes is the same visit, so a reload doesn't empty its lists.
const VisitGap = 30 * time.Minute

// TodayNew caps the new leads listed (the count is complete).
const TodayNew = 50

// since is when "new" starts for Today (M7-01, OI-20): the start of the visit before the
// current one, or the time "Mark all seen" was pressed if later; with no earlier visit,
// the last 24 hours.
func since(visits []store.Visit, now time.Time) time.Time {
	s := now.Add(-24 * time.Hour)
	if len(visits) > 1 {
		s = visits[1].StartedAt.Time
	}
	if len(visits) > 0 && visits[0].MarkedAt.Valid && visits[0].MarkedAt.Time.After(s) {
		s = visits[0].MarkedAt.Time
	}
	return s.UTC()
}

// visit records that the owner opened Today: a new visit after 30 minutes away, else the
// current one goes on. An ADR-0022 write.
func (a *api) visit(w http.ResponseWriter, r *http.Request) {
	if !guardWrite(w, r) {
		return
	}
	now := a.now()
	vs, err := a.q.LatestVisits(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if len(vs) == 0 || now.Sub(vs[0].LastSeenAt.Time) > VisitGap {
		_, err = a.q.StartVisit(r.Context(), ts(now))
	} else {
		err = a.q.ExtendVisit(r.Context(), store.ExtendVisitParams{ID: vs[0].ID, AtTime: ts(now)})
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.writeSince(w, r, now)
}

// markSeen is "Mark all seen": nothing before now counts as new any more.
func (a *api) markSeen(w http.ResponseWriter, r *http.Request) {
	if !guardWrite(w, r) {
		return
	}
	now := a.now()
	n, err := a.q.MarkVisitSeen(r.Context(), ts(now))
	if err == nil && n == 0 { // no visit yet: start one, then mark it
		if _, err = a.q.StartVisit(r.Context(), ts(now)); err == nil {
			_, err = a.q.MarkVisitSeen(r.Context(), ts(now))
		}
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.writeSince(w, r, now)
}

func (a *api) writeSince(w http.ResponseWriter, r *http.Request, now time.Time) {
	vs, err := a.q.LatestVisits(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]time.Time{"since": since(vs, now)})
}

// ClosedLead is a lead you acted on whose posting closed since the last visit.
type ClosedLead struct {
	Lead
	ClosedAt time.Time `json:"closed_at"`
}

// today answers the Today view (M7-01): since when, the new leads the default preset shows
// (best fit first), and the closed postings of leads you acted on. Read-only: opening the
// page records the visit with POST /api/visits first.
func (a *api) today(w http.ResponseWriter, r *http.Request) {
	now := a.now()
	vs, err := a.q.LatestVisits(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	from := since(vs, now)
	f, err := a.parse(url.Values{})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	all, err := a.load(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	shown, _ := a.split(all, f)
	fresh := []Lead{}
	for _, l := range shown {
		if l.FirstSeenAt.After(from) {
			fresh = append(fresh, l)
		}
	}
	total := len(fresh)
	if len(fresh) > TodayNew {
		fresh = fresh[:TodayNew]
	}
	rows, err := a.q.ClosedSince(r.Context(), ts(from))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	type group struct {
		company int64
		title   string
	}
	byGroup := make(map[group]Lead, len(all))
	for _, l := range all {
		byGroup[group{l.companyID, l.titleKey}] = l
	}
	closed := []ClosedLead{}
	for _, c := range rows {
		if l, ok := byGroup[group{c.CompanyID, c.TitleKey}]; ok {
			closed = append(closed, ClosedLead{l, c.ClosedAt.Time.UTC()})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"since": from, "new_total": total, "new": fresh, "closed": closed})
}
