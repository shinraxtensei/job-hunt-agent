-- +goose Up
-- ADR-0001 (M3-02): the work queue, and the Go <-> Python contract
-- (db/contract/work_items.columns). Go enqueues (the reconcile step); Python claims with
-- a lease and acknowledges with its claim token.
--
-- One row per posting and kind. A done row records which content_hash was processed,
-- so reconcile re-queues the posting only when its content changes; the table stays as
-- small as the set of shown postings.
CREATE TABLE work_items (
	id           bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	posting_id   bigint      NOT NULL REFERENCES postings (id) ON DELETE CASCADE,
	kind         text        NOT NULL CHECK (kind IN ('extract')),
	content_hash bytea       NOT NULL, -- the posting content this item is for
	state        text        NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'running', 'done', 'failed')),
	attempts     integer     NOT NULL DEFAULT 0 CHECK (attempts >= 0),
	max_attempts integer     NOT NULL DEFAULT 5 CHECK (max_attempts > 0),
	run_after    timestamptz NOT NULL DEFAULT now(), -- backoff: not claimable before
	locked_until timestamptz, -- the lease; running and past it means its worker is gone
	claim_token  uuid,        -- fencing: only the current claim may complete or fail it
	-- Error kind and a short safe message, never posting text or model output.
	last_error   text        CHECK (length(last_error) <= 500),
	created_at   timestamptz NOT NULL DEFAULT now(),
	updated_at   timestamptz NOT NULL DEFAULT now(),
	UNIQUE (posting_id, kind),
	CHECK ((state = 'running') = (claim_token IS NOT NULL AND locked_until IS NOT NULL))
);

-- What claim scans: live items only.
CREATE INDEX work_items_live ON work_items (run_after, id) WHERE state IN ('pending', 'running');
