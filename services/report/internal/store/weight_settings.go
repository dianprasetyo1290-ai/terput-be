package store

import (
	"context"
	"database/sql"

	"skci/report/internal/model"
)

// GetWeightSettings returns a school's Nilai Akhir / Nilai Sementara
// formula weights, falling back to model.DefaultWeightSettings if the
// school hasn't configured its own (same fallback pattern as
// GetGradeBands).
func (s *Store) GetWeightSettings(ctx context.Context, schoolID string) (model.WeightSettings, error) {
	var w model.WeightSettings
	var updatedBy sql.NullString
	err := s.DB.QueryRowContext(ctx, `
		SELECT school_id, akhir_formatif_weight, akhir_sts_weight, akhir_sas_weight,
			sementara_formatif_weight, sementara_praktik_weight, sementara_sts_weight,
			updated_by, updated_at
		FROM report_weight_settings WHERE school_id = $1`, schoolID).Scan(
		&w.SchoolID, &w.AkhirFormatifWeight, &w.AkhirStsWeight, &w.AkhirSasWeight,
		&w.SementaraFormatifWeight, &w.SementaraPraktikWeight, &w.SementaraStsWeight,
		&updatedBy, &w.UpdatedAt)
	if err == sql.ErrNoRows {
		d := model.DefaultWeightSettings
		d.SchoolID = schoolID
		return d, nil
	}
	if err != nil {
		return model.WeightSettings{}, err
	}
	if updatedBy.Valid {
		w.UpdatedBy = &updatedBy.String
	}
	return w, nil
}

func (s *Store) SetWeightSettings(ctx context.Context, w model.WeightSettings, updatedBy string) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO report_weight_settings (school_id, akhir_formatif_weight, akhir_sts_weight, akhir_sas_weight,
			sementara_formatif_weight, sementara_praktik_weight, sementara_sts_weight, updated_by, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,now())
		ON CONFLICT (school_id) DO UPDATE SET
			akhir_formatif_weight = EXCLUDED.akhir_formatif_weight,
			akhir_sts_weight = EXCLUDED.akhir_sts_weight,
			akhir_sas_weight = EXCLUDED.akhir_sas_weight,
			sementara_formatif_weight = EXCLUDED.sementara_formatif_weight,
			sementara_praktik_weight = EXCLUDED.sementara_praktik_weight,
			sementara_sts_weight = EXCLUDED.sementara_sts_weight,
			updated_by = EXCLUDED.updated_by,
			updated_at = now()`,
		w.SchoolID, w.AkhirFormatifWeight, w.AkhirStsWeight, w.AkhirSasWeight,
		w.SementaraFormatifWeight, w.SementaraPraktikWeight, w.SementaraStsWeight, updatedBy)
	return err
}
