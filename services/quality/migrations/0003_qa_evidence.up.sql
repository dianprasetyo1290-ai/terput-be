CREATE TABLE qa_evidence (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('file', 'sekoci_report', 'link')),
    title TEXT NOT NULL,
    object_key TEXT NULL,
    mime TEXT NULL,
    size_bytes BIGINT NULL,
    checksum TEXT NULL,
    report_ref JSONB NULL,
    url TEXT NULL,
    version INT NOT NULL DEFAULT 1,
    supersedes_id UUID NULL REFERENCES qa_evidence(id) ON DELETE SET NULL,
    uploaded_by UUID NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'verified', 'rejected')),
    verified_by UUID NULL,
    verified_at TIMESTAMPTZ NULL,
    reject_reason TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT chk_file_kind CHECK (
        (kind = 'file' AND object_key IS NOT NULL) OR
        (kind <> 'file' AND object_key IS NULL)
    ),
    CONSTRAINT chk_report_kind CHECK (
        (kind = 'sekoci_report' AND report_ref IS NOT NULL) OR
        (kind <> 'sekoci_report' AND report_ref IS NULL)
    )
);
CREATE INDEX idx_qa_evidence_school ON qa_evidence(school_id, status) WHERE deleted_at IS NULL;

CREATE TABLE qa_evidence_links (
    evidence_id UUID NOT NULL REFERENCES qa_evidence(id) ON DELETE CASCADE,
    indicator_id UUID NOT NULL REFERENCES qa_indicators(id) ON DELETE CASCADE,
    academic_year_id UUID NOT NULL,
    locked_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (evidence_id, indicator_id, academic_year_id)
);
CREATE INDEX idx_qa_evidence_links_indicator ON qa_evidence_links(indicator_id, academic_year_id);
