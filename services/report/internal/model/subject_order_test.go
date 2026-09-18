package model

import "testing"

func subjects(names ...string) []ReportCardSubject {
	out := make([]ReportCardSubject, 0, len(names))
	for _, n := range names {
		name := n
		out = append(out, ReportCardSubject{SubjectName: &name})
	}
	return out
}

func names(s []ReportCardSubject) []string {
	out := make([]string, 0, len(s))
	for i := range s {
		out = append(out, *s[i].SubjectName)
	}
	return out
}

func equal(a, b []string) bool {
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

func TestSortSubjectsCurriculum(t *testing.T) {
	// Input order is what Store.ListSubjects returns today: alphabetical.
	// This is the exact Kelompok Umum list from the client's screenshot.
	got := subjects(
		"Bahasa Indonesia",
		"PJOK",
		"Pendidikan Agama Islam",
		"Pendidikan Pancasila",
		"Sejarah",
		"Seni Budaya",
	)
	SortSubjectsCurriculum(got)
	want := []string{
		"Pendidikan Agama Islam",
		"Pendidikan Pancasila",
		"Bahasa Indonesia",
		"PJOK",
		"Sejarah",
		"Seni Budaya",
	}
	for i, w := range want {
		if names(got)[i] != w {
			t.Fatalf("umum: position %d = %q, want %q (full: %v)", i, names(got)[i], w, names(got))
		}
	}
}

func TestSortSubjectsCurriculumKejuruan(t *testing.T) {
	got := subjects(
		"Bahasa Inggris",
		"Bahasa Jepang",
		"Dasar-dasar Animasi",
		"Informatika",
		"Koding AI",
		"Matematika",
		"Projek IPAS",
	)
	SortSubjectsCurriculum(got)
	want := []string{
		"Matematika",
		"Bahasa Inggris",
		"Informatika",
		"Projek IPAS",
		"Dasar-dasar Animasi",
		// Unlisted subjects rank last, alphabetical among themselves.
		"Bahasa Jepang",
		"Koding AI",
	}
	for i, w := range want {
		if names(got)[i] != w {
			t.Fatalf("kejuruan: position %d = %q, want %q (full: %v)", i, names(got)[i], w, names(got))
		}
	}
}

// A school with no recognized subject at all must come out exactly as it
// went in -- no order imposed where we don't know one.
func TestSortSubjectsCurriculumLeavesUnknownAlone(t *testing.T) {
	got := subjects("Bahasa Sunda", "Tahfidz", "Zoologi")
	SortSubjectsCurriculum(got)
	for i, w := range []string{"Bahasa Sunda", "Tahfidz", "Zoologi"} {
		if names(got)[i] != w {
			t.Fatalf("unknown: position %d = %q, want %q", i, names(got)[i], w)
		}
	}
}

func TestSortSubjectsCurriculumNilName(t *testing.T) {
	got := []ReportCardSubject{{}, {}}
	SortSubjectsCurriculum(got) // must not panic
}

// TestSortSubjectsCurriculumTemplateGPM is the client's own reference
// workbook (Format Rapor (3).xlsx) reproduced verbatim: the exact subject
// names, fed in the alphabetical order Store.ListSubjects returns them,
// must come back out in the template's printed order.
func TestSortSubjectsCurriculumTemplateGPM(t *testing.T) {
	// Kelompok Umum, alphabetical as the DB returns it.
	umum := subjects("Bahasa Indonesia", "Pendidikan Agama dan Budi Pekerti", "Pendidikan Pancasila")
	SortSubjectsCurriculum(umum)
	wantUmum := []string{"Pendidikan Agama dan Budi Pekerti", "Pendidikan Pancasila", "Bahasa Indonesia"}
	if got := names(umum); !equal(got, wantUmum) {
		t.Errorf("Kelompok Umum:\ngot  %v\nwant %v", got, wantUmum)
	}

	// Kelompok Kejuruan, including the Mata Pelajaran Pilihan row that has
	// no curriculum rank and must therefore land last.
	kejuruan := subjects(
		"Bahasa Inggris",
		"Bahasa Jepang",
		"Konsentrasi Keahlian Desain Komunikasi Visual",
		"Kreativitas, Inovasi dan Kewirausahaan",
		"Matematika",
	)
	SortSubjectsCurriculum(kejuruan)
	wantKejuruan := []string{
		"Matematika",
		"Bahasa Inggris",
		"Konsentrasi Keahlian Desain Komunikasi Visual",
		"Kreativitas, Inovasi dan Kewirausahaan",
		"Bahasa Jepang",
	}
	if got := names(kejuruan); !equal(got, wantKejuruan) {
		t.Errorf("Kelompok Kejuruan:\ngot  %v\nwant %v", got, wantKejuruan)
	}
}
