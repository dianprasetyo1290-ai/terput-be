package handler

import (
	"net/http"
	"strings"
	"time"

	"skci/billing/internal/common"
)

// ListDiscounts is GET /admin/discounts — read-only list of the school's
// discount/scholarship rules (bill_discounts) with how much each has actually
// been applied (bill_discount_steps). ?scholarship=true limits to beasiswa.
func (a *App) ListDiscounts(w http.ResponseWriter, r *http.Request) {
	if common.RequireAdmin(w, r) == nil {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id is required")
		return
	}
	scholarship := strings.TrimSpace(r.URL.Query().Get("scholarship")) == "true"

	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT d.id,d.name,d.kind,d.value,d.cap_idr,d.priority,d.scope,COALESCE(s.name,''),COALESCE(d.grade_level,''),
		        COALESCE(d.student_user_id::text,''),d.scholarship,d.active,d.created_at,
		        count(DISTINCT b.student_user_id),count(DISTINCT b.id),COALESCE(sum(st.applied_idr) FILTER (WHERE b.id IS NOT NULL),0)
		 FROM bill_discounts d
		 LEFT JOIN billing_schemes s ON s.id=d.scheme_id
		 LEFT JOIN bill_discount_steps st ON st.discount_id=d.id AND st.school_id=d.school_id
		 LEFT JOIN student_bills b ON b.id=st.bill_id AND b.status<>'void'
		 WHERE d.school_id=$1 AND (NOT $2 OR d.scholarship)
		 GROUP BY d.id,s.name
		 ORDER BY d.active DESC,d.priority,d.created_at`,
		schoolID, scholarship)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		var (
			id, name, kind, scope, schemeName, grade, student string
			value, applied                                    int64
			capIDR                                            *int64
			priority, students, bills                         int
			isScholarship, active                             bool
			createdAt                                         time.Time
		)
		if err := rows.Scan(&id, &name, &kind, &value, &capIDR, &priority, &scope, &schemeName, &grade,
			&student, &isScholarship, &active, &createdAt, &students, &bills, &applied); err != nil {
			writeErr(w, err)
			return
		}
		items = append(items, map[string]any{
			"id": id, "name": name, "kind": kind, "value": value, "cap_idr": capIDR, "priority": priority,
			"scope": scope, "scheme_name": schemeName, "grade_level": grade, "student_user_id": student,
			"scholarship": isScholarship, "active": active, "created_at": createdAt,
			"student_count": students, "bill_count": bills, "applied_idr": applied,
		})
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, items, nil)
}

// ListDiscountRecipients is GET /admin/discounts/recipients — per student and
// discount, the total actually applied to non-void bills (from the immutable
// bill_discount_steps snapshot). Filters: ?student_user_id=, ?scholarship=true.
func (a *App) ListDiscountRecipients(w http.ResponseWriter, r *http.Request) {
	if common.RequireAdmin(w, r) == nil {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id is required")
		return
	}
	studentID := strings.TrimSpace(r.URL.Query().Get("student_user_id"))
	scholarship := strings.TrimSpace(r.URL.Query().Get("scholarship")) == "true"

	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT b.student_user_id::text,COALESCE(st.discount_id::text,''),st.label,st.source,
		        COALESCE(d.kind,''),COALESCE(d.value,0),COALESCE(d.scholarship,false),COALESCE(d.active,false),
		        count(DISTINCT st.bill_id),sum(st.applied_idr),max(st.created_at)
		 FROM bill_discount_steps st
		 JOIN student_bills b ON b.id=st.bill_id AND b.status<>'void'
		 LEFT JOIN bill_discounts d ON d.id=st.discount_id
		 WHERE st.school_id=$1 AND ($2='' OR b.student_user_id::text=$2) AND (NOT $3 OR d.scholarship)
		 GROUP BY b.student_user_id,st.discount_id,st.label,st.source,d.kind,d.value,d.scholarship,d.active
		 ORDER BY max(st.created_at) DESC`,
		schoolID, studentID, scholarship)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		var (
			student, discountID, label, source, kind string
			value, applied                           int64
			isScholarship, active                    bool
			bills                                    int
			lastAt                                   time.Time
		)
		if err := rows.Scan(&student, &discountID, &label, &source, &kind, &value, &isScholarship, &active,
			&bills, &applied, &lastAt); err != nil {
			writeErr(w, err)
			return
		}
		items = append(items, map[string]any{
			"student_user_id": student, "discount_id": discountID, "label": label, "source": source,
			"kind": kind, "value": value, "scholarship": isScholarship, "active": active,
			"bill_count": bills, "applied_idr": applied, "last_applied_at": lastAt,
		})
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, items, nil)
}
