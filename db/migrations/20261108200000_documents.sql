-- +goose Up
-- M5-04: every exported PDF (M5-03), kept as it was. A row is never changed or deleted,
-- except `sent_at`, which the owner sets once ("I sent this"); the system sends nothing
-- (guardrail rule 6). PDF bytes: personal, never logged.
CREATE TABLE documents (
	id               bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	draft_id         bigint      NOT NULL REFERENCES drafts (id),
	posting_id       bigint      NOT NULL REFERENCES postings (id),
	kind             text        NOT NULL CHECK (kind IN ('cv', 'letter')),
	pdf              bytea       NOT NULL,
	sha256           bytea       NOT NULL CHECK (length(sha256) = 32),
	evidence_version text        NOT NULL, -- copied from the draft, so a document stands alone
	renderer_version integer     NOT NULL, -- the PDF layout code (M5-03)
	made_at          timestamptz NOT NULL DEFAULT now(),
	sent_at          timestamptz
);
CREATE INDEX documents_draft_idx ON documents (draft_id);

-- +goose StatementBegin
CREATE FUNCTION documents_keep() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
	IF TG_OP = 'DELETE' THEN
		RAISE EXCEPTION 'documents are kept: never deleted';
	END IF;
	IF OLD.sent_at IS NOT NULL OR NEW.sent_at IS NULL
		OR (to_jsonb(NEW) - 'sent_at') <> (to_jsonb(OLD) - 'sent_at') THEN
		RAISE EXCEPTION 'documents are kept: only sent_at may be set, once';
	END IF;
	RETURN NEW;
END $$;
-- +goose StatementEnd

CREATE TRIGGER documents_keep BEFORE UPDATE OR DELETE ON documents
	FOR EACH ROW EXECUTE FUNCTION documents_keep();
