package servicedesk

import (
	"errors"
	"testing"
	"time"
)

var jkt = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		panic(err)
	}
	return loc
}()

// weekdayCal: Senin–Jumat 07:00–15:00 (vektor AC-138-01).
func weekdayCal(holidays ...string) Calendar {
	c := Calendar{Loc: jkt, Hours: map[time.Weekday]Window{}, Holidays: map[string]bool{}}
	for d := 1; d <= 5; d++ {
		c.Hours[ISOWeekday(d)] = Window{Open: 7 * 60, Close: 15 * 60}
	}
	for _, h := range holidays {
		c.Holidays[h] = true
	}
	return c
}

// 2026-10-12 adalah Senin.
func wib(day, hour, minute int) time.Time { return time.Date(2026, 10, day, hour, minute, 0, 0, jkt) }

func TestDueAt(t *testing.T) {
	cases := []struct {
		name  string
		start time.Time
		value int
		unit  string
		cal   Calendar
		want  time.Time
	}{
		{"senin 10:00 + 2 hari kerja", wib(12, 10, 0), 2, UnitBusinessDays, weekdayCal(), wib(14, 10, 0)},
		{"jumat 14:00 + 2 hari kerja", wib(16, 14, 0), 2, UnitBusinessDays, weekdayCal(), wib(20, 14, 0)},
		{"sabtu mulai senin 07:00", wib(17, 9, 30), 2, UnitBusinessDays, weekdayCal(), wib(21, 7, 0)},
		{"libur selasa digeser 1 hari", wib(12, 10, 0), 2, UnitBusinessDays, weekdayCal("2026-10-13"), wib(15, 10, 0)},
		{"sebelum jam buka", wib(12, 5, 0), 1, UnitBusinessDays, weekdayCal(), wib(13, 7, 0)},
		{"setelah jam tutup", wib(12, 16, 0), 1, UnitBusinessDays, weekdayCal(), wib(14, 7, 0)},
		{"4 jam kerja dari 13:00", wib(12, 13, 0), 4, UnitBusinessHours, weekdayCal(), wib(13, 9, 0)},
		{"4 jam kerja jumat 13:00 lewat akhir pekan", wib(16, 13, 0), 4, UnitBusinessHours, weekdayCal(), wib(19, 9, 0)},
		{"jam kerja di hari libur", wib(13, 8, 0), 2, UnitBusinessHours, weekdayCal("2026-10-13"), wib(14, 9, 0)},
		{"zona waktu: masukan UTC", wib(12, 10, 0).UTC(), 2, UnitBusinessDays, weekdayCal(), wib(14, 10, 0)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DueAt(tc.start, tc.value, tc.unit, tc.cal)
			if err != nil {
				t.Fatal(err)
			}
			if !got.Equal(tc.want) {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
	if _, err := DueAt(wib(12, 10, 0), 1, UnitBusinessDays, Calendar{Loc: jkt}); !errors.Is(err, ErrNoBusinessHours) {
		t.Fatalf("calendar kosong: err = %v", err)
	}
	if _, err := DueAt(wib(12, 10, 0), 1, "minutes", weekdayCal()); err == nil {
		t.Fatal("unit tidak dikenal harus galat")
	}
}

func TestSLAState(t *testing.T) {
	cal := weekdayCal()
	start := wib(12, 10, 0)
	due := wib(14, 10, 0) // 16 jam kerja
	cases := []struct {
		name string
		now  time.Time
		want string
	}{
		{"baru dibuat", wib(12, 11, 0), SLAOk},
		{"rabu 06:00 = 13 jam kerja (81%)", wib(14, 6, 0), SLAWarning}, // malam tidak dihitung
		{"tepat jatuh tempo", due, SLAWarning},
		{"2 hari kerja + 1 menit (AC-138-01)", due.Add(time.Minute), SLABreached},
		{"selasa 14:00 = 12 jam kerja (75%)", wib(13, 14, 0), SLAOk},
		{"di bawah ambang", wib(13, 9, 0), SLAOk},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SLAState(due, start, tc.now, 8000, cal); got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}

func TestBusinessDuration(t *testing.T) {
	cal := weekdayCal()
	if got := BusinessDuration(wib(16, 14, 0), wib(19, 8, 0), cal); got != 2*time.Hour {
		t.Fatalf("jumat 14:00 → senin 08:00 = %s", got)
	}
	if got := BusinessDuration(wib(19, 8, 0), wib(16, 14, 0), cal); got != 0 {
		t.Fatalf("terbalik harus 0, got %s", got)
	}
}

func TestValidTransition(t *testing.T) {
	ok := [][2]string{
		{StatusNew, StatusAssigned}, {StatusAssigned, StatusInProgress}, {StatusInProgress, StatusResolved},
		{StatusResolved, StatusClosed}, {StatusResolved, StatusInProgress},
	}
	for _, c := range ok {
		if err := ValidTransition(c[0], c[1], ""); err != nil {
			t.Errorf("%s→%s: %v", c[0], c[1], err)
		}
	}
	for _, from := range []string{StatusNew, StatusAssigned, StatusInProgress} {
		if err := ValidTransition(from, StatusRejected, ""); !errors.Is(err, ErrNoteRequired) {
			t.Errorf("%s→rejected tanpa catatan: %v", from, err)
		}
		if err := ValidTransition(from, StatusRejected, "duplikat"); err != nil {
			t.Errorf("%s→rejected dengan catatan: %v", from, err)
		}
	}
	bad := [][2]string{
		{StatusNew, StatusResolved}, {StatusClosed, StatusInProgress}, {StatusRejected, StatusNew},
		{StatusResolved, StatusRejected}, {StatusAssigned, StatusNew},
	}
	for _, c := range bad {
		if err := ValidTransition(c[0], c[1], "x"); !errors.Is(err, ErrInvalidTransition) {
			t.Errorf("%s→%s harus ditolak, got %v", c[0], c[1], err)
		}
	}
}

func TestResolvePolicy(t *testing.T) {
	cat := "cat-1"
	end := "2026-06-30"
	policies := []Policy{
		{ID: "general", Priority: "normal", EffectiveFrom: "2026-01-01"},
		{ID: "cat-old", CategoryID: &cat, Priority: "normal", EffectiveFrom: "2026-01-01", EffectiveTo: &end},
		{ID: "cat-new", CategoryID: &cat, Priority: "normal", EffectiveFrom: "2026-07-01"},
		{ID: "cat-high", CategoryID: &cat, Priority: "high", EffectiveFrom: "2026-01-01"},
	}
	cases := []struct {
		cat, prio, date, want string
	}{
		{cat, "normal", "2026-03-01", "cat-old"},
		{cat, "normal", "2026-10-12", "cat-new"},
		{"other", "normal", "2026-10-12", "general"},
		{cat, "high", "2026-10-12", "cat-high"},
		{"other", "urgent", "2026-10-12", ""},
		{"other", "normal", "2025-12-31", ""},
	}
	for _, tc := range cases {
		got := ResolvePolicy(policies, tc.cat, tc.prio, tc.date)
		id := ""
		if got != nil {
			id = got.ID
		}
		if id != tc.want {
			t.Errorf("%s/%s/%s: got %q want %q", tc.cat, tc.prio, tc.date, id, tc.want)
		}
	}
}

func TestNextEscalations(t *testing.T) {
	rules := []EscalationRule{
		{Trigger: TriggerWarning, Level: 1, TargetKind: TargetTeamManager},
		{Trigger: TriggerWarning, Level: 2, TargetKind: TargetTeamManager},
		{Trigger: TriggerBreach, Level: 4, TargetKind: TargetRole},
		{Trigger: TriggerBreach, Level: 3, TargetKind: TargetTeamManager},
	}
	levels := func(rs []EscalationRule) []int {
		var out []int
		for _, r := range rs {
			out = append(out, r.Level)
		}
		return out
	}
	eq := func(a, b []int) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	cases := []struct {
		name  string
		level int
		state string
		want  []int
	}{
		{"warning pertama", 0, SLAWarning, []int{1}},
		{"warning berikutnya", 1, SLAWarning, []int{2}},
		{"warning habis", 2, SLAWarning, nil},
		{"breach semua yang tertunda, urut", 0, SLABreached, []int{3, 4}},
		{"breach sudah dijalankan", 4, SLABreached, nil},
		{"ok tidak ada", 0, SLAOk, nil},
	}
	for _, tc := range cases {
		if got := levels(NextEscalations(tc.level, tc.state, rules)); !eq(got, tc.want) {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
	if got := NextEscalations(0, SLABreached, nil); len(got) != 0 {
		t.Errorf("tanpa aturan harus kosong, got %v", got)
	}
}

func TestValidateForm(t *testing.T) {
	schema := []FormField{
		{Key: "nis", Label: "NIS", Type: "text", Required: true},
		{Key: "jumlah", Label: "Jumlah", Type: "number"},
		{Key: "tanggal", Label: "Tanggal", Type: "date"},
		{Key: "jenis", Label: "Jenis", Type: "select", Options: []string{"SPP", "Seragam"}},
	}
	errs := ValidateForm(schema, map[string]any{"jumlah": "abc", "tanggal": "12/10/2026", "jenis": "Buku"})
	for _, k := range []string{"nis", "jumlah", "tanggal", "jenis"} {
		if errs[k] == "" {
			t.Errorf("field %s harus galat", k)
		}
	}
	errs = ValidateForm(schema, map[string]any{"nis": "123", "jumlah": 5.0, "tanggal": "2026-10-12", "jenis": "SPP"})
	if len(errs) != 0 {
		t.Errorf("jawaban valid ditolak: %v", errs)
	}
	if errs := ValidateForm(schema, map[string]any{"nis": "1", "jumlah": "12.5"}); len(errs) != 0 {
		t.Errorf("angka dalam string harus diterima: %v", errs)
	}
}

func TestValidateSchema(t *testing.T) {
	if err := ValidateSchema([]FormField{{Key: "a", Label: "A", Type: "select"}}); err == nil {
		t.Error("select tanpa opsi harus ditolak")
	}
	if err := ValidateSchema([]FormField{{Key: "a", Label: "A", Type: "file"}}); err == nil {
		t.Error("tipe tidak dikenal harus ditolak")
	}
	if err := ValidateSchema([]FormField{{Key: "a", Label: "A", Type: "text"}, {Key: "a", Label: "B", Type: "text"}}); err == nil {
		t.Error("key ganda harus ditolak")
	}
	if err := ValidateSchema([]FormField{{Key: "a", Label: "A", Type: "text"}}); err != nil {
		t.Error(err)
	}
}

func TestCanReport(t *testing.T) {
	cat := Category{Active: true, ReporterKinds: []string{KindParent, KindStudent}}
	if CanReport(cat, KindEmployee) {
		t.Error("kategori tanpa pegawai harus menolak pegawai")
	}
	if !CanReport(cat, KindParent) {
		t.Error("orang tua harus diterima")
	}
	cat.Active = false
	if CanReport(cat, KindParent) {
		t.Error("kategori nonaktif menolak")
	}
	if ReporterKindForRole("guru") != KindEmployee || ReporterKindForRole("orang_tua") != KindParent {
		t.Error("pemetaan peran ke jenis pelapor salah")
	}
}

func TestToServiceRequestStatus(t *testing.T) {
	want := map[string]string{
		StatusNew: "submitted", StatusAssigned: "accepted", StatusInProgress: "in_progress",
		StatusResolved: "done", StatusClosed: "done", StatusRejected: "rejected",
	}
	for in, out := range want {
		if got := ToServiceRequestStatus(in); got != out {
			t.Errorf("%s → %s, want %s", in, got, out)
		}
	}
}

func TestCanRate(t *testing.T) {
	now := time.Now()
	base := Ticket{ReporterUserID: "u1", Status: StatusResolved}
	if err := CanRate(base, "u1"); err != nil {
		t.Error(err)
	}
	if err := CanRate(base, "u2"); err == nil {
		t.Error("bukan pelapor harus ditolak")
	}
	open := base
	open.Status = StatusInProgress
	if err := CanRate(open, "u1"); err == nil {
		t.Error("belum selesai harus ditolak")
	}
	rated := base
	rated.RatedAt = &now
	if err := CanRate(rated, "u1"); err == nil {
		t.Error("sudah dinilai harus ditolak")
	}
	closed := base
	closed.Status = StatusClosed
	if err := CanRate(closed, "u1"); err != nil {
		t.Error(err)
	}
}

func TestComplianceBpAndNumbers(t *testing.T) {
	if v := ComplianceBp(8, 10); v == nil || *v != 8000 {
		t.Errorf("8/10 = %v", v)
	}
	if v := ComplianceBp(2, 3); v == nil || *v != 6666 {
		t.Errorf("2/3 = %v", v)
	}
	if ComplianceBp(0, 0) != nil {
		t.Error("total 0 harus nil")
	}
	if got := TicketNumber(2026, 7); got != "SD-2026-000007" {
		t.Errorf("nomor tiket = %s", got)
	}
	for in, out := range map[string]string{"low": "rendah", "normal": "sedang", "high": "tinggi", "urgent": "darurat"} {
		if WorkOrderPriority(in) != out {
			t.Errorf("prioritas %s", in)
		}
	}
}
