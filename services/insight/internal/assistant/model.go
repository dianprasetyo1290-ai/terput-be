package assistant

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"

	"skci/insight/internal/common"
)

type AssistantSettings struct {
	SchoolID         string    `json:"school_id"`
	LogRetentionDays *int      `json:"log_retention_days,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type HelpArticle struct {
	ID        string    `json:"id"`
	SchoolID  *string   `json:"school_id,omitempty"`
	Title     string    `json:"title"`
	BodyMd    string    `json:"body_md"`
	Tags      []string  `json:"tags"`
	RouteName *string   `json:"route_name,omitempty"`
	Status    string    `json:"status"` // 'draft', 'published'
	UpdatedBy *string   `json:"updated_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AssistantIntent struct {
	ID        string    `json:"id"`
	SchoolID  *string   `json:"school_id,omitempty"`
	Key       string    `json:"key"`
	Phrases   []string  `json:"phrases"`
	ToolKey   string    `json:"tool_key"`
	Roles     []string  `json:"roles"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AssistantConversation struct {
	ID            string    `json:"id"`
	SchoolID      string    `json:"school_id"`
	UserID        string    `json:"user_id"`
	ActiveRole    string    `json:"active_role"`
	StartedAt     time.Time `json:"started_at"`
	LastMessageAt time.Time `json:"last_message_at"`
}

type AssistantMessage struct {
	ID             string                 `json:"id"`
	SchoolID       string                 `json:"school_id"`
	ConversationID string                 `json:"conversation_id"`
	Sender         string                 `json:"sender"` // 'user', 'assistant'
	Content        string                 `json:"content"`
	AIGenerated    bool                   `json:"ai_generated"`
	Sources        []map[string]interface{} `json:"sources"`
	ToolCalls      []map[string]interface{} `json:"tool_calls"`
	Provider       *string                `json:"provider,omitempty"`
	LatencyMs      *int                   `json:"latency_ms,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
}

type AssistantAction struct {
	ID             string                 `json:"id"`
	SchoolID       string                 `json:"school_id"`
	ConversationID string                 `json:"conversation_id"`
	MessageID      string                 `json:"message_id"`
	ActionKey      string                 `json:"action_key"`
	Payload        map[string]interface{} `json:"payload"`
	Preview        map[string]interface{} `json:"preview"`
	Status         string                 `json:"status"` // 'proposed', 'confirmed', 'executed', 'cancelled', 'failed', 'expired'
	ProposedBy     string                 `json:"proposed_by"`
	ConfirmedAt    *time.Time             `json:"confirmed_at,omitempty"`
	ExecutedAt     *time.Time             `json:"executed_at,omitempty"`
	Result         map[string]interface{} `json:"result,omitempty"`
	ExpiresAt      time.Time              `json:"expires_at"`
	IdempotencyKey string                 `json:"idempotency_key"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

// Tokenize and clean text
func tokenize(text string) []string {
	clean := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return ' '
	}, text)
	return strings.Fields(clean)
}

// ClassifyIntent scores query against intents based on token overlap
func ClassifyIntent(text string, intents []AssistantIntent, role string) (string, float64) {
	queryTokens := tokenize(text)
	if len(queryTokens) == 0 {
		return "", 0
	}

	querySet := make(map[string]bool)
	for _, t := range queryTokens {
		querySet[t] = true
	}

	bestTool := ""
	bestScore := 0.0

	for _, intent := range intents {
		// Verify role authorization
		if role != common.RoleSuperadmin && !slices.Contains(intent.Roles, role) {
			continue
		}

		maxIntentScore := 0.0
		for _, phrase := range intent.Phrases {
			phraseTokens := tokenize(phrase)
			if len(phraseTokens) == 0 {
				continue
			}

			overlap := 0
			for _, pt := range phraseTokens {
				if querySet[pt] {
					overlap++
				}
			}

			score := float64(overlap) / float64(len(phraseTokens))
			if score > maxIntentScore {
				maxIntentScore = score
			}
		}

		if maxIntentScore > bestScore {
			bestScore = maxIntentScore
			bestTool = intent.ToolKey
		}
	}

	if bestScore < 0.2 { // threshold
		return "", 0
	}
	return bestTool, bestScore
}

// RetrieveHelp finds published articles matching text
func RetrieveHelp(text string, articles []HelpArticle, k int) []HelpArticle {
	queryTokens := tokenize(text)
	if len(queryTokens) == 0 {
		return nil
	}

	querySet := make(map[string]bool)
	for _, t := range queryTokens {
		querySet[t] = true
	}

	type match struct {
		article HelpArticle
		score   int
	}

	var matches []match
	for _, a := range articles {
		if a.Status != "published" {
			continue
		}

		score := 0
		tokens := tokenize(a.Title + " " + a.BodyMd + " " + strings.Join(a.Tags, " "))
		for _, t := range tokens {
			if querySet[t] {
				score++
			}
		}

		if score > 0 {
			matches = append(matches, match{article: a, score: score})
		}
	}

	slices.SortFunc(matches, func(a, b match) int {
		return b.score - a.score
	})

	if k <= 0 {
		k = 3
	}
	if len(matches) > k {
		matches = matches[:k]
	}

	res := make([]HelpArticle, 0, len(matches))
	for _, m := range matches {
		res = append(res, m.article)
	}
	return res
}

// Minimize removes PII (name, nisn, nik, email, phone, address, student_user_id)
// and maps IDs to S1, S2, etc. (NFR-ADV-01)
func Minimize(payload map[string]interface{}) map[string]interface{} {
	aliasMap := make(map[string]string)
	aliasCounter := 1

	var cleanVal func(val interface{}, key string) interface{}
	cleanVal = func(val interface{}, key string) interface{} {
		lowerKey := strings.ToLower(key)
		switch lowerKey {
		case "name", "nisn", "nik", "email", "phone", "address", "alamat", "telepon", "no_hp", "student_user_id":
			return nil
		}

		switch v := val.(type) {
		case map[string]interface{}:
			res := make(map[string]interface{})
			for k, inner := range v {
				c := cleanVal(inner, k)
				if c != nil {
					res[k] = c
				}
			}
			return res

		case []interface{}:
			res := make([]interface{}, 0, len(v))
			for _, item := range v {
				c := cleanVal(item, key)
				if c != nil {
					res = append(res, c)
				}
			}
			return res

		case string:
			if strings.Contains(lowerKey, "id") || lowerKey == "student" || lowerKey == "user" {
				if v == "" {
					return v
				}
				if alias, ok := aliasMap[v]; ok {
					return alias
				}
				alias := fmt.Sprintf("S%d", aliasCounter)
				aliasCounter++
				aliasMap[v] = alias
				return alias
			}
			return v

		default:
			return v
		}
	}

	out := make(map[string]interface{})
	for k, v := range payload {
		c := cleanVal(v, k)
		if c != nil {
			out[k] = c
		}
	}
	return out
}
