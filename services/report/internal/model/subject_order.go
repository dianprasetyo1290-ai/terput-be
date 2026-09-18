package model

import (
	"sort"
	"strings"
)

// curriculumOrder is the baku urutan mata pelajaran the rapor must follow
// (SEKOCI V10 item A: "Pendidikan Agama harus berada di paling atas. Mapel
// lain disusun sesuai kelompoknya ... jika ada urutan baku, mohon
// disertakan daftar urutannya"). The list is the Kurikulum Merdeka SMK
// structure from Kepmendikbudristek 262/M/2022 -- Kelompok Umum in its
// decreed order, then Kelompok Kejuruan in its decreed order.
//
// Confirmed against the client's own workbook, Format Rapor (3).xlsx
// (2026-09-17): its Kelompok Umum reads Pendidikan Agama dan Budi Pekerti,
// Pendidikan Pancasila, Bahasa Indonesia; its Kelompok Kejuruan reads
// Matematika, Bahasa Inggris, Konsentrasi Keahlian <...>, "Kreativitas,
// Inovasi dan Kewirausahaan", then a "Mata Pelajaran Pilihan" sub-list
// (Bahasa Jepang). TestSortSubjectsCurriculumTemplateGPM pins that exact
// sequence. Mata Pelajaran Pilihan has no entry of its own below because
// its subjects are named freely per school -- they are unmatched, so they
// rank last inside Kelompok Kejuruan, which is where the template puts
// them.
//
// Entries are matched as case-insensitive substrings of the subject name so
// the real-world spellings a school actually types ("Pendidikan Agama Islam
// dan Budi Pekerti", "PJOK", "Projek IPAS", "Dasar-dasar Animasi") all land
// on the right rank without an admin having to rename anything. First match
// wins, so more specific entries are listed before the ones they'd
// otherwise collide with.
//
// Anything unmatched ranks last and keeps its existing alphabetical order --
// a non-SMK school, a muatan lokal, or a mapel pilihan degrades to today's
// behavior rather than having a wrong order imposed on it.
var curriculumOrder = []string{
	// A. Kelompok Mata Pelajaran Umum
	"pendidikan agama",
	"pendidikan pancasila",
	"pendidikan kewarganegaraan",
	"ppkn",
	"bahasa indonesia",
	"pendidikan jasmani",
	"pjok",
	"sejarah",
	"seni budaya",
	// B. Kelompok Mata Pelajaran Kejuruan
	"matematika",
	"bahasa inggris",
	"informatika",
	"projek ipas",
	"ilmu pengetahuan alam dan sosial",
	"dasar-dasar",
	"dasar dasar",
	"konsentrasi keahlian",
	"projek kreatif",
	"kreativitas",
	"kewirausahaan",
	"praktik kerja lapangan",
	"mata pelajaran pilihan",
}

func curriculumRank(s ReportCardSubject) int {
	if s.SubjectName == nil {
		return len(curriculumOrder)
	}
	name := strings.ToLower(*s.SubjectName)
	for i, key := range curriculumOrder {
		if strings.Contains(name, key) {
			return i
		}
	}
	return len(curriculumOrder)
}

// SortSubjectsCurriculum reorders subject rows into curriculumOrder,
// in place. It is a STABLE sort applied on top of the alphabetical order
// Store.ListSubjects already returns (`ORDER BY subject_name`), so subjects
// sharing a rank -- every unlisted one included -- stay alphabetical among
// themselves. Called from the store so every reader (report card detail,
// print view, leger, deskripsi rapor) gets the same order.
func SortSubjectsCurriculum(subjects []ReportCardSubject) {
	sort.SliceStable(subjects, func(i, j int) bool {
		return curriculumRank(subjects[i]) < curriculumRank(subjects[j])
	})
}
