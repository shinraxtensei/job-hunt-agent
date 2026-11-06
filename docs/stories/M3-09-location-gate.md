# M3-09: Location gate

- **Status:** Done
- **Milestone:** M3 — Extraction and scoring
- **Linear:** [HAM-48](https://linear.app/hamid-corp/issue/HAM-48/m3-09-location-gate)
- **Estimate:** ½ day

## Story

As the job seeker, I want a lead I can't work at (wrong country, no remote, no visa) never
to rank as a strong fit, however well my skills match, so that the top of the list is
jobs I can actually take.

## Context

After M3-08 the top ten held three Stripe roles in São Paulo, the US and Canada. Each scored
3 of 15 on location, remote and visa, but the other factors carried them to "very
strong". The weights stay as the PRD says; this adds a gate on top of them. Owner's
direction on #43's review.

## Acceptance criteria

1. **Given** a lead whose location/remote/visa points are below `bands.location_gate`
   in `scoring.yaml` (start: 5 of 15)
   **When** its fit level is computed (worker, `lead_scores` view, API)
   **Then** the level is at most "stretch"; the total is unchanged (table tests in
   Python and Go, same cases).
2. **Given** score order (`sort=score`)
   **When** I page
   **Then** leads are ordered by fit level first (capped included), then by total, with
   the exactly-once cursor (integration test).
3. **Given** a capped lead
   **When** its reason is written
   **Then** the model sees that the level is capped and why, from the breakdown: the
   location detail names the place and workplace ("on-site in São Paulo, visa
   unknown"). The reason's worst sentence says you can't work there and why (prompt
   v3; fake-provider test that the breakdown carries the cap).
4. **Given** the gate in configuration
   **When** it changes
   **Then** every shown lead is re-scored with no model call (bands version).

## Out of scope

- Changing the weights or band cut-offs. A UI badge for capped leads: the level and the
  reason say it.

## Security notes

Location names come from postings (untrusted). They reach the reason model only inside
the escaped data block, as skill names already do. No new input, endpoint or
dependency.
