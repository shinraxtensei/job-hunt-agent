-- name: ListLeadScores :many
-- Every shown posting's score state for the leads API (M3-08, ADR-0021): the extract
-- item (Go queued it because the default preset showed the lead), the coded score with
-- the freshness and bands it was scored with, and the reason with whether it is current.
SELECT w.posting_id, w.state AS extract_state,
	s.points, s.factors, s.not_enough_info, s.freshness, s.bands, s.stretch, -- M9-03: a flag
	r.reason, r.gaps, r.cv_variant, s.cv_variant AS score_variant, -- M4-06: the score's wins
	(r.posting_id IS NOT NULL AND s.posting_id IS NOT NULL AND r.content_hash = s.content_hash
		AND r.inputs = s.reason_inputs)::boolean AS reason_current,
	rw.state AS reason_state
FROM work_items w
LEFT JOIN scores s ON s.posting_id = w.posting_id
LEFT JOIN score_reasons r ON r.posting_id = w.posting_id
LEFT JOIN work_items rw ON rw.posting_id = w.posting_id AND rw.kind = 'reason'
WHERE w.kind = 'extract';

-- name: RequestReason :execrows
-- Marks a lead opened, so the worker regenerates its reason (ADR-0021, ADR-0022); only
-- when the reason is missing or not current.
UPDATE scores s SET reason_requested_at = now()
WHERE s.posting_id = @posting_id AND NOT EXISTS (
	SELECT 1 FROM score_reasons r
	WHERE r.posting_id = s.posting_id AND r.content_hash = s.content_hash AND r.inputs = s.reason_inputs
);
