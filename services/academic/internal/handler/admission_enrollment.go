package handler

import (
	"encoding/json"
	"net/http"

	"skci/academic/internal/common"
	"skci/academic/internal/model"
)

// upsertAdmissionEnrollmentRequest is the body admission-service's
// applicant->student conversion saga sends (see
// services/admission/internal/handler/conversion.go, academicAdapter.Enroll).
//
// classroom_id / academic_year_id are optional: admission converts a student
// before a homeroom is necessarily picked, so a conversion with no placement
// yet must still be able to complete the enrollment step.
type upsertAdmissionEnrollmentRequest struct {
	SchoolID       string `json:"school_id"`
	StudentID      string `json:"student_id"`
	ApplicationID  string `json:"application_id"`
	ConversionRef  string `json:"conversion_ref"`
	ClassroomID    string `json:"classroom_id"`
	AcademicYearID string `json:"academic_year_id"`
}

// UpsertAdmissionEnrollment: POST /internal/admission/enrollments/upsert
//
// Idempotent handoff from admission-service's conversion saga (X-Internal-Key,
// no user JWT). Two behaviors:
//
//   - No placement yet (classroom_id or academic_year_id empty): write
//     nothing and answer 200 {"status":"pending_placement",...} so the saga
//     counts the enrollment step as done and the student stays in the class
//     placement queue.
//   - Placement present: upsert into class_enrollments on conflict
//     (academic_year_id, student_user_id) and return the enrollment row.
//
// Because it writes on behalf of a sibling service with no user JWT, the
// target classroom is re-validated against school_id + academic_year_id from
// the body before any write, so a forged/foreign classroom_id can never move
// a row outside its tenant or academic year.
func (h *Handler) UpsertAdmissionEnrollment(w http.ResponseWriter, r *http.Request) {
	var req upsertAdmissionEnrollmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.SchoolID == "" || req.StudentID == "" || req.ApplicationID == "" || req.ConversionRef == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id, student_id, application_id and conversion_ref are required")
		return
	}
	// Internal (X-Internal-Key) callers are trusted and use the body's
	// school_id. A plain user JWT must not be able to name an arbitrary
	// tenant in the body, so scope it to the token and reject a mismatch.
	if !common.IsInternal(r) {
		schoolID, ok := scopeSchoolID(w, r)
		if !ok {
			return
		}
		if schoolID != req.SchoolID {
			common.WriteError(w, http.StatusForbidden, "school_id does not match the caller's tenant")
			return
		}
		req.SchoolID = schoolID
	}

	// No classroom/year yet: do not write anything, just acknowledge so the
	// student lands in the placement queue rather than failing the saga.
	if req.ClassroomID == "" || req.AcademicYearID == "" {
		common.WriteJSON(w, http.StatusOK, map[string]string{
			"status":  "pending_placement",
			"message": "classroom and academic year not yet assigned",
		}, nil)
		return
	}

	classroom, err := h.Store.GetClassroom(r.Context(), req.SchoolID, req.ClassroomID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if classroom == nil {
		// Either the classroom does not exist or it belongs to another school
		// (GetClassroom is school-scoped) — both are a 404 to the caller.
		common.WriteError(w, http.StatusNotFound, "classroom not found for this school")
		return
	}
	if classroom.AcademicYearID != req.AcademicYearID {
		common.WriteError(w, http.StatusBadRequest, "classroom does not belong to the given academic year")
		return
	}

	e, err := h.Store.UpsertAdmissionEnrollment(r.Context(), model.ClassEnrollment{
		SchoolID:       req.SchoolID,
		AcademicYearID: req.AcademicYearID,
		ClassroomID:    req.ClassroomID,
		StudentUserID:  req.StudentID,
		Status:         "active",
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, e, nil)
}
