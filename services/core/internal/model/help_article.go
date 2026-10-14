package model

import (
	"errors"
	"slices"
	"strings"
	"time"
)

// Help article audiences and kinds (BUG-165, menu 50).
const (
	HelpAudienceParent  = "orang_tua"
	HelpAudienceStudent = "siswa"
	HelpKindGuide       = "guide"
	HelpKindFAQ         = "faq"
)

// HelpArticle is one guide or FAQ entry. SchoolID nil = platform default
// shown to every school until the school replaces or hides it.
type HelpArticle struct {
	ID         string    `json:"id"`
	SchoolID   *string   `json:"school_id,omitempty"`
	Audience   string    `json:"audience"`
	Kind       string    `json:"kind"`
	Title      string    `json:"title"`
	Body       string    `json:"body"`
	SortOrder  int       `json:"sort_order"`
	Published  bool      `json:"published"`
	ReplacesID *string   `json:"replaces_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	// IsDefault and ReplacedBy are computed for the admin list.
	IsDefault  bool    `json:"is_default"`
	ReplacedBy *string `json:"replaced_by,omitempty"`
}

var (
	ErrHelpAudience = errors.New("audiens harus orang_tua atau siswa")
	ErrHelpKind     = errors.New("jenis harus guide (panduan) atau faq")
	ErrHelpTitle    = errors.New("judul wajib diisi (maksimal 200 karakter)")
	ErrHelpBody     = errors.New("isi wajib diisi (maksimal 20.000 karakter)")
)

// ValidateHelpArticle checks the editable fields.
func ValidateHelpArticle(a HelpArticle) error {
	switch {
	case a.Audience != HelpAudienceParent && a.Audience != HelpAudienceStudent:
		return ErrHelpAudience
	case a.Kind != HelpKindGuide && a.Kind != HelpKindFAQ:
		return ErrHelpKind
	case strings.TrimSpace(a.Title) == "" || len(a.Title) > 200:
		return ErrHelpTitle
	case strings.TrimSpace(a.Body) == "" || len(a.Body) > 20000:
		return ErrHelpBody
	}
	return nil
}

// MergeHelpArticles combines platform defaults with a school's own articles:
// a school article whose ReplacesID names a default replaces it (an
// unpublished replacement hides it). With readerView only published articles
// are returned; otherwise every row is returned and replaced defaults carry
// ReplacedBy. Result is ordered by sort_order, then title.
func MergeHelpArticles(rows []HelpArticle, readerView bool) []HelpArticle {
	replacedBy := map[string]string{}
	for _, a := range rows {
		if a.SchoolID != nil && a.ReplacesID != nil {
			replacedBy[*a.ReplacesID] = a.ID
		}
	}
	out := make([]HelpArticle, 0, len(rows))
	for _, a := range rows {
		a.IsDefault = a.SchoolID == nil
		if by, ok := replacedBy[a.ID]; ok && a.IsDefault {
			if readerView {
				continue
			}
			a.ReplacedBy = &by
		}
		if readerView && !a.Published {
			continue
		}
		out = append(out, a)
	}
	slices.SortStableFunc(out, func(x, y HelpArticle) int {
		if x.SortOrder != y.SortOrder {
			return x.SortOrder - y.SortOrder
		}
		return strings.Compare(x.Title, y.Title)
	})
	return out
}
