package qa

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// RecordAudit writes an event to audit_events.
func (s *Store) RecordAudit(ctx context.Context, schoolID, entityType, entityID, action, actorID, actorRole string, before, after interface{}, reason string) error {
	var bJSON, aJSON []byte
	if before != nil {
		bJSON, _ = json.Marshal(before)
	}
	if after != nil {
		aJSON, _ = json.Marshal(after)
	}

	query := `
		INSERT INTO audit_events (school_id, module, entity_type, entity_id, action, actor_user_id, actor_role, before, after, reason)
		VALUES ($1, 'quality', $2, $3, $4, NULLIF($5, '')::uuid, $6, $7, $8, $9)
	`
	_, err := s.pool.Exec(ctx, query, schoolID, entityType, entityID, action, actorID, actorRole, bJSON, aJSON, reason)
	return err
}

// ------------------- Instruments, Standards, Items, Indicators -------------------

func (s *Store) ListInstruments(ctx context.Context, schoolID string) ([]QAInstrument, error) {
	query := `
		SELECT id, school_id, code, name, issuer, version_label, score_min, score_max, status,
		       effective_from::text, created_at, updated_at
		FROM qa_instruments
		WHERE school_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`
	rows, err := s.pool.Query(ctx, query, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []QAInstrument
	for rows.Next() {
		var inst QAInstrument
		var eff sql.NullString
		if err := rows.Scan(&inst.ID, &inst.SchoolID, &inst.Code, &inst.Name, &inst.Issuer,
			&inst.VersionLabel, &inst.ScoreMin, &inst.ScoreMax, &inst.Status, &eff, &inst.CreatedAt, &inst.UpdatedAt); err != nil {
			return nil, err
		}
		if eff.Valid {
			inst.EffectiveFrom = &eff.String
		}
		list = append(list, inst)
	}
	if list == nil {
		list = []QAInstrument{}
	}
	return list, nil
}

func (s *Store) GetInstrument(ctx context.Context, schoolID, id string) (*QAInstrument, error) {
	query := `
		SELECT id, school_id, code, name, issuer, version_label, score_min, score_max, status,
		       effective_from::text, created_at, updated_at
		FROM qa_instruments
		WHERE id = $1 AND school_id::text = $2 AND deleted_at IS NULL
	`
	var inst QAInstrument
	var eff sql.NullString
	err := s.pool.QueryRow(ctx, query, id, schoolID).Scan(
		&inst.ID, &inst.SchoolID, &inst.Code, &inst.Name, &inst.Issuer,
		&inst.VersionLabel, &inst.ScoreMin, &inst.ScoreMax, &inst.Status, &eff, &inst.CreatedAt, &inst.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if eff.Valid {
		inst.EffectiveFrom = &eff.String
	}

	// Fetch tree of standards -> items -> indicators
	stdRows, err := s.pool.Query(ctx, `
		SELECT id, school_id, instrument_id, sort, code, name, weight_bp, created_at, updated_at
		FROM qa_standards WHERE instrument_id = $1 ORDER BY sort, code ASC
	`, inst.ID)
	if err != nil {
		return &inst, nil
	}
	defer stdRows.Close()

	var standards []QAStandard
	for stdRows.Next() {
		var std QAStandard
		if err := stdRows.Scan(&std.ID, &std.SchoolID, &std.InstrumentID, &std.Sort, &std.Code, &std.Name, &std.WeightBP, &std.CreatedAt, &std.UpdatedAt); err == nil {
			standards = append(standards, std)
		}
	}

	for i := range standards {
		itRows, err := s.pool.Query(ctx, `
			SELECT id, school_id, standard_id, sort, code, statement, created_at, updated_at
			FROM qa_items WHERE standard_id = $1 ORDER BY sort, code ASC
		`, standards[i].ID)
		if err == nil {
			var items []QAItem
			for itRows.Next() {
				var it QAItem
				if err := itRows.Scan(&it.ID, &it.SchoolID, &it.StandardID, &it.Sort, &it.Code, &it.Statement, &it.CreatedAt, &it.UpdatedAt); err == nil {
					items = append(items, it)
				}
			}
			itRows.Close()

			for j := range items {
				indRows, err := s.pool.Query(ctx, `
					SELECT id, school_id, item_id, sort, code, name, weight_bp, unit, direction, source_kind, metric_key, rubric, created_at, updated_at
					FROM qa_indicators WHERE item_id = $1 AND deleted_at IS NULL ORDER BY sort, code ASC
				`, items[j].ID)
				if err == nil {
					var indicators []QAIndicator
					for indRows.Next() {
						var ind QAIndicator
						var rubricRaw []byte
						if err := indRows.Scan(&ind.ID, &ind.SchoolID, &ind.ItemID, &ind.Sort, &ind.Code, &ind.Name, &ind.WeightBP, &ind.Unit, &ind.Direction, &ind.SourceKind, &ind.MetricKey, &rubricRaw, &ind.CreatedAt, &ind.UpdatedAt); err == nil {
							_ = json.Unmarshal(rubricRaw, &ind.Rubric)
							indicators = append(indicators, ind)
						}
					}
					indRows.Close()
					items[j].Indicators = indicators
				}
			}
			standards[i].Items = items
		}
	}
	inst.Standards = standards
	return &inst, nil
}

func (s *Store) CreateInstrument(ctx context.Context, inst QAInstrument) (*QAInstrument, error) {
	query := `
		INSERT INTO qa_instruments (school_id, code, name, issuer, version_label, score_min, score_max, status, effective_from)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, '')::date)
		RETURNING id, created_at, updated_at
	`
	var eff string
	if inst.EffectiveFrom != nil {
		eff = *inst.EffectiveFrom
	}
	err := s.pool.QueryRow(ctx, query, inst.SchoolID, inst.Code, inst.Name, inst.Issuer, inst.VersionLabel, inst.ScoreMin, inst.ScoreMax, inst.Status, eff).Scan(
		&inst.ID, &inst.CreatedAt, &inst.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &inst, nil
}

func (s *Store) UpdateInstrument(ctx context.Context, inst QAInstrument) error {
	var eff string
	if inst.EffectiveFrom != nil {
		eff = *inst.EffectiveFrom
	}
	query := `
		UPDATE qa_instruments
		SET code = $3, name = $4, issuer = $5, version_label = $6, score_min = $7, score_max = $8,
		    status = $9, effective_from = NULLIF($10, '')::date, updated_at = now()
		WHERE id = $1 AND school_id = $2 AND deleted_at IS NULL
	`
	_, err := s.pool.Exec(ctx, query, inst.ID, inst.SchoolID, inst.Code, inst.Name, inst.Issuer, inst.VersionLabel, inst.ScoreMin, inst.ScoreMax, inst.Status, eff)
	return err
}

func (s *Store) DeleteInstrument(ctx context.Context, schoolID, id string) error {
	query := `UPDATE qa_instruments SET deleted_at = now(), updated_at = now() WHERE id = $1 AND school_id = $2`
	_, err := s.pool.Exec(ctx, query, id, schoolID)
	return err
}

// ------------------- Standards, Items, Indicators Mutations -------------------

func (s *Store) CreateStandard(ctx context.Context, std QAStandard) (*QAStandard, error) {
	query := `
		INSERT INTO qa_standards (school_id, instrument_id, sort, code, name, weight_bp)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`
	err := s.pool.QueryRow(ctx, query, std.SchoolID, std.InstrumentID, std.Sort, std.Code, std.Name, std.WeightBP).Scan(
		&std.ID, &std.CreatedAt, &std.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &std, nil
}

func (s *Store) CreateItem(ctx context.Context, it QAItem) (*QAItem, error) {
	query := `
		INSERT INTO qa_items (school_id, standard_id, sort, code, statement)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at
	`
	err := s.pool.QueryRow(ctx, query, it.SchoolID, it.StandardID, it.Sort, it.Code, it.Statement).Scan(
		&it.ID, &it.CreatedAt, &it.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &it, nil
}

func (s *Store) CreateIndicator(ctx context.Context, ind QAIndicator) (*QAIndicator, error) {
	rBytes, _ := json.Marshal(ind.Rubric)
	query := `
		INSERT INTO qa_indicators (school_id, item_id, sort, code, name, weight_bp, unit, direction, source_kind, metric_key, rubric)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, created_at, updated_at
	`
	err := s.pool.QueryRow(ctx, query, ind.SchoolID, ind.ItemID, ind.Sort, ind.Code, ind.Name, ind.WeightBP, ind.Unit, ind.Direction, ind.SourceKind, ind.MetricKey, rBytes).Scan(
		&ind.ID, &ind.CreatedAt, &ind.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &ind, nil
}

func (s *Store) GetIndicator(ctx context.Context, schoolID, id string) (*QAIndicator, error) {
	query := `
		SELECT id, school_id, item_id, sort, code, name, weight_bp, unit, direction, source_kind, metric_key, rubric, created_at, updated_at
		FROM qa_indicators WHERE id = $1 AND school_id::text = $2 AND deleted_at IS NULL
	`
	var ind QAIndicator
	var rBytes []byte
	err := s.pool.QueryRow(ctx, query, id, schoolID).Scan(
		&ind.ID, &ind.SchoolID, &ind.ItemID, &ind.Sort, &ind.Code, &ind.Name, &ind.WeightBP, &ind.Unit, &ind.Direction, &ind.SourceKind, &ind.MetricKey, &rBytes, &ind.CreatedAt, &ind.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	_ = json.Unmarshal(rBytes, &ind.Rubric)
	return &ind, nil
}

// ------------------- Targets -------------------

func (s *Store) ListTargets(ctx context.Context, schoolID, academicYearID string) ([]QATarget, error) {
	query := `
		SELECT id, school_id, indicator_id, academic_year_id, target_value, owner_user_id, created_at, updated_at
		FROM qa_targets
		WHERE school_id = $1 AND ($2 = '' OR academic_year_id::text = $2)
		ORDER BY created_at DESC
	`
	rows, err := s.pool.Query(ctx, query, schoolID, academicYearID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var targets []QATarget
	for rows.Next() {
		var t QATarget
		if err := rows.Scan(&t.ID, &t.SchoolID, &t.IndicatorID, &t.AcademicYearID, &t.TargetValue, &t.OwnerUserID, &t.CreatedAt, &t.UpdatedAt); err == nil {
			targets = append(targets, t)
		}
	}
	if targets == nil {
		targets = []QATarget{}
	}
	return targets, nil
}

func (s *Store) UpsertTarget(ctx context.Context, t QATarget) (*QATarget, error) {
	query := `
		INSERT INTO qa_targets (school_id, indicator_id, academic_year_id, target_value, owner_user_id)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (school_id, indicator_id, academic_year_id)
		DO UPDATE SET target_value = EXCLUDED.target_value, owner_user_id = EXCLUDED.owner_user_id, updated_at = now()
		RETURNING id, created_at, updated_at
	`
	err := s.pool.QueryRow(ctx, query, t.SchoolID, t.IndicatorID, t.AcademicYearID, t.TargetValue, t.OwnerUserID).Scan(
		&t.ID, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ------------------- Evidence & Verification -------------------

func (s *Store) ListEvidence(ctx context.Context, schoolID, indicatorID, standardID, status string) ([]QAEvidence, error) {
	query := `
		SELECT DISTINCT e.id, e.school_id, e.kind, e.title, e.object_key, e.mime, e.size_bytes, e.checksum,
		       e.report_ref, e.url, e.version, e.supersedes_id, e.uploaded_by, e.status, e.verified_by, e.verified_at, e.reject_reason,
		       e.created_at, e.updated_at
		FROM qa_evidence e
		LEFT JOIN qa_evidence_links el ON el.evidence_id = e.id
		LEFT JOIN qa_indicators ind ON ind.id = el.indicator_id
		LEFT JOIN qa_items it ON it.id = ind.item_id
		WHERE e.school_id = $1 AND e.deleted_at IS NULL
		  AND ($2 = '' OR el.indicator_id::text = $2)
		  AND ($3 = '' OR it.standard_id::text = $3)
		  AND ($4 = '' OR e.status = $4)
		ORDER BY e.created_at DESC
	`
	rows, err := s.pool.Query(ctx, query, schoolID, indicatorID, standardID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []QAEvidence
	for rows.Next() {
		var ev QAEvidence
		var rRefBytes []byte
		if err := rows.Scan(
			&ev.ID, &ev.SchoolID, &ev.Kind, &ev.Title, &ev.ObjectKey, &ev.Mime, &ev.SizeBytes, &ev.Checksum,
			&rRefBytes, &ev.URL, &ev.Version, &ev.SupersedesID, &ev.UploadedBy, &ev.Status, &ev.VerifiedBy,
			&ev.VerifiedAt, &ev.RejectReason, &ev.CreatedAt, &ev.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if len(rRefBytes) > 0 {
			_ = json.Unmarshal(rRefBytes, &ev.ReportRef)
		}
		list = append(list, ev)
	}
	if list == nil {
		list = []QAEvidence{}
	}
	return list, nil
}

func (s *Store) CreateEvidence(ctx context.Context, ev QAEvidence, indicatorIDs []string, academicYearID string) (*QAEvidence, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var rBytes []byte
	if ev.ReportRef != nil {
		rBytes, _ = json.Marshal(ev.ReportRef)
	}

	query := `
		INSERT INTO qa_evidence (school_id, kind, title, object_key, mime, size_bytes, checksum, report_ref, url, version, supersedes_id, uploaded_by, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, 'pending')
		RETURNING id, created_at, updated_at
	`
	err = tx.QueryRow(ctx, query, ev.SchoolID, ev.Kind, ev.Title, ev.ObjectKey, ev.Mime, ev.SizeBytes, ev.Checksum, rBytes, ev.URL, ev.Version, ev.SupersedesID, ev.UploadedBy).Scan(
		&ev.ID, &ev.CreatedAt, &ev.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	for _, indID := range indicatorIDs {
		_, err = tx.Exec(ctx, `
			INSERT INTO qa_evidence_links (evidence_id, indicator_id, academic_year_id)
			VALUES ($1, $2, $3)
			ON CONFLICT DO NOTHING
		`, ev.ID, indID, academicYearID)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	ev.IndicatorIDs = indicatorIDs
	return &ev, nil
}

func (s *Store) VerifyEvidence(ctx context.Context, schoolID, id, decision, reason, verifiedBy string) error {
	query := `
		UPDATE qa_evidence
		SET status = $3, reject_reason = NULLIF($4, ''), verified_by = $5::uuid, verified_at = now(), updated_at = now()
		WHERE id = $1 AND school_id = $2 AND deleted_at IS NULL
	`
	res, err := s.pool.Exec(ctx, query, id, schoolID, decision, reason, verifiedBy)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return errors.New("evidence not found")
	}
	return nil
}

func (s *Store) GetCompleteness(ctx context.Context, schoolID, instrumentID, academicYearID string) (int64, error) {
	// Total indicators for instrument
	var total int
	err := s.pool.QueryRow(ctx, `
		SELECT count(ind.id)
		FROM qa_indicators ind
		JOIN qa_items it ON it.id = ind.item_id
		JOIN qa_standards std ON std.id = it.standard_id
		WHERE std.instrument_id = $1 AND ind.deleted_at IS NULL
	`, instrumentID).Scan(&total)
	if err != nil {
		return 0, err
	}

	// Indicators with verified evidence
	var verified int
	err = s.pool.QueryRow(ctx, `
		SELECT count(DISTINCT el.indicator_id)
		FROM qa_evidence_links el
		JOIN qa_evidence e ON e.id = el.evidence_id
		JOIN qa_indicators ind ON ind.id = el.indicator_id
		JOIN qa_items it ON it.id = ind.item_id
		JOIN qa_standards std ON std.id = it.standard_id
		WHERE std.instrument_id = $1 AND el.academic_year_id = $2
		  AND e.status = 'verified' AND e.deleted_at IS NULL
	`, instrumentID, academicYearID).Scan(&verified)
	if err != nil {
		return 0, err
	}

	return Completeness(total, verified), nil
}

// ------------------- Self Evaluations (EDS) -------------------

func (s *Store) ListSelfEvaluations(ctx context.Context, schoolID string) ([]QASelfEvaluation, error) {
	query := `
		SELECT id, school_id, instrument_id, academic_year_id, period_label, period_from::text, period_to::text,
		       status, approval_id::text, reopen_approval_id::text, finalized_at, finalized_by::text, version, created_at, updated_at
		FROM qa_self_evaluations
		WHERE school_id = $1
		ORDER BY created_at DESC
	`
	rows, err := s.pool.Query(ctx, query, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []QASelfEvaluation
	for rows.Next() {
		var se QASelfEvaluation
		var appID, reopenID, finBy sql.NullString
		if err := rows.Scan(&se.ID, &se.SchoolID, &se.InstrumentID, &se.AcademicYearID, &se.PeriodLabel,
			&se.PeriodFrom, &se.PeriodTo, &se.Status, &appID, &reopenID, &se.FinalizedAt, &finBy, &se.Version, &se.CreatedAt, &se.UpdatedAt); err != nil {
			return nil, err
		}
		if appID.Valid {
			se.ApprovalID = &appID.String
		}
		if reopenID.Valid {
			se.ReopenApprovalID = &reopenID.String
		}
		if finBy.Valid {
			se.FinalizedBy = &finBy.String
		}
		list = append(list, se)
	}
	if list == nil {
		list = []QASelfEvaluation{}
	}
	return list, nil
}

func (s *Store) GetSelfEvaluation(ctx context.Context, schoolID, id string) (*QASelfEvaluation, error) {
	query := `
		SELECT id, school_id, instrument_id, academic_year_id, period_label, period_from::text, period_to::text,
		       status, approval_id::text, reopen_approval_id::text, finalized_at, finalized_by::text, version, created_at, updated_at
		FROM qa_self_evaluations
		WHERE id = $1 AND school_id::text = $2
	`
	var se QASelfEvaluation
	var appID, reopenID, finBy sql.NullString
	err := s.pool.QueryRow(ctx, query, id, schoolID).Scan(
		&se.ID, &se.SchoolID, &se.InstrumentID, &se.AcademicYearID, &se.PeriodLabel,
		&se.PeriodFrom, &se.PeriodTo, &se.Status, &appID, &reopenID, &se.FinalizedAt, &finBy, &se.Version, &se.CreatedAt, &se.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if appID.Valid {
		se.ApprovalID = &appID.String
	}
	if reopenID.Valid {
		se.ReopenApprovalID = &reopenID.String
	}
	if finBy.Valid {
		se.FinalizedBy = &finBy.String
	}

	// Fetch items
	itRows, err := s.pool.Query(ctx, `
		SELECT sei.id, sei.school_id, sei.self_evaluation_id, sei.indicator_id, sei.target_value,
		       sei.actual_value, sei.value_source, sei.metric_as_of, sei.metric_fetched_at, sei.score,
		       sei.below_target, sei.note, sei.assessed_by::text, sei.created_at, sei.updated_at
		FROM qa_self_eval_items sei
		WHERE sei.self_evaluation_id = $1
	`, se.ID)
	if err == nil {
		var items []QASelfEvalItem
		for itRows.Next() {
			var it QASelfEvalItem
			var assessedBy sql.NullString
			if err := itRows.Scan(&it.ID, &it.SchoolID, &it.SelfEvaluationID, &it.IndicatorID, &it.TargetValue,
				&it.ActualValue, &it.ValueSource, &it.MetricAsOf, &it.MetricFetchedAt, &it.Score,
				&it.BelowTarget, &it.Note, &assessedBy, &it.CreatedAt, &it.UpdatedAt); err == nil {
				if assessedBy.Valid {
					it.AssessedBy = &assessedBy.String
				}
				items = append(items, it)
			}
		}
		itRows.Close()

		// Fetch action plans for each item
		for i := range items {
			planRows, err := s.pool.Query(ctx, `
				SELECT id, school_id, self_eval_item_id, action, owner_user_id, due_date::text, status, done_at, created_at, updated_at
				FROM qa_action_plans WHERE self_eval_item_id = $1 AND deleted_at IS NULL
			`, items[i].ID)
			if err == nil {
				var plans []QAActionPlan
				todayWIB := time.Now().UTC().Add(7 * time.Hour).Format("2006-01-02")
				for planRows.Next() {
					var p QAActionPlan
					if err := planRows.Scan(&p.ID, &p.SchoolID, &p.SelfEvalItemID, &p.Action, &p.OwnerUserID, &p.DueDate, &p.Status, &p.DoneAt, &p.CreatedAt, &p.UpdatedAt); err == nil {
						if p.DueDate < todayWIB && (p.Status == "open" || p.Status == "in_progress") {
							p.IsOverdue = true
						}
						plans = append(plans, p)
					}
				}
				planRows.Close()
				items[i].ActionPlans = plans
			}
		}
		se.Items = items
	}
	return &se, nil
}

func (s *Store) CreateSelfEvaluation(ctx context.Context, se QASelfEvaluation) (*QASelfEvaluation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	query := `
		INSERT INTO qa_self_evaluations (school_id, instrument_id, academic_year_id, period_label, period_from, period_to, status, version)
		VALUES ($1, $2, $3, $4, $5::date, $6::date, 'draft', 0)
		RETURNING id, created_at, updated_at
	`
	err = tx.QueryRow(ctx, query, se.SchoolID, se.InstrumentID, se.AcademicYearID, se.PeriodLabel, se.PeriodFrom, se.PeriodTo).Scan(
		&se.ID, &se.CreatedAt, &se.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Auto-seed self eval items for all indicators in instrument
	indRows, err := tx.Query(ctx, `
		SELECT ind.id, ind.source_kind, tgt.target_value
		FROM qa_indicators ind
		JOIN qa_items it ON it.id = ind.item_id
		JOIN qa_standards std ON std.id = it.standard_id
		LEFT JOIN qa_targets tgt ON tgt.indicator_id = ind.id AND tgt.academic_year_id = $2
		WHERE std.instrument_id = $1 AND ind.deleted_at IS NULL
	`, se.InstrumentID, se.AcademicYearID)
	if err != nil {
		return nil, err
	}
	defer indRows.Close()

	type indSeed struct {
		id     string
		source string
		target *int64
	}
	var seeds []indSeed
	for indRows.Next() {
		var is indSeed
		if err := indRows.Scan(&is.id, &is.source, &is.target); err == nil {
			seeds = append(seeds, is)
		}
	}
	indRows.Close()

	for _, seed := range seeds {
		_, err = tx.Exec(ctx, `
			INSERT INTO qa_self_eval_items (school_id, self_evaluation_id, indicator_id, target_value, value_source)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT DO NOTHING
		`, se.SchoolID, se.ID, seed.id, seed.target, seed.source)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	se.Status = "draft"
	return &se, nil
}

// UpdateSelfEvalItem updates score, actual_value, note for an indicator item in an evaluation.
// BR-QA-01: Verifies evaluation is not locked (or returns ErrLocked).
var ErrEvaluationLocked = errors.New("evaluasi diri sudah difinalisasi; ajukan pembukaan kembali")

func (s *Store) UpdateSelfEvalItem(ctx context.Context, schoolID, selfEvalID, indicatorID string, actualVal *int64, score *int, note *string, assessedBy string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// SELECT ... FOR UPDATE to lock row and verify status
	var evalStatus string
	err = tx.QueryRow(ctx, `SELECT status FROM qa_self_evaluations WHERE id = $1 AND school_id = $2 FOR UPDATE`, selfEvalID, schoolID).Scan(&evalStatus)
	if err != nil {
		return err
	}
	if IsLocked(evalStatus) {
		return ErrEvaluationLocked
	}

	// Fetch indicator direction and target
	var direction string
	var targetVal *int64
	err = tx.QueryRow(ctx, `
		SELECT ind.direction, sei.target_value
		FROM qa_self_eval_items sei
		JOIN qa_indicators ind ON ind.id = sei.indicator_id
		WHERE sei.self_evaluation_id = $1 AND sei.indicator_id = $2
	`, selfEvalID, indicatorID).Scan(&direction, &targetVal)
	if err != nil {
		return err
	}

	belowTarget := false
	if targetVal != nil && actualVal != nil {
		belowTarget = BelowTarget(actualVal, *targetVal, direction)
	}

	query := `
		UPDATE qa_self_eval_items
		SET actual_value = $3, score = $4, below_target = $5, note = $6, assessed_by = NULLIF($7, '')::uuid, updated_at = now()
		WHERE self_evaluation_id = $1 AND indicator_id = $2
	`
	_, err = tx.Exec(ctx, query, selfEvalID, indicatorID, actualVal, score, belowTarget, note, assessedBy)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ------------------- Approval Center Callbacks -------------------

func (s *Store) HandleApprovalCallback(ctx context.Context, approvalID, requestType, decision, decidedBy, comment string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	switch requestType {
	case "qa_self_eval_finalize":
		var se QASelfEvaluation
		err := tx.QueryRow(ctx, `
			SELECT id, school_id, instrument_id, academic_year_id, version, status
			FROM qa_self_evaluations WHERE approval_id = $1 FOR UPDATE
		`, approvalID).Scan(&se.ID, &se.SchoolID, &se.InstrumentID, &se.AcademicYearID, &se.Version, &se.Status)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil // idempotent / already processed
			}
			return err
		}

		if decision == "approved" {
			newVersion := se.Version + 1
			// Build snapshot of items and score
			snapshot := map[string]interface{}{
				"finalized_at": time.Now(),
				"decided_by":   decidedBy,
				"comment":      comment,
				"version":      newVersion,
			}
			snapBytes, _ := json.Marshal(snapshot)

			_, err = tx.Exec(ctx, `
				UPDATE qa_self_evaluations
				SET status = 'finalized', version = $2, finalized_at = now(), finalized_by = NULLIF($3, '')::uuid, updated_at = now()
				WHERE id = $1
			`, se.ID, newVersion, decidedBy)
			if err != nil {
				return err
			}

			// Store version snapshot
			_, err = tx.Exec(ctx, `
				INSERT INTO qa_self_eval_versions (school_id, self_evaluation_id, version, snapshot, finalized_at, finalized_by)
				VALUES ($1, $2, $3, $4, now(), NULLIF($5, '')::uuid)
				ON CONFLICT (self_evaluation_id, version) DO NOTHING
			`, se.SchoolID, se.ID, newVersion, snapBytes, decidedBy)
			if err != nil {
				return err
			}

			// Lock linked evidence
			_, err = tx.Exec(ctx, `
				UPDATE qa_evidence_links
				SET locked_at = now()
				WHERE indicator_id IN (
					SELECT ind.id FROM qa_indicators ind
					JOIN qa_items it ON it.id = ind.item_id
					JOIN qa_standards std ON std.id = it.standard_id
					WHERE std.instrument_id = $1
				) AND academic_year_id = $2
			`, se.InstrumentID, se.AcademicYearID)
			if err != nil {
				return err
			}
		} else {
			// rejected -> back to draft
			_, err = tx.Exec(ctx, `UPDATE qa_self_evaluations SET status = 'draft', updated_at = now() WHERE id = $1`, se.ID)
			if err != nil {
				return err
			}
		}

	case "qa_self_eval_reopen":
		var se QASelfEvaluation
		err := tx.QueryRow(ctx, `
			SELECT id, school_id, instrument_id, academic_year_id, status
			FROM qa_self_evaluations WHERE reopen_approval_id = $1 FOR UPDATE
		`, approvalID).Scan(&se.ID, &se.SchoolID, &se.InstrumentID, &se.AcademicYearID, &se.Status)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}

		if decision == "approved" {
			_, err = tx.Exec(ctx, `
				UPDATE qa_self_evaluations SET status = 'draft', updated_at = now() WHERE id = $1
			`, se.ID)
			if err != nil {
				return err
			}
			// Unlock linked evidence
			_, err = tx.Exec(ctx, `
				UPDATE qa_evidence_links
				SET locked_at = NULL
				WHERE indicator_id IN (
					SELECT ind.id FROM qa_indicators ind
					JOIN qa_items it ON it.id = ind.item_id
					JOIN qa_standards std ON std.id = it.standard_id
					WHERE std.instrument_id = $1
				) AND academic_year_id = $2
			`, se.InstrumentID, se.AcademicYearID)
			if err != nil {
				return err
			}
		} else {
			_, err = tx.Exec(ctx, `UPDATE qa_self_evaluations SET status = 'finalized', updated_at = now() WHERE id = $1`, se.ID)
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit(ctx)
}
