package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"skci/exam/internal/common"
)

func NewRouter(h *Handler, jwtSecret, internalKey string, corsOrigins []string) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(common.SentryMiddleware())
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   corsOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Internal-Key"},
		AllowCredentials: true,
	}))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		common.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"}, nil)
	})

	r.Group(func(pr chi.Router) {
		pr.Use(common.JWTMiddleware(jwtSecret))

		// Kelola Tipe Ujian
		pr.Route("/exam-types", func(rt chi.Router) {
			rt.Post("/", h.CreateExamType)
			rt.Get("/{id}", h.GetExamType)
			rt.Put("/{id}", h.UpdateExamType)
			rt.Delete("/{id}", h.DeleteExamType)
		})

		// Bank Soal
		pr.Route("/question-folders", func(rt chi.Router) {
			rt.Post("/", h.CreateFolder)
			rt.Get("/", h.ListFolders)
			rt.Get("/{id}", h.GetFolder)
			rt.Put("/{id}", h.UpdateFolder)
			rt.Delete("/{id}", h.DeleteFolder)
			rt.Post("/{folderId}/questions", h.CreateQuestion)
			rt.Get("/{folderId}/questions", h.ListQuestions)
		})
		pr.Route("/questions", func(rt chi.Router) {
			rt.Get("/{id}", h.GetQuestion)
			rt.Put("/{id}", h.UpdateQuestion)
			rt.Delete("/{id}", h.DeleteQuestion)
			rt.Post("/{id}/publish", h.PublishQuestion)
		})
		pr.Get("/question-import-template", h.QuestionImportTemplate)

		// Buat Ujian Baru
		pr.Route("/exams", func(rt chi.Router) {
			rt.Post("/", h.CreateExam)
			rt.Get("/{id}", h.GetExam)
			rt.Put("/{id}", h.UpdateExam)
			rt.Delete("/{id}", h.DeleteExam)

			rt.Post("/{id}/questions", h.AddExamQuestion)
			rt.Delete("/{id}/questions/{eqId}", h.RemoveExamQuestion)
			rt.Post("/{id}/questions/import", h.ImportQuestions)
			rt.Post("/{id}/normalize-points", h.NormalizeDraftPoints)
			rt.Post("/{id}/publish", h.PublishExam)
			rt.Post("/{id}/duplicate", h.DuplicateExam)

			// Live monitoring
			rt.Get("/{id}/monitor", h.Monitor)
			rt.Get("/{id}/attempts/{attemptId}", h.AttemptDetail)
			rt.Post("/{id}/attempts/{attemptId}/larang", h.LarangUjian)
			rt.Post("/{id}/students/{userId}/larang", h.LarangUjianByUser)
			rt.Post("/{id}/attempts/{attemptId}/buka-larangan", h.BukaLarangan)
			rt.Post("/{id}/attempts/{attemptId}/blokir", h.Blokir)
			rt.Post("/{id}/attempts/{attemptId}/buka-blokir", h.BukaBlokir)
			rt.Patch("/{id}/attempts/{attemptId}/answers/{answerId}", h.OverrideAnswer)

			// Student-facing
			rt.Post("/{id}/enter", h.EnterExam)
			rt.Get("/{id}/attempt", h.MyAttempt)
			rt.Get("/{id}/attempt/questions/{position}", h.AttemptQuestion)
			rt.Post("/{id}/attempt/answers", h.SubmitAnswer)
			rt.Post("/{id}/attempt/submit", h.SubmitExam)
			rt.Post("/{id}/attempt/focus-lost", h.FocusLost)
		})

		// Voucher (student-level, not exam-scoped — see internal/handler/tokens.go)
		pr.Get("/vouchers", h.ListTokens)
		pr.Post("/vouchers/bulk", h.BulkIssueTokens)
		pr.Post("/vouchers/{tokenId}/regenerate", h.RegenerateToken)
		pr.Post("/students/{userId}/voucher", h.IssueToken)

		pr.Get("/me/exams", h.MyExams)
	})

	// report-service integration: read-only, internal-key-authenticated
	// (falls through to JWT too, see internal_auth.go). Mounted as its own
	// group, not nested inside the /exams or /exam-types Route() blocks
	// above, since a static route and a wildcard route ({id}) covering the
	// same path space only get correct precedence when they're registered
	// in the SAME chi router/group — see identity-service's near-identical
	// /users/export bug, found the same day. /exams and /exam-types (both
	// list endpoints, no wildcard collision risk at that depth) are here
	// too because report-service's actual upstream client calls them
	// directly — found via a real "gagal memuat" bug report reproduced
	// with wget from inside the report container, not just code review.
	r.Group(func(ir chi.Router) {
		ir.Use(common.InternalOrJWTMiddleware(jwtSecret, internalKey))
		ir.Get("/exams/{id}/scores", h.ExamScores)
		ir.Get("/exams", h.ListExams)
		ir.Get("/exam-types", h.ListExamTypes)
	})

	return r
}
