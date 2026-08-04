package store

import (
	"skci/evaluation/internal/model"
)

// Default evaluator-category forms and their scoring items, sourced from the
// client-provided reference spreadsheet:
//   "RAPOT KINERJA GURU SMK 2025-2026 (2).xlsx"
// Sheets: A-Kepala Sekolah, B-Waka Kurikulum, C-Waka Kesiswaan, D-Teman
// Sejawat, E-Penilaian Murid, F-Self-Assessment, plus a RINGKASAN (summary)
// sheet giving the per-category final-score weights:
//   F self=25%, E siswa=22%, A kepala_sekolah=20%, B waka_kurikulum=18%,
//   C waka_kesiswaan=10%, D peer=5%  (sums to 100%)
//
// Item-level "weight" below is that item's share of the FINAL 0-4 score
// within its own form (renormalized at scoring time by the sum of weights
// of items actually answered in a task) -- it is informational granularity
// for the rubric, not summed directly into the final score. The final score
// rollup uses the FORM-level weight only (see internal/handler/results.go).

type defaultItem struct {
	Code        string
	Indicator   string
	IndicatorEn string
	Weight      float64
	ScoreLabels map[string]string
}

type defaultForm struct {
	Code          string
	Title         string
	EvaluatorType string
	Priority      int
	Weight        float64
	Items         []defaultItem
}

var DefaultForms = []defaultForm{
	{
		Code: "F", Title: "Self-Assessment — Guru Sendiri", EvaluatorType: "self", Priority: 1, Weight: 0.25,
		Items: []defaultItem{
			{"F-1", "Kualitas dan dampak pembelajaran terhadap siswa", "Quality and learning impact on students", 0.05, map[string]string{"4": "Bukti konkret siswa tumbuh secara akademik & karakter; data capaian tersedia", "3": "Sebagian besar siswa menunjukkan kemajuan; ada beberapa bukti", "2": "Ada kemajuan tapi tidak konsisten; bukti terbatas", "1": "Tidak ada bukti kemajuan siswa yang signifikan"}},
			{"F-2", "Konsistensi persiapan dan pelaksanaan mengajar", "Consistency of teaching preparation and delivery", 0.04, map[string]string{"4": "Selalu terencana, RPP lengkap sebelum kelas; zero pengingat dari atasan", "3": "Umumnya terencana; sesekali membutuhkan pengingat", "2": "Sering butuh pengingat; persiapan kurang konsisten", "1": "Jarang terencana; RPP sering tidak lengkap"}},
			{"F-3", "Inovasi pembelajaran yang diterapkan semester ini", "Learning innovations implemented this semester", 0.04, map[string]string{"4": "2-3+ inovasi nyata diterapkan; dampak terukur pada keterlibatan siswa", "3": "1-2 inovasi dicoba; ada dampak positif", "2": "Sesekali berinovasi; hasil kurang konsisten", "1": "Tidak ada inovasi; metode monoton"}},
			{"F-4", "Kontribusi terhadap budaya dan komunitas sekolah", "Contribution to school culture and community", 0.04, map[string]string{"4": "Aktif secara sukarela di berbagai kegiatan sekolah di luar kelas", "3": "Berkontribusi pada beberapa kegiatan di luar kelas", "2": "Sesekali berkontribusi; sebagian besar hanya kegiatan wajib", "1": "Tidak ada kontribusi di luar kewajiban formal"}},
			{"F-5", "Rencana pengembangan diri ke depan", "Personal development plan going forward", 0.03, map[string]string{"4": "Rencana SMART dan konkret; langkah jelas, realistis, dan ada timeline", "3": "Rencana ada; cukup jelas namun perlu detail lebih", "2": "Rencana umum; tanpa langkah konkret", "1": "Tidak ada rencana pengembangan diri"}},
			{"F-6", "Kualitas umpan balik (feedback) yang diberikan kepada siswa", "Quality of feedback given to students", 0.03, map[string]string{"4": "Feedback spesifik, konstruktif, tepat waktu, dan dibuktikan dengan respon siswa", "3": "Feedback umumnya jelas dan tepat waktu", "2": "Feedback ada tapi umum/generik; tidak selalu tepat waktu", "1": "Jarang atau tidak memberikan feedback bermakna kepada siswa"}},
			{"F-7", "Keseimbangan beban kerja dan kesejahteraan diri (wellbeing)", "Work-life balance and personal wellbeing", 0.02, map[string]string{"4": "Mampu mengelola beban kerja dengan sehat; tidak burn out; produktif dan positif", "3": "Umumnya mampu menyeimbangkan; sesekali kewalahan namun bisa pulih", "2": "Sering merasa kewalahan; mulai menunjukkan tanda kelelahan", "1": "Mengalami burn out yang berdampak negatif pada kinerja"}},
		},
	},
	{
		Code: "E", Title: "Penilaian Murid — Student Assessment", EvaluatorType: "siswa", Priority: 2, Weight: 0.22,
		Items: []defaultItem{
			{"E-1", "Guru yang Helpful – mudah didekati dan responsif terhadap kebutuhan siswa", "Helpful Teacher – approachable and responsive to student needs", 0.03, map[string]string{"4": "Selalu siap membantu; mudah didekati; responsif kapanpun dibutuhkan", "3": "Umumnya membantu ketika diminta; responsif", "2": "Kadang membantu, kadang tidak; tidak konsisten", "1": "Sulit dihubungi atau jarang membantu siswa"}},
			{"E-2", "Guru yang Memotivasi – menginspirasi dan mendorong potensi terbaik siswa", "Motivating Teacher – inspires and brings out students' best potential", 0.03, map[string]string{"4": "Selalu menginspirasi; mendorong potensi terbaik setiap individu siswa", "3": "Sering memotivasi dengan cara yang efektif", "2": "Sesekali memotivasi; tidak konsisten", "1": "Jarang atau tidak pernah memotivasi siswa"}},
			{"E-3", "Guru yang Berinovasi – menggunakan metode dan media pembelajaran yang menarik", "Innovative Teacher – uses engaging methods and learning media", 0.03, map[string]string{"4": "Selalu menggunakan metode dan media baru yang menarik dan relevan", "3": "Sering berinovasi dalam pembelajaran", "2": "Sesekali berinovasi; lebih sering konvensional", "1": "Metode monoton; tidak pernah berinovasi"}},
			{"E-4", "Guru Pencipta Suasana Berbahasa Inggris di kelas", "English Atmosphere Creator – builds English-speaking classroom", 0.03, map[string]string{"4": "Aktif menciptakan & mempertahankan suasana Bahasa Inggris secara konsisten", "3": "Sering menciptakan suasana berbahasa Inggris", "2": "Kadang menggunakan Bahasa Inggris di kelas", "1": "Tidak menciptakan suasana berbahasa Inggris sama sekali"}},
			{"E-5", "Kejelasan guru dalam menyampaikan materi pelajaran", "Clarity of teacher in delivering lesson material", 0.03, map[string]string{"4": "Materi selalu disampaikan sangat jelas; siswa mudah memahami tanpa kebingungan", "3": "Umumnya jelas; sesekali perlu pengulangan", "2": "Kadang membingungkan; siswa sering perlu bertanya ulang", "1": "Penjelasan tidak jelas; siswa sulit memahami materi"}},
			{"E-6", "Keadilan guru dalam penilaian – objektif dan tidak pilih kasih", "Fairness in assessment – objective and impartial", 0.03, map[string]string{"4": "Selalu menilai secara adil & objektif; kriteria jelas; tidak ada keberpihakan", "3": "Umumnya adil; sesekali ada persepsi ketidakadilan minor", "2": "Kadang terlihat pilih kasih atau penilaian tidak konsisten", "1": "Sering dianggap tidak adil; ada bukti keberpihakan nyata"}},
			{"E-7", "Kemampuan guru mengelola kelas agar kondusif dan produktif", "Teacher's classroom management – conducive and productive atmosphere", 0.02, map[string]string{"4": "Kelas selalu tertib, kondusif, dan produktif; transisi antar aktivitas mulus", "3": "Kelas umumnya kondusif; gangguan minor dapat diatasi dengan baik", "2": "Kelas kadang tidak kondusif; pengelolaan kurang konsisten", "1": "Kelas sering gaduh; guru kesulitan mengelola situasi"}},
			{"E-8", "Guru memberi kesempatan siswa berpendapat, bertanya, dan mengekspresikan ide", "Teacher encourages students to voice opinions, ask questions, and express ideas", 0.02, map[string]string{"4": "Selalu membuka ruang diskusi; setiap siswa merasa aman berpendapat", "3": "Sering memberi kesempatan; siswa cukup aktif", "2": "Sesekali memberi kesempatan; lebih banyak satu arah", "1": "Tidak memberi ruang bagi siswa untuk berpendapat atau bertanya"}},
		},
	},
	{
		Code: "A", Title: "Kepala Sekolah / Principal", EvaluatorType: "kepala_sekolah", Priority: 3, Weight: 0.20,
		Items: []defaultItem{
			{"A-1", "Kehadiran dan ketepatan waktu dalam semua kewajiban sekolah", "Attendance and punctuality in all school obligations", 0.02, map[string]string{"4": "Selalu hadir & tepat waktu; tanpa pengingat; absensi 0%", "3": "Hampir selalu tepat waktu; sesekali terlambat dengan alasan jelas", "2": "Sering terlambat atau absen tanpa keterangan yang memadai", "1": "Kehadiran sangat tidak konsisten; perlu pembinaan segera"}},
			{"A-2", "Kesiapan mengajar tanpa perlu diingatkan atasan", "Teaching readiness without supervisor reminders", 0.02, map[string]string{"4": "Selalu siap mengajar; perangkat lengkap sebelum hari H", "3": "Umumnya siap; sesekali butuh pengingat ringan", "2": "Sering butuh pengingat dari atasan", "1": "Tidak pernah siap tanpa pengingat; perlu pembinaan"}},
			{"A-3", "Respons terhadap arahan dan kebijakan sekolah", "Response to school directives and policies", 0.01, map[string]string{"4": "Merespons cepat; melaksanakan sepenuhnya; proaktif memberi masukan", "3": "Melaksanakan arahan dengan baik dan tepat waktu", "2": "Melaksanakan tapi lambat atau tidak sepenuhnya", "1": "Sering tidak merespons atau menolak arahan"}},
			{"A-4", "Partisipasi aktif dalam rapat dan kegiatan institusional", "Active participation in meetings and institutional activities", 0.01, map[string]string{"4": "Selalu hadir; aktif berkontribusi dan memberikan ide bermakna", "3": "Selalu hadir; partisipasi cukup", "2": "Kadang tidak hadir atau pasif saat rapat", "1": "Sering tidak hadir; tidak berpartisipasi"}},
			{"A-5", "Integritas dan etika profesional di lingkungan sekolah", "Integrity and professional ethics in school environment", 0.01, map[string]string{"4": "Menjadi teladan integritas; tidak pernah ada keluhan etika", "3": "Berperilaku etis secara konsisten", "2": "Ada insiden etika minor yang terdokumentasi", "1": "Ada pelanggaran etika serius; perlu pembinaan segera"}},
			{"A-6", "Menjadi teladan nilai GPM Future Ready di luar kelas", "Modeling GPM Future Ready values outside the classroom", 0.03, map[string]string{"4": "Secara konsisten mencontohkan & mempromosikan nilai GPM dalam keseharian", "3": "Umumnya menjadi teladan nilai GPM", "2": "Kadang-kadang menjadi teladan; tidak konsisten", "1": "Tidak menjadi teladan nilai GPM"}},
			{"A-7", "Mendukung dan memimpin program English atmosphere sekolah", "Supporting and leading school English atmosphere program", 0.02, map[string]string{"4": "Aktif mempraktikkan & mendorong Bahasa Inggris di seluruh area sekolah", "3": "Mendukung program English atmosphere dengan baik", "2": "Sesekali menggunakan Bahasa Inggris di luar kelas", "1": "Tidak mendukung program English atmosphere"}},
			{"A-8", "Berkontribusi pada iklim kerja yang positif dan kolaboratif", "Contributing to a positive and collaborative work climate", 0.02, map[string]string{"4": "Aktif membangun harmoni; mediasi konflik; menginspirasi rekan", "3": "Berkontribusi pada iklim positif secara konsisten", "2": "Kadang berkontribusi; kadang memicu ketegangan", "1": "Sering memicu konflik atau tidak kooperatif"}},
			{"A-9", "Inisiatif di luar tugas pokok untuk kebaikan sekolah", "Initiatives beyond main duties for school's benefit", 0.01, map[string]string{"4": "Sering berinisiatif sendiri tanpa diminta; dampak nyata bagi sekolah", "3": "Sesekali berinisiatif dengan hasil yang baik", "2": "Jarang berinisiatif di luar kewajiban formal", "1": "Tidak pernah berinisiatif di luar tugas pokok"}},
			{"A-10", "Keaktifan mengikuti pelatihan, seminar, atau sertifikasi", "Activeness in attending training, seminars, or certifications", 0.02, map[string]string{"4": "Minimal 2 pelatihan/seminar/sertifikasi per semester; hasil diterapkan di kelas", "3": "Minimal 1 pelatihan per semester; ada upaya penerapan", "2": "Jarang mengikuti pelatihan; penerapan minim", "1": "Tidak pernah mengikuti pelatihan profesional"}},
			{"A-11", "Penerapan hasil pengembangan diri dalam pembelajaran nyata", "Application of professional development outcomes in actual teaching", 0.02, map[string]string{"4": "Selalu menerapkan ilmu baru; ada bukti perubahan praktik mengajar", "3": "Sebagian hasil pelatihan diterapkan di kelas", "2": "Jarang menerapkan; pengetahuan berhenti di teori", "1": "Tidak ada penerapan hasil pelatihan dalam mengajar"}},
		},
	},
	{
		Code: "B", Title: "Waka Kurikulum / Curriculum Vice-Principal", EvaluatorType: "waka_kurikulum", Priority: 4, Weight: 0.18,
		Items: []defaultItem{
			{"B-1", "Kelengkapan perangkat pembelajaran (RPP, Silabus, LO, Rubrik)", "Completeness of teaching documents (Lesson Plan, Syllabus, LO, Rubrics)", 0.03, map[string]string{"4": "100% lengkap, berkualitas, diserahkan tepat waktu sebelum semester dimulai", "3": "Lengkap, diserahkan tepat waktu", "2": "Sebagian besar lengkap; ada keterlambatan minor", "1": "Tidak lengkap; sering terlambat menyerahkan"}},
			{"B-2", "Kualitas dan kesesuaian RPP dengan prinsip OBE", "Lesson Plan quality and alignment with OBE principles", 0.03, map[string]string{"4": "RPP berbasis OBE penuh; LO terukur; rubrik autentik dan digunakan nyata", "3": "RPP cukup berbasis OBE; LO ada dan cukup jelas", "2": "RPP ada tapi kurang mencerminkan OBE", "1": "RPP tidak berbasis OBE; LO tidak terukur"}},
			{"B-3", "Kualitas soal/asesmen dan rubrik penilaian (berbasis OBE)", "Quality of assessments and rubrics (OBE-based)", 0.03, map[string]string{"4": "Soal HOTS; rubrik detail; asesmen otentik & beragam; feedback ke siswa jelas", "3": "Soal cukup HOTS; rubrik ada dan digunakan", "2": "Soal dominan LOTS; rubrik ada tapi kurang detail", "1": "Soal LOTS semua; tanpa rubrik yang bermakna"}},
			{"B-4", "Pelaksanaan supervisi dan observasi kelas (sebagai yang disupervisi)", "Classroom supervision participation (as supervised teacher)", 0.02, map[string]string{"4": "Menyambut supervisi terbuka; menerapkan hasil supervisi secara konsisten", "3": "Berpartisipasi baik; ada tindak lanjut", "2": "Berpartisipasi tapi tindak lanjut minim", "1": "Menolak supervisi atau tidak menindaklanjuti sama sekali"}},
			{"B-5", "Portofolio siswa tersusun sebagai bukti capaian OBE", "Student portfolio compiled as evidence of OBE achievement", 0.03, map[string]string{"4": ">90% siswa memiliki portofolio reflektif dan terstruktur", "3": "70-90% siswa memiliki portofolio yang baik", "2": "50-69% siswa memiliki portofolio", "1": "<50% siswa memiliki portofolio; tidak terstruktur"}},
			{"B-6", "Partisipasi dalam review dan pembaruan kurikulum/Learning Outcome", "Participation in curriculum review and Learning Outcome updates", 0.02, map[string]string{"4": "Aktif berkontribusi; mengusulkan revisi berbasis data capaian siswa", "3": "Berpartisipasi dalam review dan mengikuti revisi yang disepakati", "2": "Hadir dalam review tapi kontribusi minim", "1": "Tidak hadir atau tidak berpartisipasi dalam review"}},
			{"B-7", "Keaktifan dalam Lesson Study dan komunitas belajar guru", "Activeness in Lesson Study and teacher learning community", 0.02, map[string]string{"4": "Menjadi penggerak Lesson Study di mapelnya; mendokumentasi & berbagi hasil", "3": "Berpartisipasi aktif dalam semua siklus Lesson Study", "2": "Berpartisipasi dalam sebagian siklus Lesson Study", "1": "Tidak berpartisipasi dalam Lesson Study"}},
		},
	},
	{
		Code: "C", Title: "Waka Kesiswaan / Student Affairs Vice-Principal", EvaluatorType: "waka_kesiswaan", Priority: 5, Weight: 0.10,
		Items: []defaultItem{
			{"C-1", "Keterlibatan dalam pembinaan karakter dan kedisiplinan siswa", "Involvement in student character development and discipline", 0.02, map[string]string{"4": "Aktif mendukung BK & kesiswaan; menjadi mentor karakter di luar kelas", "3": "Mendukung program kesiswaan dan berkontribusi pada pembinaan karakter", "2": "Terlibat hanya jika diminta atau diwajibkan", "1": "Tidak terlibat dalam pembinaan karakter siswa"}},
			{"C-2", "Responsivitas terhadap laporan masalah siswa (akademik & non-akademik)", "Responsiveness to student problem reports", 0.02, map[string]string{"4": "Selalu merespons cepat & solutif; proaktif berkoordinasi dengan BK dan orang tua", "3": "Merespons masalah siswa dengan baik dan tepat waktu", "2": "Merespons tapi lambat atau tidak tuntas", "1": "Jarang merespons laporan masalah siswa"}},
			{"C-3", "Kehadiran dan kontribusi dalam kegiatan kesiswaan dan ekstrakurikuler", "Presence and contribution in student activities and extracurriculars", 0.01, map[string]string{"4": "Aktif terlibat; sering berinisiatif dalam kegiatan kesiswaan di luar jam mengajar", "3": "Hadir dan berkontribusi dalam kegiatan kesiswaan yang ditetapkan", "2": "Hadir jika diwajibkan tapi kontribusi minimal", "1": "Tidak hadir atau menghindari kegiatan kesiswaan"}},
			{"C-4", "Membangun komunikasi positif dengan orang tua siswa", "Building positive communication with students' parents", 0.01, map[string]string{"4": "Proaktif menjalin komunikasi; selalu memberikan info perkembangan siswa", "3": "Berkomunikasi dengan orang tua ketika diperlukan dengan baik", "2": "Komunikasi minimal; hanya saat ada masalah", "1": "Tidak pernah berkomunikasi dengan orang tua"}},
			{"C-5", "Kepekaan terhadap kesehatan mental dan kesejahteraan siswa", "Sensitivity to student mental health and well-being", 0.01, map[string]string{"4": "Sangat peka; mendeteksi dini masalah siswa; koordinasi segera dengan BK", "3": "Peka terhadap kondisi siswa; melaporkan ke BK bila diperlukan", "2": "Kurang peka; hanya merespons masalah yang sudah jelas terlihat", "1": "Tidak peka terhadap kondisi mental/kesejahteraan siswa"}},
			{"C-6", "Penanganan kasus siswa bermasalah secara restoratif", "Restorative approach in handling student disciplinary cases", 0.01, map[string]string{"4": "Selalu menggunakan pendekatan restoratif; siswa diberdayakan bukan dihukum", "3": "Umumnya restoratif; sesekali punitive tapi bertanggung jawab", "2": "Lebih sering punitive; jarang pendekatan restoratif", "1": "Tidak memiliki pendekatan yang jelas dalam menangani siswa bermasalah"}},
			{"C-7", "Kontribusi pada program anti-bullying dan keamanan siswa", "Contribution to anti-bullying program and student safety", 0.01, map[string]string{"4": "Aktif mengedukasi, mencegah, dan melaporkan kasus bullying; jadi role model", "3": "Mendukung program anti-bullying; merespons kasus dengan baik", "2": "Merespons kasus bullying jika dilaporkan; tidak proaktif", "1": "Tidak peduli atau tidak merespons kasus bullying"}},
			{"C-8", "Dokumentasi perkembangan karakter siswa secara berkala", "Periodic documentation of student character development", 0.01, map[string]string{"4": "Mendokumentasi perkembangan karakter siswa secara sistematis dan berkala", "3": "Ada dokumentasi; cukup teratur dan bisa dijadikan referensi", "2": "Dokumentasi ada tapi tidak teratur atau tidak lengkap", "1": "Tidak ada dokumentasi perkembangan karakter siswa"}},
		},
	},
	// G/H/I: upward-feedback track (guru rates a school leader). NOT part of
	// the 100% blend above (Weight: 0, excluded from computeResults -- see
	// internal/handler/results.go) -- a separate track with its own simple
	// per-leader average. Each of the 3 forms has its OWN 6-item rubric
	// targeting that specific position's actual competencies (kehadiran/
	// vision/decisions for Kepala Sekolah, RPP/curriculum/academic quality
	// for Waka Kurikulum, discipline/extracurricular/student welfare for
	// Waka Kesiswaan) -- previously these 3 forms shared one generic
	// "Upward Feedback" rubric with only the role name swapped in, which
	// read as an obviously templated questionnaire to teachers filling it
	// out for 3 different leaders.
	{
		Code: "G", Title: "Kepala Sekolah (oleh Guru) / Principal (rated by Teacher)", EvaluatorType: model.EvalTypeGuruToKepsek, Priority: 7, Weight: 0,
		Items: kepsekUpwardItems,
	},
	{
		Code: "H", Title: "Waka Kurikulum (oleh Guru) / Curriculum VP (rated by Teacher)", EvaluatorType: model.EvalTypeGuruToWakaKurikulum, Priority: 8, Weight: 0,
		Items: wakaKurikulumUpwardItems,
	},
	{
		Code: "I", Title: "Waka Kesiswaan (oleh Guru) / Student Affairs VP (rated by Teacher)", EvaluatorType: model.EvalTypeGuruToWakaKesiswaan, Priority: 9, Weight: 0,
		Items: wakaKesiswaanUpwardItems,
	},
	{
		Code: "D", Title: "Teman Sejawat / Peer Teachers", EvaluatorType: "peer", Priority: 6, Weight: 0.05,
		Items: []defaultItem{
			{"D-1", "Kesediaan berbagi materi, ide, dan sumber ajar", "Willingness to share materials, ideas and teaching resources", 0.01, map[string]string{"4": "Proaktif berbagi tanpa diminta; menginisiasi sesi berbagi antar guru", "3": "Bersedia berbagi ketika diminta; responsif dan terbuka", "2": "Berbagi hanya jika ada timbal balik; atau jarang berbagi", "1": "Tidak mau berbagi; menyimpan materi untuk diri sendiri"}},
			{"D-2", "Kolaborasi dalam perencanaan dan pelaksanaan program sekolah", "Collaboration in school program planning and implementation", 0.01, map[string]string{"4": "Aktif terlibat; kontribusi nyata; menginisiasi kerja sama lintas mapel", "3": "Terlibat dengan baik dalam program yang ada", "2": "Terlibat hanya jika diwajibkan", "1": "Tidak mau berkolaborasi; menghindari program bersama"}},
			{"D-3", "Keterbukaan menerima masukan dan kritik konstruktif", "Openness to receiving feedback and constructive criticism", 0.01, map[string]string{"4": "Menyambut masukan antusias; langsung menerapkan; berterima kasih tulus", "3": "Menerima masukan dengan baik dan berusaha menindaklanjuti", "2": "Menerima secara verbal tapi jarang menindaklanjuti; atau defensif", "1": "Menolak masukan; sangat defensif"}},
			{"D-4", "Kontribusi pada Lesson Study dan komunitas belajar guru", "Contribution to Lesson Study and teacher learning community", 0.01, map[string]string{"4": "Menjadi penggerak Lesson Study; mengundang pengamat; mendorong refleksi bersama", "3": "Berpartisipasi aktif dalam siklus Lesson Study yang ada", "2": "Hadir tapi kontribusi minim; atau tidak berpartisipasi", "1": "Tidak berpartisipasi sama sekali"}},
			{"D-5", "Menjaga etika dan komunikasi yang sehat antar guru", "Maintaining healthy ethics and communication among teachers", 0.005, map[string]string{"4": "Selalu berkomunikasi hormat, jelas, membangun; menjadi teladan etika", "3": "Berkomunikasi dengan baik; tidak ada keluhan dari rekan", "2": "Kadang komunikasi kurang profesional; atau sering menimbulkan konflik", "1": "Sering berkomunikasi tidak profesional; memicu konflik"}},
			{"D-6", "Mendukung rekan yang membutuhkan bantuan tanpa diminta", "Supporting colleagues who need help without being asked", 0.005, map[string]string{"4": "Aktif menawarkan bantuan; peka terhadap kesulitan rekan tanpa diminta", "3": "Membantu ketika diminta; responsif dan tulus", "2": "Membantu hanya jika ada keuntungan timbal balik; atau tidak peduli", "1": "Tidak mau membantu rekan; acuh tak acuh"}},
		},
	},
}

// Upward-feedback item weights are equal (1/6 each) so a task's raw_score
// (see TaskStore.SubmitAnswers) is a plain average across whichever items
// were answered -- there's no form-level weight to renormalize against
// since these forms carry Weight: 0 (excluded from the blended teacher
// score).
const upwardItemWeight = 1.0 / 6

// kepsekUpwardItems: guru rates the Kepala Sekolah on presence/oversight,
// school-wide leadership & vision, and decision-making -- the competencies
// a teacher actually observes in their principal, not a generic "rate your
// boss" list.
var kepsekUpwardItems = []defaultItem{
	{"G-1", "Kehadiran dan keberadaan Kepala Sekolah di lingkungan sekolah", "Principal's presence and visibility around the school", upwardItemWeight, map[string]string{"4": "Selalu hadir dan mudah ditemui; hadir langsung memantau kegiatan sekolah", "3": "Umumnya hadir; sesekali tidak di tempat dengan alasan jelas", "2": "Sering tidak berada di sekolah tanpa keterangan yang memadai", "1": "Jarang hadir; sulit ditemui ketika dibutuhkan"}},
	{"G-2", "Kejelasan visi dan arah pengembangan sekolah yang dikomunikasikan Kepala Sekolah", "Clarity of school vision and direction communicated by the Principal", upwardItemWeight, map[string]string{"4": "Visi & arah sekolah jelas, dikomunikasikan rutin, dan dipahami seluruh guru", "3": "Visi cukup jelas; dikomunikasikan meski tidak terlalu rutin", "2": "Visi ada tapi jarang dikomunikasikan; guru kurang paham arah sekolah", "1": "Tidak ada visi/arah yang jelas dikomunikasikan ke guru"}},
	{"G-3", "Kualitas pengambilan keputusan Kepala Sekolah dalam situasi penting/mendesak", "Quality of the Principal's decision-making in important or urgent situations", upwardItemWeight, map[string]string{"4": "Keputusan cepat, tepat, dan mempertimbangkan dampak bagi guru & siswa", "3": "Keputusan umumnya tepat meski kadang perlu waktu lebih lama", "2": "Keputusan sering lambat atau kurang mempertimbangkan dampak di lapangan", "1": "Keputusan sering tidak tepat atau tidak diambil sama sekali saat dibutuhkan"}},
	{"G-4", "Pengawasan (oversight) Kepala Sekolah terhadap operasional sekolah sehari-hari", "Principal's oversight of day-to-day school operations", upwardItemWeight, map[string]string{"4": "Memantau operasional secara konsisten; cepat menangkap dan menindaklanjuti masalah", "3": "Cukup memantau operasional; menindaklanjuti sebagian besar masalah", "2": "Pengawasan minim; banyak masalah operasional baru diketahui belakangan", "1": "Tidak ada pengawasan nyata terhadap operasional sekolah"}},
	{"G-5", "Keteladanan dan integritas kepemimpinan Kepala Sekolah", "Role-modeling and leadership integrity of the Principal", upwardItemWeight, map[string]string{"4": "Menjadi teladan nyata dalam etika dan integritas kepemimpinan", "3": "Berperilaku etis dan konsisten sebagai pemimpin", "2": "Ada insiden etika minor yang terdokumentasi", "1": "Ada pelanggaran etika/integritas kepemimpinan yang serius"}},
	{"G-6", "Kemampuan Kepala Sekolah menangani krisis atau masalah besar di sekolah", "Principal's ability to handle school crises or major issues", upwardItemWeight, map[string]string{"4": "Menangani krisis dengan tenang, terkoordinasi, dan solutif", "3": "Menangani krisis dengan cukup baik meski kadang terlambat", "2": "Penanganan krisis kurang terkoordinasi atau kurang tegas", "1": "Krisis/masalah besar dibiarkan tanpa penanganan yang jelas"}},
}

// wakaKurikulumUpwardItems: guru rates the Waka Kurikulum on RPP support &
// review, curriculum guidance, and academic-quality oversight.
var wakaKurikulumUpwardItems = []defaultItem{
	{"H-1", "Dukungan dan review Waka Kurikulum terhadap RPP (rencana pelaksanaan pembelajaran) guru", "Curriculum VP's support and review of teachers' lesson plans (RPP)", upwardItemWeight, map[string]string{"4": "Selalu mereview RPP tepat waktu dengan masukan konstruktif dan spesifik", "3": "Mereview RPP secara rutin dengan masukan yang cukup jelas", "2": "Review RPP jarang atau masukan bersifat umum/tidak spesifik", "1": "Tidak pernah mereview atau memberi masukan terhadap RPP guru"}},
	{"H-2", "Kejelasan panduan kurikulum yang diberikan Waka Kurikulum kepada guru", "Clarity of curriculum guidance given by the Curriculum VP to teachers", upwardItemWeight, map[string]string{"4": "Panduan kurikulum (termasuk prinsip OBE) jelas, terdokumentasi, dan mudah diterapkan", "3": "Panduan cukup jelas meski kadang perlu klarifikasi tambahan", "2": "Panduan ada tapi sering membingungkan atau tidak konsisten", "1": "Tidak ada panduan kurikulum yang jelas dari Waka Kurikulum"}},
	{"H-3", "Pengawasan Waka Kurikulum terhadap kualitas akademik pembelajaran di kelas", "Curriculum VP's oversight of classroom academic quality", upwardItemWeight, map[string]string{"4": "Aktif memantau kualitas akademik (supervisi, hasil belajar) secara berkala", "3": "Cukup memantau kualitas akademik meski tidak terlalu rutin", "2": "Pemantauan kualitas akademik minim atau tidak terjadwal", "1": "Tidak ada pengawasan terhadap kualitas akademik pembelajaran"}},
	{"H-4", "Bantuan teknis Waka Kurikulum dalam implementasi kurikulum/OBE", "Curriculum VP's technical assistance in implementing the curriculum/OBE", upwardItemWeight, map[string]string{"4": "Selalu siap membantu kendala teknis kurikulum dengan solusi konkret", "3": "Membantu ketika diminta; solusi cukup membantu", "2": "Bantuan teknis lambat atau kurang memadai", "1": "Tidak memberikan bantuan teknis terkait kurikulum sama sekali"}},
	{"H-5", "Responsivitas Waka Kurikulum terhadap kendala akademik yang dilaporkan guru", "Curriculum VP's responsiveness to academic issues reported by teachers", upwardItemWeight, map[string]string{"4": "Selalu merespons cepat dan menindaklanjuti kendala akademik guru", "3": "Merespons dengan baik meski kadang perlu waktu", "2": "Respons lambat atau tidak tuntas", "1": "Jarang atau tidak pernah merespons kendala akademik guru"}},
	{"H-6", "Kualitas umpan balik supervisi akademik dari Waka Kurikulum", "Quality of academic supervision feedback from the Curriculum VP", upwardItemWeight, map[string]string{"4": "Umpan balik supervisi spesifik, konstruktif, dan membantu perbaikan nyata", "3": "Umpan balik umumnya jelas dan bermanfaat", "2": "Umpan balik bersifat umum/generik; kurang membantu perbaikan", "1": "Tidak ada umpan balik supervisi akademik yang berarti"}},
}

// wakaKesiswaanUpwardItems: guru rates the Waka Kesiswaan on student-affairs
// & discipline handling, extracurricular support, and student welfare.
var wakaKesiswaanUpwardItems = []defaultItem{
	{"I-1", "Penanganan Waka Kesiswaan terhadap masalah disiplin siswa yang dilaporkan guru", "Student Affairs VP's handling of student discipline issues reported by teachers", upwardItemWeight, map[string]string{"4": "Selalu menindaklanjuti laporan disiplin dengan cepat dan tuntas", "3": "Menindaklanjuti laporan disiplin dengan baik meski kadang perlu waktu", "2": "Penanganan disiplin lambat atau tidak tuntas", "1": "Laporan disiplin siswa jarang atau tidak pernah ditindaklanjuti"}},
	{"I-2", "Dukungan Waka Kesiswaan terhadap kegiatan ekstrakurikuler", "Student Affairs VP's support for extracurricular activities", upwardItemWeight, map[string]string{"4": "Aktif mendukung & memfasilitasi ekstrakurikuler; responsif terhadap kebutuhan pembina", "3": "Mendukung kegiatan ekstrakurikuler dengan cukup baik", "2": "Dukungan minim; ekstrakurikuler sering kekurangan fasilitas/perhatian", "1": "Tidak memberikan dukungan terhadap kegiatan ekstrakurikuler"}},
	{"I-3", "Perhatian Waka Kesiswaan terhadap kesejahteraan (wellbeing) siswa", "Student Affairs VP's attention to student welfare and wellbeing", upwardItemWeight, map[string]string{"4": "Sangat peka terhadap kesejahteraan siswa; proaktif berkoordinasi dengan BK/orang tua", "3": "Cukup peka; merespons ketika ada masalah kesejahteraan siswa", "2": "Kurang peka; hanya merespons masalah yang sudah jelas terlihat", "1": "Tidak menunjukkan perhatian terhadap kesejahteraan siswa"}},
	{"I-4", "Responsivitas Waka Kesiswaan terhadap laporan guru tentang siswa bermasalah", "Student Affairs VP's responsiveness to teacher reports on troubled students", upwardItemWeight, map[string]string{"4": "Selalu merespons cepat dan berkoordinasi aktif dengan guru terkait", "3": "Merespons dengan baik meski kadang perlu waktu", "2": "Respons lambat atau tidak melibatkan guru pelapor", "1": "Jarang atau tidak pernah merespons laporan guru tentang siswa bermasalah"}},
	{"I-5", "Koordinasi Waka Kesiswaan dengan guru dalam pembinaan karakter siswa", "Student Affairs VP's coordination with teachers on student character development", upwardItemWeight, map[string]string{"4": "Aktif berkoordinasi dan melibatkan guru dalam program pembinaan karakter", "3": "Berkoordinasi dengan guru secara cukup rutin", "2": "Koordinasi jarang; guru sering tidak dilibatkan", "1": "Tidak ada koordinasi dengan guru dalam pembinaan karakter siswa"}},
	{"I-6", "Keadilan Waka Kesiswaan dalam menangani kasus kedisiplinan siswa", "Student Affairs VP's fairness in handling student disciplinary cases", upwardItemWeight, map[string]string{"4": "Selalu adil dan konsisten; keputusan disiplin berdasarkan fakta, bukan favoritisme", "3": "Umumnya adil; sesekali ada persepsi ketidakadilan minor", "2": "Kadang terlihat pilih kasih atau tidak konsisten menangani kasus", "1": "Sering dianggap tidak adil dalam menangani kasus kedisiplinan siswa"}},
}
