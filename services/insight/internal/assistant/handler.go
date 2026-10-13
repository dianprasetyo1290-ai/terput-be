package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/insight/internal/assistant/provider"
	"skci/insight/internal/client"
	"skci/insight/internal/common"
	"skci/insight/internal/config"
	"skci/insight/internal/metrics"
)

type Handler struct {
	cfg         *config.Config
	store       *Store
	metricStore *metrics.Store
	client      *client.Client
	aiProvider  provider.Provider
}

func NewHandler(cfg *config.Config, store *Store, metricStore *metrics.Store, client *client.Client, aiProvider provider.Provider) *Handler {
	return &Handler{
		cfg:         cfg,
		store:       store,
		metricStore: metricStore,
		client:      client,
		aiProvider:  aiProvider,
	}
}

type MessageRequest struct {
	ConversationID *string `json:"conversation_id,omitempty"`
	Text           string  `json:"text"`
}

func (h *Handler) PostMessage(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	var req MessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Text == "" {
		common.WriteError(w, http.StatusBadRequest, "Pesan teks wajib diisi")
		return
	}

	conv, err := h.store.GetOrCreateConversation(r.Context(), claims.SchoolID, claims.UserID, claims.ActiveRole, req.ConversationID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat percakapan", err.Error())
		return
	}

	// 1. Record User Message
	userMsg := AssistantMessage{
		SchoolID:       claims.SchoolID,
		ConversationID: conv.ID,
		Sender:         "user",
		Content:        req.Text,
		AIGenerated:    false,
		Sources:        []map[string]interface{}{},
		ToolCalls:      []map[string]interface{}{},
	}
	_, _ = h.store.CreateMessage(r.Context(), userMsg)

	// 2. Classify Intent
	intents, _ := h.store.ListIntents(r.Context(), &claims.SchoolID)
	toolKey, _ := ClassifyIntent(req.Text, intents, claims.ActiveRole)

	var answerText string
	var aiGenerated bool
	var sources []map[string]interface{}
	var toolCalls []map[string]interface{}
	var proposedAction *AssistantAction
	authHeader := r.Header.Get("Authorization")

	switch toolKey {
	case "help_search":
		articles, _ := h.store.ListHelpArticles(r.Context(), &claims.SchoolID, "published")
		matched := RetrieveHelp(req.Text, articles, 3)
		if len(matched) == 0 {
			answerText = "Panduan untuk topik ini belum tersedia."
		} else {
			top := matched[0]
			answerText = fmt.Sprintf("Berikut panduan terkait **%s**:\n\n%s", top.Title, top.BodyMd)
			sources = append(sources, map[string]interface{}{
				"kind":  "help_article",
				"id":    top.ID,
				"title": top.Title,
				"route": top.RouteName,
			})
		}
		toolCalls = append(toolCalls, map[string]interface{}{"tool": "help_search", "status": "ok"})

	case "metric_lookup":
		defs, _ := h.metricStore.ListDefinitions(r.Context(), "active")
		answerText = "Berikut kamus metrik aktif di sistem SEKOCI:\n"
		for _, d := range defs {
			answerText += fmt.Sprintf("- **%s** (%s): %s (Rumus: `%s`)\n", d.Name, d.Key, d.Description, d.Formula)
		}
		sources = append(sources, map[string]interface{}{
			"kind":  "metric_dictionary",
			"count": len(defs),
			"as_of": time.Now(),
		})
		toolCalls = append(toolCalls, map[string]interface{}{"tool": "metric_lookup", "status": "ok"})

	case "my_bills":
		url := fmt.Sprintf("%s/me/bills", h.cfg.BillingURL)
		status, _, errF := h.client.ForwardWithAuth(r.Context(), http.MethodGet, url, authHeader, nil)
		if errF != nil || status != http.StatusOK {
			answerText = "Anda tidak memiliki akses ke data tersebut."
		} else {
			answerText = "Berikut data tagihan Anda yang berhasil diambil dari sistem pembayaran."
			sources = append(sources, map[string]interface{}{"kind": "billing_me_bills", "as_of": time.Now()})
		}
		toolCalls = append(toolCalls, map[string]interface{}{"tool": "my_bills", "status": status})

	case "child_bills":
		// AC-153-01: parents checking child bills. If unauthorized or child not theirs -> reject cleanly!
		url := fmt.Sprintf("%s/parent/children/bills", h.cfg.BillingURL)
		status, body, errF := h.client.ForwardWithAuth(r.Context(), http.MethodGet, url, authHeader, nil)
		if errF != nil || status != http.StatusOK {
			answerText = "Anda tidak memiliki akses ke data tersebut."
		} else {
			answerText = fmt.Sprintf("Berikut ringkasan tagihan anak Anda:\n%s", string(body))
			sources = append(sources, map[string]interface{}{"kind": "billing_child_bills", "as_of": time.Now()})
		}
		toolCalls = append(toolCalls, map[string]interface{}{"tool": "child_bills", "status": status})

	case "arrears_summary":
		url := fmt.Sprintf("%s/admin/finance/summary", h.cfg.BillingURL)
		status, body, errF := h.client.ForwardWithAuth(r.Context(), http.MethodGet, url, authHeader, nil)
		if errF != nil || status != http.StatusOK {
			answerText = "Anda tidak memiliki akses ke data tersebut."
		} else {
			answerText = fmt.Sprintf("Berikut ringkasan tunggakan keuangan sekolah:\n%s", string(body))
			sources = append(sources, map[string]interface{}{"kind": "billing_finance_summary", "as_of": time.Now()})
		}
		toolCalls = append(toolCalls, map[string]interface{}{"tool": "arrears_summary", "status": status})

	case "draft_announcement", "draft_message", "summarize_report":
		// Calls AI provider with minimized data
		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(h.cfg.AITimeoutSeconds)*time.Second)
		defer cancel()

		aiResp, errAI := h.aiProvider.GenerateText(ctx, req.Text)
		if errAI != nil {
			common.WriteError(w, http.StatusServiceUnavailable, "Penyedia AI belum dikonfigurasi atau tidak tersedia")
			return
		}
		answerText = aiResp
		aiGenerated = true
		toolCalls = append(toolCalls, map[string]interface{}{"tool": toolKey, "status": "ok"})

	case "send_payment_reminders":
		// AC-153-02: Action proposing payment reminder. Zero write calls until confirm!
		preview := map[string]interface{}{
			"title":   "Kirim Pengingat Tagihan SPP",
			"message": "Yth. Orang Tua / Wali Murid, mengingatkan bahwa terdapat tagihan SPP yang mendekati / melewati tanggal jatuh tempo.",
		}
		idempotencyKey := fmt.Sprintf("action:reminders:%s:%d", claims.UserID, time.Now().UnixNano())
		act := AssistantAction{
			SchoolID:       claims.SchoolID,
			ConversationID: conv.ID,
			ActionKey:      "send_payment_reminders",
			Payload:        preview,
			Preview:        preview,
			Status:         "proposed",
			ProposedBy:     claims.UserID,
			ExpiresAt:      time.Now().Add(24 * time.Hour),
			IdempotencyKey: idempotencyKey,
		}
		answerText = "Saya telah menyiapkan usulan pengingat tagihan SPP. Silakan tinjau dan klik tombol **Konfirmasi** untuk mengirimkan notifikasi."
		proposedAction = &act
		toolCalls = append(toolCalls, map[string]interface{}{"tool": "send_payment_reminders", "action": "proposed"})

	default:
		// Fallback to searching help articles
		articles, _ := h.store.ListHelpArticles(r.Context(), &claims.SchoolID, "published")
		matched := RetrieveHelp(req.Text, articles, 1)
		if len(matched) > 0 {
			answerText = fmt.Sprintf("Mungkin artikel ini membantu: **%s**\n\n%s", matched[0].Title, matched[0].BodyMd)
			sources = append(sources, map[string]interface{}{"kind": "help_article", "id": matched[0].ID, "title": matched[0].Title})
		} else {
			answerText = "Halo! Saya adalah Asisten SEKOCI. Anda dapat menanyakan panduan penggunaan, kamus metrik, atau tagihan Anda."
		}
	}

	// 3. Record Assistant Message
	providerName := h.aiProvider.Name()
	latency := 50
	asstMsg := AssistantMessage{
		SchoolID:       claims.SchoolID,
		ConversationID: conv.ID,
		Sender:         "assistant",
		Content:        answerText,
		AIGenerated:    aiGenerated,
		Sources:        sources,
		ToolCalls:      toolCalls,
		Provider:       &providerName,
		LatencyMs:      &latency,
	}
	savedMsg, err := h.store.CreateMessage(r.Context(), asstMsg)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan pesan asisten", err.Error())
		return
	}

	if proposedAction != nil {
		proposedAction.MessageID = savedMsg.ID
		createdAction, _ := h.store.CreateAction(r.Context(), *proposedAction)
		proposedAction = createdAction
	}

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"conversation_id": conv.ID,
		"message":         savedMsg,
		"proposed_action": proposedAction,
	})
}

// AC-153-02: Action confirmation
func (h *Handler) ConfirmAction(w http.ResponseWriter, r *http.Request) {
	actionID := chi.URLParam(r, "id")
	claims := common.FromContext(r.Context())
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	act, err := h.store.GetAction(r.Context(), actionID, claims.SchoolID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Aksi tidak ditemukan")
		return
	}

	if act.ProposedBy != claims.UserID {
		common.WriteError(w, http.StatusForbidden, "Aksi hanya dapat dikonfirmasi oleh pengguna yang mengusulkannya")
		return
	}

	if act.ExpiresAt.Before(time.Now()) {
		common.WriteError(w, http.StatusUnprocessableEntity, "Aksi telah kedaluwarsa")
		return
	}

	// Idempotency: if already executed, return current state
	if act.Status == "executed" {
		common.WriteJSON(w, http.StatusOK, map[string]interface{}{"status": "executed", "result": act.Result})
		return
	}

	// Execute action
	var result map[string]interface{}
	if act.ActionKey == "send_payment_reminders" {
		billingURL := fmt.Sprintf("%s/admin/bills/reminders", h.cfg.BillingURL)
		authHeader := r.Header.Get("Authorization")
		status, body, errF := h.client.ForwardWithAuth(r.Context(), http.MethodPost, billingURL, authHeader, act.Payload)
		if errF != nil || status >= 400 {
			result = map[string]interface{}{"error": "Gagal mengirim pengingat ke layanan billing"}
			_ = h.store.UpdateActionStatus(r.Context(), act.ID, "failed", result)
			common.WriteError(w, http.StatusInternalServerError, "Gagal mengeksekusi aksi di layanan billing")
			return
		}
		_ = json.Unmarshal(body, &result)
	}

	_ = h.store.UpdateActionStatus(r.Context(), act.ID, "executed", result)
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status": "executed",
		"result": result,
	})
}

func (h *Handler) CancelAction(w http.ResponseWriter, r *http.Request) {
	actionID := chi.URLParam(r, "id")
	claims := common.FromContext(r.Context())
	act, err := h.store.GetAction(r.Context(), actionID, claims.SchoolID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Aksi tidak ditemukan")
		return
	}
	if act.ProposedBy != claims.UserID {
		common.WriteError(w, http.StatusForbidden, "Aksi hanya dapat dibatalkan oleh pengguna yang mengusulkannya")
		return
	}

	_ = h.store.UpdateActionStatus(r.Context(), act.ID, "cancelled", nil)
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Aksi berhasil dibatalkan"})
}

func (h *Handler) ListConversations(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	convs, err := h.store.ListConversations(r.Context(), claims.SchoolID, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil percakapan", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, convs)
}

func (h *Handler) GetConversation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	messages, err := h.store.ListMessages(r.Context(), id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil pesan", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, messages)
}

// Help Articles CRUD
func (h *Handler) ListHelpArticles(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	articles, err := h.store.ListHelpArticles(r.Context(), &claims.SchoolID, "")
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil panduan", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, articles)
}

func (h *Handler) CreateHelpArticle(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	var a HelpArticle
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Invalid payload")
		return
	}
	if claims.ActiveRole != common.RoleSuperadmin {
		a.SchoolID = &claims.SchoolID
	}
	a.UpdatedBy = &claims.UserID

	created, err := h.store.CreateHelpArticle(r.Context(), a)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat panduan", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created)
}

func (h *Handler) UpdateHelpArticle(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := common.FromContext(r.Context())
	var a HelpArticle
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Invalid payload")
		return
	}
	a.ID = id
	a.UpdatedBy = &claims.UserID

	if err := h.store.UpdateHelpArticle(r.Context(), a); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui panduan", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Panduan berhasil diperbarui"})
}

func (h *Handler) DeleteHelpArticle(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.store.DeleteHelpArticle(r.Context(), id); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menghapus panduan", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Panduan berhasil dihapus"})
}

// Intents CRUD
func (h *Handler) ListIntents(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	intents, err := h.store.ListIntents(r.Context(), &claims.SchoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil intent", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, intents)
}

func (h *Handler) CreateIntent(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	var i AssistantIntent
	if err := json.NewDecoder(r.Body).Decode(&i); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Invalid payload")
		return
	}
	if claims.ActiveRole != common.RoleSuperadmin {
		i.SchoolID = &claims.SchoolID
	}

	created, err := h.store.CreateIntent(r.Context(), i)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat intent", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created)
}

func (h *Handler) UpdateIntent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var i AssistantIntent
	if err := json.NewDecoder(r.Body).Decode(&i); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Invalid payload")
		return
	}
	i.ID = id
	if err := h.store.UpdateIntent(r.Context(), i); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui intent", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Intent berhasil diperbarui"})
}

func (h *Handler) DeleteIntent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.store.DeleteIntent(r.Context(), id); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menghapus intent", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Intent berhasil dihapus"})
}

// Settings
func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	st, err := h.store.GetSettings(r.Context(), claims.SchoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil pengaturan asisten", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, st)
}

func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	var req struct {
		LogRetentionDays *int `json:"log_retention_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Invalid payload")
		return
	}

	if err := h.store.UpdateSettings(r.Context(), claims.SchoolID, req.LogRetentionDays); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui pengaturan asisten", err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Pengaturan asisten berhasil diperbarui"})
}
