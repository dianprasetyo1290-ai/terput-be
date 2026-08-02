-- Data Jabatan's "Prioritas" field (org-chart hierarchy ordering) was collected in the UI but
-- had nowhere to persist to — the input value was silently dropped. Default 0 keeps existing
-- positions sorting the same as before (by name) until an admin sets a real priority.
ALTER TABLE positions ADD COLUMN priority INTEGER NOT NULL DEFAULT 0;
