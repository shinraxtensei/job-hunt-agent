-- name: EnsureApplication :one
-- The lead's application, created at `new` if it has none (a note or contact on a new
-- lead, M6-03). No row: no such lead.
WITH g AS (SELECT j.company_id, j.title_key FROM jobs j WHERE j.posting_id = @posting_id::bigint),
ins AS (
	INSERT INTO applications (company_id, title_key, posting_id, status)
	SELECT g.company_id, g.title_key, @posting_id::bigint, 'new' FROM g
	ON CONFLICT (company_id, title_key) DO NOTHING
	RETURNING id
)
SELECT id FROM ins
UNION ALL
SELECT a.id FROM applications a JOIN g ON a.company_id = g.company_id AND a.title_key = g.title_key;

-- name: AddNote :one
INSERT INTO notes (application_id, body) VALUES (@application_id::bigint, @body::text) RETURNING id;

-- name: EditNote :execrows
UPDATE notes SET body = @body::text, edited_at = now() WHERE id = @id::bigint;

-- name: DeleteNote :execrows
DELETE FROM notes WHERE id = @id::bigint;

-- name: ListNotes :many
SELECT id, body, created_at, edited_at FROM notes
WHERE application_id = @application_id::bigint ORDER BY created_at DESC, id DESC;

-- name: AddContact :one
INSERT INTO contacts (application_id, name, role, email, linkedin, phone)
VALUES (@application_id::bigint, @name::text, @role::text, @email::text, @linkedin::text, @phone::text)
RETURNING id;

-- name: EditContact :execrows
UPDATE contacts SET name = @name::text, role = @role::text, email = @email::text,
	linkedin = @linkedin::text, phone = @phone::text
WHERE id = @id::bigint;

-- name: DeleteContact :execrows
DELETE FROM contacts WHERE id = @id::bigint;

-- name: ListContacts :many
SELECT id, name, role, email, linkedin, phone, created_at FROM contacts
WHERE application_id = @application_id::bigint ORDER BY created_at, id;
