---
name: source-adapter
description: Use when adding, changing, or debugging a job-board source adapter in the job-hunt-agent repo (Lever, Greenhouse, Ashby, Workable, remote boards, Moroccan boards, career pages), or when a collector run fails for one source. Covers terms checks, the adapter contract, rate limits, golden-file tests, and fuzzing.
---

# Adding a job source adapter

Adapters are the unit of growth: adding a source means writing one adapter package in
`internal/source/<name>/` and nothing else changes. Keep each adapter small, isolated,
and boring.

## 0. Check you are allowed to collect from it

Before any code, record in the story's **Security notes**:
- Does the source offer an official API or public feed? Prefer that.
- What do its terms of service and `robots.txt` say about automated access?
- Does it require logging in? **If yes, stop.** No logged-in collection, ever.
  Those postings enter only through manual add (paste a link).
- Is there anti-bot protection (CAPTCHA, challenge pages)? **If yes, stop.** We never
  bypass blocks.

If any answer is unclear, ask the user before continuing.

## 1. The contract

Every adapter implements the interface defined in the collector (the consumer side):

```go
// in internal/collector
type Source interface {
    Name() string
    Fetch(ctx context.Context, company Company) ([]job.Raw, error)
}
```

- `Fetch` returns `job.Raw` records: the original payload bytes plus source metadata.
  Normalisation into `job.Job` happens in `internal/job`, not in the adapter.
- Always store the raw text. If extraction improves, old postings are reprocessed
  without fetching again.
- Respect `ctx`: every HTTP request uses `http.NewRequestWithContext`.
- Return wrapped errors: `fmt.Errorf("lever: fetch %s: %w", company.Slug, err)`.
- A malformed single posting is logged and skipped. It never fails the whole fetch.

## 2. Being a polite client

- One `*rate.Limiter` per host (`golang.org/x/time/rate`); default 1 request per
  second unless the source documents a higher limit.
- A descriptive `User-Agent` with a contact URL.
- Timeouts on every client (`http.Client{Timeout: ...}`); no default client.
- Retry only idempotent GETs, only on 429 / 5xx / timeouts, with exponential backoff and
  jitter, at most 3 attempts. Honour `Retry-After`.
- Never run more than the configured concurrency per host.

## 3. Tests (write these first)

Directory layout:
```
internal/source/lever/
  lever.go
  lever_test.go
  testdata/
    acme_postings.json        # recorded real response, trimmed
    empty.json
    malformed_posting.json
```

Required tests:
1. **Golden-file parse test**: serve `testdata/*.json` from `httptest.NewServer`,
   assert the exact `job.Raw` records produced.
2. **Empty board**: zero postings returns an empty slice, not an error.
3. **Malformed posting**: one bad entry is skipped, the rest are returned.
4. **HTTP errors**: 404 → error naming the company; 429 → retried and `Retry-After`
   respected (use a fake clock or a short limiter in tests).
5. **Context cancellation**: a cancelled context stops the fetch promptly.
6. **Fuzz test** for the parsing function:
   `func FuzzParse(f *testing.F)` seeded with the golden files; it must never panic.

CI never touches the live site. To refresh a golden file, record it manually once,
strip anything personal, and commit it with the date in the commit message.

## 4. Wire it in

- Register the adapter in the collector's source registry.
- Add the board type to the `companies.board_type` enum with a migration.
- Add the source to the README's list of supported sources.

## 5. Done checklist

- [ ] Terms and robots checked and noted in the story
- [ ] All six test types present and passing with `-race`
- [ ] Rate limit and timeout configured
- [ ] Ran once against the real source locally and checked the output by eye
- [ ] No credentials, cookies, or session tokens anywhere in the adapter
