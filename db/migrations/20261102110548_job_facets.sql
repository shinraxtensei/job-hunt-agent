-- +goose Up
-- ADR-0011: facets the presets filter on, written with the job record. Rows made before
-- this have classifier_version 0 until `collector classify` fills them.
ALTER TABLE jobs
	ADD COLUMN role_family text NOT NULL DEFAULT 'unknown' CHECK (role_family IN ('backend', 'frontend',
		'fullstack', 'mobile', 'platform', 'data', 'ai_ml', 'security', 'qa', 'eng_management', 'product',
		'design', 'sales', 'marketing', 'customer', 'operations', 'finance', 'people', 'legal', 'other', 'unknown')),
	ADD COLUMN seniority text NOT NULL DEFAULT 'unknown' CHECK (seniority IN ('intern', 'junior', 'mid',
		'senior', 'staff_plus', 'lead_manager', 'unknown')),
	ADD COLUMN experience_min_years integer CHECK (experience_min_years BETWEEN 1 AND 20), -- NULL: none stated
	ADD COLUMN remote_scopes text[] NOT NULL DEFAULT '{}',
	ADD COLUMN countries text[] NOT NULL DEFAULT '{}',
	ADD COLUMN classifier_version integer NOT NULL DEFAULT 0;
