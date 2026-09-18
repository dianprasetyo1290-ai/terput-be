UPDATE report_assessment_types
SET bucket = 'formatif', exam_linked = false, updated_at = now()
WHERE bucket IN ('sts', 'sas') AND lower(name) ~ '\m(pts|uts|pas|uas)\M';
