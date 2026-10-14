package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Discount rule eligibility (BR-PPDB-01, TC-09-038)
// ---------------------------------------------------------------------------

// DiscountConditions is the typed form of discount_rules.conditions. Only the
// keys below are understood; anything else is rejected when the rule is saved
// and makes the rule inapplicable at issuance (fail closed).
type DiscountConditions struct {
	TrackIDs []string `json:"track_ids,omitempty"`
	WaveIDs  []string `json:"wave_ids,omitempty"`
	// Sibling requires another child of the same guardian who is an accepted
	// applicant (offered/reregistration) or already an active student
	// (converted).
	Sibling bool `json:"sibling,omitempty"`
}

// ParseDiscountConditions decodes and validates raw rule conditions.
func ParseDiscountConditions(raw []byte) (DiscountConditions, error) {
	var c DiscountConditions
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return c, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return DiscountConditions{}, fmt.Errorf("syarat promo tidak dikenali: %w", err)
	}
	return c, nil
}

// ApplicantFacts are the facts a discount condition is evaluated against.
type ApplicantFacts struct {
	TrackID    string
	WaveID     string
	HasSibling bool
}

// Matches reports whether every configured condition holds.
func (c DiscountConditions) Matches(f ApplicantFacts) bool {
	if len(c.TrackIDs) > 0 && !contains(c.TrackIDs, f.TrackID) {
		return false
	}
	if len(c.WaveIDs) > 0 && !contains(c.WaveIDs, f.WaveID) {
		return false
	}
	if c.Sibling && !f.HasSibling {
		return false
	}
	return true
}

// PromoActive reports whether a promo with an optional validity window is
// usable at the issuance instant. Both bounds are inclusive.
func PromoActive(at time.Time, from, until *time.Time) bool {
	if from != nil && at.Before(*from) {
		return false
	}
	if until != nil && at.After(*until) {
		return false
	}
	return true
}

// PromoQuotaAvailable reports whether a promo still has quota. quota 0 means
// unlimited.
func PromoQuotaAvailable(quota, used int) bool {
	return quota <= 0 || used < quota
}

// ValidatePromoWindow rejects an inverted validity window (TC-09-027).
func ValidatePromoWindow(from, until *time.Time) error {
	if from != nil && until != nil && !until.After(*from) {
		return errors.New("rentang tanggal promo tidak valid: tanggal akhir harus setelah tanggal mulai")
	}
	return nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Per-applicant installments (BUG-003) and BR-PPDB-04 minimum
// ---------------------------------------------------------------------------

// InstallmentLine is one planned installment of a single applicant's charge.
type InstallmentLine struct {
	Sequence    int       `json:"sequence"`
	Description string    `json:"description"`
	Amount      int64     `json:"amount"`
	DueAt       time.Time `json:"due_at"`
}

// PlanInstallments splits net into count installments. When minPct is below
// 100 and count > 1, the first installment is exactly the BR-PPDB-04 minimum so
// paying it on time completes re-registration; the rest is spread evenly with
// the remainder on the last line. Later installments fall due monthly after
// firstDue.
func PlanInstallments(net int64, count, minPct int, firstDue time.Time) []InstallmentLine {
	if count < 1 || net <= 0 {
		count = 1
	}
	if net < 0 {
		net = 0
	}
	lines := make([]InstallmentLine, 0, count)
	add := func(amount int64) {
		seq := len(lines) + 1
		lines = append(lines, InstallmentLine{
			Sequence:    seq,
			Description: fmt.Sprintf("Termin %d", seq),
			Amount:      amount,
			DueAt:       firstDue.AddDate(0, seq-1, 0),
		})
	}
	if count == 1 {
		add(net)
		return lines
	}
	rest, parts := net, count
	if minPct > 0 && minPct < 100 {
		first := MinimumPayment(net, minPct)
		add(first)
		rest -= first
		parts--
	}
	base := rest / int64(parts)
	remainder := rest - base*int64(parts)
	for i := 0; i < parts; i++ {
		amount := base
		if i == parts-1 {
			amount += remainder
		}
		add(amount)
	}
	return lines
}

// InstallmentState is one installment with its settled amount.
type InstallmentState struct {
	InstallmentLine
	Paid   int64  `json:"paid"`
	Status string `json:"status"`
}

// SettleInstallments spreads an applicant's paid total over their own
// installments in sequence order (waterfall), so the status of one
// applicant's installments can never be affected by another's payments.
func SettleInstallments(lines []InstallmentLine, paid int64, now time.Time) []InstallmentState {
	ordered := append([]InstallmentLine(nil), lines...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Sequence < ordered[j].Sequence })
	out := make([]InstallmentState, 0, len(ordered))
	remaining := paid
	if remaining < 0 {
		remaining = 0
	}
	for i, l := range ordered {
		settled := l.Amount
		if remaining < settled {
			settled = remaining
		}
		if i == len(ordered)-1 && remaining > settled {
			settled = remaining
		}
		remaining -= settled
		status := "unpaid"
		switch {
		case settled >= l.Amount:
			status = "paid"
		case settled > 0:
			status = "partial"
		case l.DueAt.Before(now):
			status = "overdue"
		}
		if status == "partial" && l.DueAt.Before(now) {
			status = "overdue"
		}
		out = append(out, InstallmentState{InstallmentLine: l, Paid: settled, Status: status})
	}
	return out
}

// InstallmentBalance is an installment id with its still-open amount.
type InstallmentBalance struct {
	ID        string
	Remaining int64
}

// AutoAllocate spreads a verified payment over open installments in order.
// Any overpayment is booked on the last installment so the allocation always
// sums to the payment amount.
func AutoAllocate(amount int64, open []InstallmentBalance) ([]Allocation, error) {
	if amount <= 0 {
		return nil, errors.New("payment amount must be positive")
	}
	if len(open) == 0 {
		return nil, errors.New("tagihan tidak memiliki cicilan untuk dialokasikan")
	}
	var out []Allocation
	left := amount
	for i, b := range open {
		take := b.Remaining
		if take < 0 {
			take = 0
		}
		if take > left || i == len(open)-1 {
			take = left
		}
		if take > 0 {
			out = append(out, Allocation{InstallmentID: b.ID, Amount: take})
			left -= take
		}
		if left == 0 {
			break
		}
	}
	return out, nil
}

// ReregistrationComplete is BR-PPDB-04: the configured minimum is paid and
// every required checklist item is done.
func ReregistrationComplete(paid, minimum int64, requiredItemsDone bool) bool {
	return paid >= minimum && requiredItemsDone
}

// ---------------------------------------------------------------------------
// Deadline jobs (BR-PPDB-07 / BR-PPDB-08)
// ---------------------------------------------------------------------------

// ShouldDropUnpaidForm reports whether an application whose form fee is not
// settled must be dropped. The deadline is inclusive (paying at 23:59 is on
// time) and a proof transferred on or before the deadline that is still
// awaiting review keeps the application alive.
func ShouldDropUnpaidForm(now time.Time, deadline *time.Time, settled, pendingOnTime bool) bool {
	if deadline == nil || settled || pendingOnTime {
		return false
	}
	return now.After(*deadline)
}

// ShouldDropReregistration reports whether an accepted applicant missed the
// BR-PPDB-04 minimum by the re-registration deadline.
func ShouldDropReregistration(now time.Time, due *time.Time, paid, minimum int64, pendingOnTime bool) bool {
	if due == nil || paid >= minimum || pendingOnTime {
		return false
	}
	return now.After(*due)
}

// ---------------------------------------------------------------------------
// Quota and waitlist (BUG-031 / BUG-035)
// ---------------------------------------------------------------------------

// ErrQuotaFull is returned when a track has no seat left.
var ErrQuotaFull = errors.New("kuota jalur sudah penuh")

// CheckQuota allows one more application when active < quota. Quota 0 means
// the track accepts nobody.
func CheckQuota(quota, active int) error {
	if active >= quota {
		return ErrQuotaFull
	}
	return nil
}

// VacantSeats is the number of free seats in a track given how many
// applicants currently hold an offer.
func VacantSeats(quota, holding int) int {
	if holding >= quota {
		return 0
	}
	return quota - holding
}

// WaitlistCandidate is a waitlisted application with its ranking position.
type WaitlistCandidate struct {
	ApplicationID string
	Rank          int
	Total         int64
}

// PickWaitlist returns at most seats candidates in rank order, skipping any
// application that already has a pending proposal or was rejected before, so
// no candidate is proposed twice (TC-13-017) and a rejected one is passed over
// for the next (TC-13-016).
func PickWaitlist(candidates []WaitlistCandidate, skip map[string]bool, seats int) []WaitlistCandidate {
	if seats <= 0 {
		return nil
	}
	ordered := append([]WaitlistCandidate(nil), candidates...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Rank != ordered[j].Rank {
			return ordered[i].Rank < ordered[j].Rank
		}
		return ordered[i].ApplicationID < ordered[j].ApplicationID
	})
	var out []WaitlistCandidate
	for _, c := range ordered {
		if skip[c.ApplicationID] {
			continue
		}
		out = append(out, c)
		if len(out) == seats {
			break
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Track eligibility (BUG-110)
// ---------------------------------------------------------------------------

// Eligibility is the typed form of admission_tracks.eligibility_config.
type Eligibility struct {
	MinAgeYears int `json:"min_age_years,omitempty"`
	MaxAgeYears int `json:"max_age_years,omitempty"`
	// AgeReferenceDate (YYYY-MM-DD) is the date age is measured at, e.g.
	// "2027-07-01" for "minimal 6 tahun per 1 Juli".
	AgeReferenceDate string   `json:"age_reference_date,omitempty"`
	WaveIDs          []string `json:"wave_ids,omitempty"`
}

// ParseEligibility decodes and validates a track eligibility config.
func ParseEligibility(raw []byte) (Eligibility, error) {
	var e Eligibility
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return e, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&e); err != nil {
		return Eligibility{}, fmt.Errorf("syarat kelayakan tidak dikenali: %w", err)
	}
	if e.MinAgeYears < 0 || e.MaxAgeYears < 0 {
		return Eligibility{}, errors.New("batas usia tidak boleh negatif")
	}
	if e.MaxAgeYears > 0 && e.MinAgeYears > e.MaxAgeYears {
		return Eligibility{}, errors.New("usia minimal tidak boleh lebih besar dari usia maksimal")
	}
	if e.AgeReferenceDate != "" {
		if _, err := time.Parse("2006-01-02", e.AgeReferenceDate); err != nil {
			return Eligibility{}, errors.New("tanggal acuan usia harus berformat YYYY-MM-DD")
		}
	}
	return e, nil
}

// AgeYears returns completed years between birth and at.
func AgeYears(birth, at time.Time) int {
	years := at.Year() - birth.Year()
	if at.Month() < birth.Month() || (at.Month() == birth.Month() && at.Day() < birth.Day()) {
		years--
	}
	return years
}

// CheckEligibility lists every unmet track requirement in Indonesian.
func (e Eligibility) CheckEligibility(birth *time.Time, waveID string, now time.Time) []string {
	var issues []string
	if len(e.WaveIDs) > 0 && !contains(e.WaveIDs, waveID) {
		issues = append(issues, "jalur ini tidak dibuka pada gelombang yang dipilih")
	}
	if e.MinAgeYears > 0 || e.MaxAgeYears > 0 {
		if birth == nil {
			return append(issues, "tanggal lahir wajib diisi untuk jalur ini")
		}
		ref := now
		if e.AgeReferenceDate != "" {
			ref, _ = time.Parse("2006-01-02", e.AgeReferenceDate)
		}
		age := AgeYears(*birth, ref)
		if e.MinAgeYears > 0 && age < e.MinAgeYears {
			issues = append(issues, fmt.Sprintf("usia calon siswa %d tahun, minimal %d tahun per %s", age, e.MinAgeYears, ref.Format("02-01-2006")))
		}
		if e.MaxAgeYears > 0 && age > e.MaxAgeYears {
			issues = append(issues, fmt.Sprintf("usia calon siswa %d tahun, maksimal %d tahun per %s", age, e.MaxAgeYears, ref.Format("02-01-2006")))
		}
	}
	return issues
}

// ---------------------------------------------------------------------------
// Wave schedule (BUG-098/155)
// ---------------------------------------------------------------------------

// WaveSchedule is the set of dates configured on a wave.
type WaveSchedule struct {
	RegistrationStart time.Time
	RegistrationEnd   time.Time
	PaymentEnd        *time.Time
	TestAt            *time.Time
	AnnouncementAt    *time.Time
	ReregistrationEnd *time.Time
}

// ValidateWaveSchedule checks the ordering of a wave's dates.
// maxPaymentGraceDays bounds how long after registration closes the form fee
// may still be paid (TC-09-036).
func ValidateWaveSchedule(s WaveSchedule, maxPaymentGraceDays int) error {
	if !s.RegistrationEnd.After(s.RegistrationStart) {
		return errors.New("tanggal tutup pendaftaran harus setelah tanggal buka")
	}
	if s.PaymentEnd != nil {
		if s.PaymentEnd.Before(s.RegistrationStart) {
			return errors.New("batas bayar formulir tidak boleh sebelum pendaftaran dibuka")
		}
		limit := s.RegistrationEnd.AddDate(0, 0, maxPaymentGraceDays)
		if s.PaymentEnd.After(limit) {
			return fmt.Errorf("batas bayar formulir paling lambat %d hari setelah pendaftaran ditutup", maxPaymentGraceDays)
		}
	}
	if s.AnnouncementAt != nil && s.AnnouncementAt.Before(s.RegistrationEnd) {
		return errors.New("tanggal pengumuman harus setelah pendaftaran ditutup")
	}
	if s.TestAt != nil && s.AnnouncementAt != nil && s.AnnouncementAt.Before(*s.TestAt) {
		return errors.New("tanggal pengumuman harus setelah tanggal tes")
	}
	if s.ReregistrationEnd != nil {
		if s.AnnouncementAt == nil {
			return errors.New("isi tanggal pengumuman sebelum batas daftar ulang")
		}
		if !s.ReregistrationEnd.After(*s.AnnouncementAt) {
			return errors.New("batas daftar ulang harus setelah tanggal pengumuman")
		}
	}
	return nil
}

// RangesOverlap reports whether [aStart,aEnd] and [bStart,bEnd] intersect.
func RangesOverlap(aStart, aEnd, bStart, bEnd time.Time) bool {
	return !aEnd.Before(bStart) && !bEnd.Before(aStart)
}

// ---------------------------------------------------------------------------
// Uploads (BUG-042/100)
// ---------------------------------------------------------------------------

// AllowedUploadMIMEs is the only set of document types PPDB accepts.
var AllowedUploadMIMEs = []string{"application/pdf", "image/jpeg", "image/png"}

var extensionMIME = map[string]string{
	".pdf":  "application/pdf",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
}

// ErrUploadType is returned for any file outside the whitelist or whose
// content does not match its extension.
var ErrUploadType = errors.New("tipe file tidak didukung; hanya PDF, JPG, atau PNG yang isinya sesuai")

// DetectUpload validates an upload by extension whitelist AND content sniffing
// (magic bytes) and returns the trusted MIME type. The client Content-Type is
// never trusted: an .exe renamed to .pdf is rejected because its content does
// not start with %PDF-.
func DetectUpload(fileName string, head []byte) (string, error) {
	want, ok := extensionMIME[strings.ToLower(filepath.Ext(fileName))]
	if !ok {
		return "", ErrUploadType
	}
	if len(head) >= 2 && head[0] == 'M' && head[1] == 'Z' {
		return "", ErrUploadType
	}
	got := http.DetectContentType(head)
	if i := strings.IndexByte(got, ';'); i >= 0 {
		got = got[:i]
	}
	if got != want {
		return "", ErrUploadType
	}
	return want, nil
}

// ValidateAllowedMIMEs restricts a requirement's accepted types to the
// whitelist (TC-09-034).
func ValidateAllowedMIMEs(mimes []string) error {
	if len(mimes) == 0 {
		return errors.New("pilih minimal satu tipe file (PDF, JPG, PNG)")
	}
	for _, m := range mimes {
		if !contains(AllowedUploadMIMEs, m) {
			return fmt.Errorf("tipe file %q tidak didukung; hanya PDF, JPG, PNG", m)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Personal data (BUG-103/121)
// ---------------------------------------------------------------------------

// ValidateNIK enforces the 16-digit Indonesian NIK format. Empty is allowed
// (the field is optional at draft time).
func ValidateNIK(nik string) error {
	if nik == "" {
		return nil
	}
	if len(nik) != 16 {
		return errors.New("NIK harus 16 digit angka")
	}
	for _, r := range nik {
		if r < '0' || r > '9' {
			return errors.New("NIK harus 16 digit angka")
		}
	}
	return nil
}

// MaskNIK keeps only the last four digits visible.
func MaskNIK(nik string) string {
	if len(nik) <= 4 {
		return strings.Repeat("*", len(nik))
	}
	return strings.Repeat("*", len(nik)-4) + nik[len(nik)-4:]
}

// ---------------------------------------------------------------------------
// Selection scores (BUG-106)
// ---------------------------------------------------------------------------

// ValidateScore enforces 0 <= score <= max.
func ValidateScore(score, max int64) error {
	if score < 0 || score > max {
		return fmt.Errorf("nilai harus di antara 0 dan %d", max)
	}
	return nil
}

// DocumentIssue explains why one required document blocks the test stage, or
// returns "" when its latest review is "accepted" (BUG-033).
func DocumentIssue(name string, uploaded bool, latestReview string) string {
	switch {
	case !uploaded:
		return "Dokumen wajib belum diunggah: " + name + "."
	case latestReview == "accepted":
		return ""
	case latestReview == "rejected":
		return "Dokumen wajib ditolak: " + name + "."
	case latestReview == "correction":
		return "Dokumen wajib perlu perbaikan: " + name + "."
	default:
		return "Dokumen wajib belum diverifikasi: " + name + "."
	}
}

// ValidateFeeScheme checks a PPDB fee scheme: non-negative amounts, 1..24
// installments and a BR-PPDB-04 minimum of 1..100 percent.
func ValidateFeeScheme(registration, reregistration, entrance int64, installments, minPct int) error {
	switch {
	case registration < 0 || reregistration < 0 || entrance < 0:
		return errors.New("nominal biaya tidak boleh negatif")
	case installments < 1 || installments > 24:
		return errors.New("jumlah cicilan harus antara 1 dan 24")
	case minPct < 1 || minPct > 100:
		return errors.New("minimum pembayaran daftar ulang harus antara 1 dan 100 persen")
	}
	return nil
}

// PaymentStatusOf summarises an application's PPDB bills for list filters
// (BUG-157): unbilled, unpaid, partial or paid.
func PaymentStatusOf(billed, outstanding int64) string {
	switch {
	case billed <= 0:
		return "unbilled"
	case outstanding <= 0:
		return "paid"
	case outstanding >= billed:
		return "unpaid"
	default:
		return "partial"
	}
}

// DocumentUploadAllowed decides whether a guardian may (re)upload a document:
// freely while the application is a draft; after submission only to replace
// a file the committee rejected or asked to correct, and never once locked.
func DocumentUploadAllowed(applicationStatus string, exists bool, latestReview string, locked bool) bool {
	if locked {
		return false
	}
	switch ApplicationStatus(applicationStatus) {
	case StatusDraft:
		return true
	case StatusSubmitted, StatusDocumentReview:
		return exists && (latestReview == "correction" || latestReview == "rejected")
	}
	return false
}

// DocumentReviewMessage is the guardian notification for a document review.
func DocumentReviewMessage(requirement, status, note string) (string, string) {
	switch status {
	case "accepted":
		return "Dokumen diterima", "Dokumen " + requirement + " telah diverifikasi dan diterima."
	case "correction":
		return "Dokumen perlu diperbaiki", "Dokumen " + requirement + " perlu diperbaiki: " + note + ". Silakan unggah ulang melalui portal PPDB."
	default:
		return "Dokumen ditolak", "Dokumen " + requirement + " ditolak: " + note + ". Silakan unggah ulang dokumen yang sesuai."
	}
}

// SafeFileName returns a header-safe download name. Quotes, separators and
// control characters are replaced; an empty name falls back to "dokumen"
// with the extension of the stored type.
func SafeFileName(name, mime string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r < 0x20 || r == 0x7f || r == '"' || r == '\\' || r == '/' || r == ';' || r > 0x7e:
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" || strings.Trim(out, "._") == "" {
		ext := ".bin"
		for e, m := range extensionMIME {
			if m == mime && e != ".jpeg" {
				ext = e
			}
		}
		return "dokumen" + ext
	}
	return out
}

// ReregistrationDeadline is when an offered applicant must have paid the
// re-registration minimum (BR-PPDB-08). Applicants promoted from the
// waitlist get waitlistDays from the promotion; announced offers use the
// wave's re-registration end, or waitlistDays from now when that is unset or
// already past. waitlistDays <= 0 falls back to 7.
func ReregistrationDeadline(now time.Time, waveEnd *time.Time, waitlistDays int, fromWaitlist bool) time.Time {
	if waitlistDays <= 0 {
		waitlistDays = 7
	}
	relative := now.AddDate(0, 0, waitlistDays)
	if fromWaitlist || waveEnd == nil || !waveEnd.After(now) {
		return relative
	}
	return *waveEnd
}
