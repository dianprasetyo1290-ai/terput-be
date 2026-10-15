// Phase 19: the teachers' teaching kit (perangkat ajar) of SD and SMK — for
// every subject teacher the annual programme (Prota) and semester programme
// (Prosem, weeks before 15 October marked done) of the grades they teach,
// Rencana Pembelajaran Mendalam (RPM) for the chapter running now (published)
// and the next one (draft), and in the classes the learning materials and
// assignments they post, with the students' submissions graded or waiting.
import { addDays, at, date, insert, merge, rng, uid } from './lib.mjs'
import { MODEL } from './model.mjs'

const SMP = '11111111-1111-1111-1111-111111111111'
const TODAY = '2026-10-15'

const fase = (level, grade) => (level === 'SD' ? (grade <= 2 ? 'A' : grade <= 4 ? 'B' : 'C') : grade === 10 ? 'E' : 'F')

// Chapters per subject (and phase where it differs): [title, JP, guiding question?].
// The first four are semester 1, the rest semester 2.
const TOPICS = {
  'Matematika': {
    A: [['Bilangan Cacah sampai 100', 20, 'Ada berapa banyak kelereng jika dikelompokkan per sepuluh?'], ['Penjumlahan dan Pengurangan', 24, 'Bagaimana cara menghitung sisa uang jajan?'],
      ['Mengenal Bangun Datar', 16, 'Bentuk apa saja yang kamu temukan di ruang kelas?'], ['Pengukuran Panjang Tidak Baku', 12], ['Pola Bilangan dan Gambar', 14], ['Waktu dan Jam', 12]],
    B: [['Bilangan Cacah sampai 10.000', 18, 'Bagaimana cara membaca harga sepeda Rp1.250.000?'], ['Perkalian dan Pembagian', 24, 'Bagaimana membagi 48 kue sama banyak ke 6 teman?'],
      ['Pecahan Senilai', 18, 'Jika satu pizza dibagi 8, berapa bagian yang dimakan 3 anak?'], ['Keliling dan Luas Bangun Datar', 16], ['Diagram Gambar dan Batang', 12], ['Sudut', 12]],
    C: [['Bilangan Bulat dan Garis Bilangan', 18, 'Mengapa suhu di puncak gunung bisa di bawah nol?'], ['Operasi Pecahan dan Desimal', 24, 'Bagaimana menghitung diskon 25% saat berbelanja?'],
      ['Rasio dan Skala Peta', 16, 'Berapa jarak sebenarnya Bekasi-Bandung jika di peta 5 cm?'], ['Volume Kubus dan Balok', 18], ['Statistika: Rata-rata dan Modus', 14], ['Bangun Ruang Gabungan', 12]],
    E: [['Eksponen dan Logaritma', 20, 'Bagaimana bunga majemuk tabungan bertambah setiap tahun?'], ['Barisan dan Deret', 18, 'Berapa total tabungan jika setiap bulan menabung bertambah Rp10.000?'],
      ['Sistem Persamaan Linear', 18, 'Bagaimana menentukan harga satuan dua paket kuota internet?'], ['Fungsi Kuadrat', 16], ['Trigonometri Dasar', 16], ['Statistika Data Tunggal', 14]],
    F: [['Matriks', 18, 'Bagaimana tabel stok gudang diolah dengan cepat?'], ['Program Linear', 18, 'Bagaimana memaksimalkan keuntungan usaha dengan bahan terbatas?'],
      ['Peluang', 16, 'Seberapa besar peluang lolos seleksi kerja?'], ['Statistika Data Kelompok', 16], ['Matematika Keuangan', 14], ['Geometri Bidang', 12]],
  },
  'Bahasa Indonesia': {
    A: [['Bunyi Apa?', 20, 'Huruf apa yang kamu dengar pada kata "bola"?'], ['Ini Temanku', 20, 'Bagaimana cara memperkenalkan diri dengan sopan?'], ['Lihat Sekitar', 18], ['Mengenal Cerita Rakyat', 16], ['Aku Bisa!', 16], ['Kata Tanya', 14]],
    B: [['Membaca Teks Narasi', 20, 'Apa yang membuat sebuah cerita menarik dibaca?'], ['Menulis Pengalaman Pribadi', 18, 'Pengalaman apa yang paling berkesan saat liburan?'],
      ['Teks Prosedur Sederhana', 16], ['Puisi Anak', 14], ['Dongeng Fabel', 16], ['Surat Pribadi', 12]],
    C: [['Teks Eksplanasi', 18, 'Mengapa banjir sering terjadi di musim hujan?'], ['Teks Persuasi dan Iklan', 18, 'Mengapa kita tertarik membeli barang setelah melihat iklan?'],
      ['Membaca Cerita Fantasi', 16], ['Menulis Laporan Pengamatan', 16], ['Pidato Singkat', 14], ['Biografi Tokoh', 14]],
    E: [['Teks Laporan Hasil Observasi', 18, 'Bagaimana melaporkan hasil kunjungan industri dengan objektif?'], ['Teks Eksposisi', 18, 'Apakah ponsel perlu dibatasi di sekolah?'],
      ['Teks Anekdot', 14], ['Teks Negosiasi', 16], ['Hikayat dan Cerpen', 14], ['Debat', 14]],
    F: [['Surat Lamaran Pekerjaan', 18, 'Apa yang membuat HRD tertarik membaca lamaranmu?'], ['Teks Prosedur Kompleks', 16, 'Bagaimana menulis SOP yang mudah diikuti rekan kerja?'],
      ['Karya Ilmiah Sederhana', 18], ['Teks Editorial', 14], ['Presentasi Profesional', 14], ['Resensi', 12]],
  },
  'IPAS': {
    A: [['Bagian Tubuhku', 18, 'Bagaimana tubuh kita membantu bermain?'], ['Merawat Tumbuhan', 16], ['Cuaca Hari Ini', 14], ['Keluargaku', 14], ['Lingkungan Rumah', 12], ['Benda di Sekitarku', 12]],
    B: [['Tumbuhan Sumber Kehidupan', 18, 'Mengapa daun berwarna hijau?'], ['Wujud Zat dan Perubahannya', 18, 'Ke mana perginya es batu yang mencair?'],
      ['Gaya di Sekitar Kita', 16], ['Keragaman Budaya Daerahku', 16], ['Peta dan Arah Mata Angin', 14], ['Energi dan Perubahannya', 14]],
    C: [['Sistem Organ Pernapasan', 18, 'Mengapa napas kita terengah-engah setelah berlari?'], ['Rangkaian Listrik Sederhana', 18, 'Bagaimana lampu senter bisa menyala?'],
      ['Ekosistem dan Rantai Makanan', 16], ['Sejarah Kerajaan Nusantara', 16], ['Kegiatan Ekonomi Masyarakat', 14], ['Tata Surya', 14]],
  },
  'Pendidikan Agama dan Budi Pekerti': [['Akhlak Mulia kepada Orang Tua dan Guru', 16, 'Bagaimana cara menunjukkan hormat kepada orang tua?'], ['Kisah Teladan', 14],
    ['Ibadah Sehari-hari', 16], ['Hidup Rukun dan Toleran', 12], ['Kejujuran dan Tanggung Jawab', 12], ['Peduli Lingkungan', 10]],
  'Pendidikan Pancasila': [['Makna Sila-sila Pancasila', 16, 'Bagaimana Pancasila terlihat dalam kegiatan di sekolah?'], ['Hak dan Kewajiban', 14],
    ['Keberagaman dalam Bingkai Bhinneka Tunggal Ika', 14], ['Norma dan Aturan', 12], ['Gotong Royong', 12], ['NKRI dan Wilayahnya', 10]],
  'Seni Budaya': [['Unsur Rupa: Garis, Bidang, Warna', 12, 'Warna apa yang membuat gambar terasa ceria?'], ['Lagu Daerah Nusantara', 12], ['Gerak Tari Kreasi', 12], ['Kolase dan Montase', 10], ['Ansambel Ritmis', 10], ['Pentas Kelas', 10]],
  'PJOK': [['Gerak Dasar Lokomotor', 12, 'Gerakan apa yang kita lakukan saat bermain kejar-kejaran?'], ['Permainan Bola Besar', 12], ['Senam Lantai', 10], ['Kebugaran Jasmani', 10], ['Atletik: Lari dan Lompat', 10], ['Pola Hidup Sehat', 8]],
  'Bahasa Inggris': [['Greetings and Introductions', 12, 'How do you greet a new friend?'], ['My Family and Home', 12], ['Things in the Classroom', 10], ['Daily Activities', 10], ['Food and Drinks', 10], ['Animals Around Us', 8]],
  'Bahasa Sunda': [['Wawangi Sunda: Kecap Panganteur', 10, 'Kumaha carana nyapa ka kolot nganggo basa lemes?'], ['Kaulinan Barudak', 10], ['Dongeng Sato', 10], ['Undak-usuk Basa', 10], ['Kawih Barudak', 8], ['Aksara Sunda', 8]],
  'Informatika': [['Berpikir Komputasional', 14, 'Bagaimana memecah masalah besar menjadi langkah kecil?'], ['Teknologi Informasi dan Komunikasi', 12], ['Sistem Komputer', 12], ['Jaringan Komputer dan Internet', 12], ['Analisis Data dengan Spreadsheet', 12], ['Algoritma dan Pemrograman', 14]],
  'Dasar-dasar Teknik Jaringan Komputer': [['K3LH di Bengkel Jaringan', 16, 'Apa risiko bekerja dengan perangkat listrik di bengkel?'], ['Pengenalan Perangkat Jaringan', 24, 'Mengapa sekolah butuh router dan switch?'],
    ['Pemasangan Kabel UTP Straight & Cross', 24, 'Kapan kita memakai kabel straight dan kapan cross?'], ['Pengalamatan IP dan Subnetting Dasar', 24], ['Instalasi Sistem Operasi Jaringan', 24], ['Proses Bisnis Layanan ISP', 16]],
  'Administrasi Infrastruktur Jaringan': [['VLAN dan Inter-VLAN Routing', 24, 'Bagaimana memisahkan jaringan guru dan siswa dalam satu switch?'], ['Routing Statis dan Dinamis (OSPF)', 24, 'Bagaimana paket menemukan jalan tercepat?'],
    ['Firewall dan NAT pada MikroTik', 24], ['Manajemen Bandwidth', 18], ['Wireless LAN dan Hotspot', 18], ['Monitoring Jaringan', 16]],
  'Dasar-dasar Akuntansi dan Keuangan Lembaga': [['Profesi dan Etika Akuntan', 16, 'Mengapa akuntan harus jujur meski tidak diawasi?'], ['Persamaan Dasar Akuntansi', 24, 'Bagaimana transaksi warung memengaruhi harta dan modal?'],
    ['Jurnal Umum Perusahaan Jasa', 24, 'Mengapa setiap transaksi harus dicatat dua sisi?'], ['Buku Besar dan Neraca Saldo', 24], ['Laporan Keuangan Sederhana', 24], ['Pengelolaan Kas Kecil', 16]],
  'Akuntansi Keuangan': [['Siklus Akuntansi Perusahaan Dagang', 24, 'Apa bedanya mencatat penjualan barang dan penjualan jasa?'], ['Persediaan Barang Dagang (FIFO & Rata-rata)', 24, 'Harga beli berubah-ubah; berapa nilai stok kita?'],
    ['Aset Tetap dan Penyusutan', 18], ['Rekonsiliasi Bank', 18], ['Laporan Keuangan Perusahaan Dagang', 24], ['Aplikasi Akuntansi Komputer', 24]],
  'Projek Kreatif dan Kewirausahaan': [['Mengenali Peluang Usaha di Sekitar Sekolah', 16, 'Masalah apa di sekitar sekolah yang bisa jadi peluang usaha?'], ['Business Model Canvas', 16],
    ['Membuat Prototipe Produk/Jasa', 20], ['Pemasaran Digital', 16], ['Pameran Produk (Market Day)', 16], ['Laporan Keuangan Usaha', 12]],
}

const MODELS = ['Problem Based Learning (PBL)', 'Project Based Learning (PjBL)', 'Discovery Learning', 'Inquiry Learning', 'Pembelajaran Berdiferensiasi']
const LMS = ['Tatap Muka & Diskusi', 'Tatap Muka', 'Praktikum Mandiri', 'Tugas Mandiri', 'Tatap Muka & Kuis', 'Proyek']
const MONTHS = ['Januari', 'Februari', 'Maret', 'April', 'Mei', 'Juni', 'Juli', 'Agustus', 'September', 'Oktober', 'November', 'Desember']
const SEM1_WEEKS = (() => {
  const w = []
  for (let d = new Date('2026-07-20T00:00:00Z'); d <= new Date('2026-12-07T00:00:00Z'); d = addDays(d, 7)) {
    if (['2026-10-26', '2026-12-07'].includes(date(d))) continue // PTS and PAS weeks
    w.push(d)
  }
  return w
})()

const chapters = (subject, f) => {
  const t = TOPICS[subject]
  return Array.isArray(t) ? t : t?.[f] ?? Object.values(t ?? {})[0] ?? [['Materi Pokok 1', 12], ['Materi Pokok 2', 12], ['Materi Pokok 3', 12], ['Materi Pokok 4', 12], ['Materi Pokok 5', 12], ['Materi Pokok 6', 12]]
}

export function generate() {
  const evalOut = []
  const acadOut = []
  for (const u of MODEL.filter((x) => x.schoolId !== SMP)) {
    const r = rng('teaching:' + u.key)
    const S = u.schoolId
    const Y = u.yearId
    const id = (...k) => uid('teach', u.key, ...k)
    evalOut.push(`-- ===== ${u.level} =====`)
    acadOut.push(`-- ===== ${u.level} =====`)
    const prota = []
    const protaItems = []
    const prosem = []
    const prosemItems = []
    const plans = []

    for (const [si, s] of u.subjects.entries()) {
      const classes = u.classes.filter((c) => !s.major || s.major === c.major)
      // Two grades per teacher (the first and a later one), so the list varies.
      const grades = [...new Set(classes.map((c) => Number(c.grade)))]
      const mine = [...new Set([grades[si % grades.length], grades[(si + 2) % grades.length]])]
      for (const g of mine) {
        const f = fase(u.level, g)
        const kelas = u.level === 'SD' ? String(g) : { 10: 'X', 11: 'XI', 12: 'XII' }[g]
        const chs = chapters(s.name, f)
        const pid = id('prota', s.id, g)
        prota.push({ id: pid, school_id: S, academic_year_id: Y, guru_user_id: s.teacher.id, mata_pelajaran: s.name, kelas, created_at: at('2026-07-08') })
        chs.forEach(([title, jp], i) => protaItems.push({ id: id('prota-item', pid, i), prota_id: pid, no: i + 1, semester: i < 4 ? '1' : '2', elemen_kode_bab: `BAB ${i + 1}`,
          kompetensi_dasar_tp: title, alokasi_waktu_jp: jp, keterangan_target_asesmen: i === 1 ? 'Formatif & Sumatif PTS' : i === 3 ? 'Formatif & Sumatif PAS' : i === 5 ? 'Formatif & Sumatif PAT' : 'Formatif' }))
        const psid = id('prosem', s.id, g)
        prosem.push({ id: psid, school_id: S, academic_year_id: Y, guru_user_id: s.teacher.id, mata_pelajaran: s.name, kelas, created_at: at('2026-07-09') })
        // Semester 1: the four chapters spread over the effective weeks.
        const per = Math.floor(SEM1_WEEKS.length / 4)
        SEM1_WEEKS.forEach((wk, wi) => {
          const ci = Math.min(3, Math.floor(wi / per))
          const [title, jp] = chs[ci]
          const part = wi - ci * per + 1
          const d = date(wk)
          prosemItems.push({ id: id('prosem-item', psid, wi), prosem_id: psid, no: wi + 1,
            materi_pokok_aktivitas: part === 1 ? `${title}: pengenalan konsep` : wi === (ci + 1) * per - 1 || (ci === 3 && wi === SEM1_WEEKS.length - 1) ? `${title}: asesmen sumatif` : `${title}: pendalaman ${part - 1}`,
            alokasi_jp: Math.max(2, Math.round(jp / per)), bulan_target: MONTHS[wk.getUTCMonth()], minggu_ke: Math.ceil(wk.getUTCDate() / 7), estimasi_tanggal: d,
            tipe_aktivitas_lms: wi === (ci + 1) * per - 1 ? 'Ujian CBT' : LMS[(wi + si) % LMS.length], status_input_guru: d < TODAY ? 'Selesai' : 'Belum' })
        })
        // RPM: chapter 3 (running in October) published, chapter 4 a draft.
        const cls = classes.find((c) => Number(c.grade) === g)
        const model = MODELS[(si + g) % MODELS.length]
        ;[[2, 'published'], [3, 'draft']].forEach(([ci, status]) => {
          const [title, jp, question] = chs[ci]
          const full = status === 'published' || si % 2 === 0
          const meetings = Math.max(1, Math.round(jp / (u.level === 'SD' ? 4 : 6)))
          plans.push({ id: id('rpm', s.id, g, ci), school_id: S, academic_year_id: Y, guru_user_id: s.teacher.id,
            title: `RPM ${s.name} Kelas ${kelas}: ${title}${status === 'draft' ? ' (draf)' : ''}`,
            section_a: { fase: f, kelas: cls?.name ?? kelas, mata_pelajaran: s.name, alokasi_waktu: `${Math.min(jp, u.level === 'SD' ? 4 : 6)} JP x ${u.level === 'SD' ? 35 : 45} menit (${meetings} pertemuan)`,
              ...(full ? { karakteristik_peserta_didik: u.level === 'SD'
                ? 'Sebagian besar peserta didik belajar efektif melalui benda konkret dan permainan; 4 anak masih perlu pendampingan membaca instruksi.'
                : 'Peserta didik aktif bertanya saat praktik; sebagian masih kesulitan mengaitkan teori dengan kasus nyata di industri.' } : {}) },
            section_b: full ? {
              capaian_pembelajaran: `Pada akhir Fase ${f}, peserta didik memahami dan menerapkan ${title.toLowerCase()} dalam konteks kehidupan sehari-hari${u.level === 'SMK' ? ' dan dunia kerja' : ''}.`,
              tujuan_pembelajaran: `Peserta didik mampu menjelaskan konsep ${title.toLowerCase()} dan menyelesaikan masalah kontekstual terkait.`,
              pemahaman_bermakna: `${title} membantu kita ${u.level === 'SD' ? 'memahami hal-hal di sekitar rumah dan sekolah' : 'bekerja lebih tepat dan profesional'}.`,
              pertanyaan_pemantik: question ?? `Mengapa ${title.toLowerCase()} penting dalam kehidupan sehari-hari?`,
              asesmen_diagnostik: u.level === 'SD' ? 'Tanya jawab lisan dan 3 soal bergambar untuk memetakan kesiapan.' : 'Kuis 5 soal daring untuk memetakan pemahaman prasyarat.',
              model_pembelajaran: model, media_ajar: u.level === 'SD' ? 'Benda konkret, kartu bergambar, LKPD' : s.major ? 'Perangkat praktik bengkel/lab, jobsheet, video tutorial' : 'Slide presentasi, LKPD, Kahoot',
            } : { tujuan_pembelajaran: `Peserta didik mampu menjelaskan konsep ${title.toLowerCase()}.` },
            section_c: full ? {
              kegiatan_awal: `Salam, doa, presensi; apersepsi dengan pertanyaan pemantik "${question ?? title}".`,
              kegiatan_inti: model.startsWith('Project') ? 'Kelompok merancang dan mengerjakan proyek kecil, guru memfasilitasi dengan lembar kerja proyek.'
                : model.startsWith('Problem') ? 'Peserta didik berdiskusi kelompok memecahkan masalah kontekstual, lalu mempresentasikan solusi.'
                  : 'Peserta didik mengamati, mencoba, dan menyimpulkan konsep bersama kelompok dengan bimbingan guru.',
              asesmen_formatif: u.level === 'SD' ? 'Observasi dengan ceklis dan LKPD 5 soal' : s.major ? 'Penilaian unjuk kerja dengan rubrik dan jobsheet' : 'LKPD 10 soal dan kuis daring',
              kegiatan_penutup: 'Menyimpulkan bersama, umpan balik, dan menyampaikan rencana pertemuan berikutnya.',
              refleksi: 'Peserta didik menuliskan satu hal yang dipelajari dan satu hal yang masih membingungkan (exit ticket).',
            } : {},
            dimensi_lulusan: full ? {
              bernalar_kritis: `Menganalisis informasi saat mempelajari ${title.toLowerCase()}.`,
              bergotong_royong: 'Bekerja sama dalam kelompok 4-5 orang.',
              ...(s.major || s.name === 'Informatika' ? { literasi_digital: 'Menggunakan perangkat digital secara aman dan bertanggung jawab.' } : { mandiri: 'Menyelesaikan LKPD secara mandiri sebelum diskusi.' }),
              ...(s.name.startsWith('Pendidikan Agama') ? { beriman_bertakwa_berakhlak_mulia: 'Membiasakan doa dan sikap santun selama pembelajaran.' } : {}),
            } : {},
            status, created_at: at(status === 'published' ? '2026-09-28' : '2026-10-12'), updated_at: at(status === 'published' ? '2026-10-02' : '2026-10-13') })
        })
      }
    }
    evalOut.push(insert('prota', prota), insert('prota_items', protaItems), insert('prosem', prosem), insert('prosem_items', prosemItems, { chunk: 1000 }), insert('lesson_plans', plans))

    // ---- classroom materials, assignments and submissions ----
    const materials = []
    const assignments = []
    const submissions = []
    for (const c of u.classes) {
      const subjects = u.subjects.filter((s) => !s.major || s.major === c.major)
      const picks = [subjects.find((s) => s.name === 'Matematika'), subjects.find((s) => s.name === 'Bahasa Indonesia'),
        u.level === 'SD' ? subjects.find((s) => s.name === 'IPAS') : subjects.find((s) => s.major)].filter(Boolean)
      for (const [pi, s] of picks.entries()) {
        const cs = uid('class-subject', c.id, s.id)
        const chs = chapters(s.name, fase(u.level, Number(c.grade)))
        const T = s.teacher.id
        ;[[0, '2026-07-27', 'published'], [1, '2026-08-24', 'published'], [2, '2026-09-28', 'published'], [3, '2026-10-14', 'draft']].forEach(([ci, d, status]) => {
          if (ci === 3 && pi > 0) return
          materials.push({ id: id('material', cs, ci), school_id: S, academic_year_id: Y, classroom_subject_id: cs, teacher_user_id: T,
            title: `Modul ${ci + 1}: ${chs[ci][0]}${status === 'draft' ? ' (draf)' : ''}`,
            description: status === 'draft' ? 'Draf materi untuk pertemuan November, masih ditinjau.' : `Ringkasan materi, contoh, dan latihan ${chs[ci][0].toLowerCase()}.`,
            link_url: ci === 1 ? `https://teratai.sch.id/files/${u.key}/${s.name.toLowerCase().replace(/[^a-z]+/g, '-')}-${ci + 1}.pdf` : null,
            file_key: ci !== 1 && status === 'published' ? `academic/materials/${cs}/modul-${ci + 1}.pdf` : null, file_name: ci !== 1 && status === 'published' ? `Modul-${ci + 1}.pdf` : null,
            file_mime: ci !== 1 && status === 'published' ? 'application/pdf' : null, file_size: ci !== 1 && status === 'published' ? 300000 + ci * 41000 + pi * 7000 : null,
            status, published_at: status === 'published' ? at(d, '07:00') : null, created_at: at(d, '06:00') })
        })
        const tasks = [
          ['past', `${u.level === 'SD' ? 'LKPD' : 'Tugas'}: ${chs[1][0]}`, '2026-09-04', '2026-08-28', 'published'],
          ['now', s.major ? `Jobsheet Praktik: ${chs[2][0]}` : `Proyek Mini: ${chs[2][0]}`, '2026-10-20', '2026-10-09', 'published'],
          ...(pi === 0 ? [['next', `Latihan: ${chs[3][0]} (draf)`, '2026-11-13', null, 'draft']] : []),
        ]
        for (const [k, title, due, pub, status] of tasks) {
          const aid = id('assignment', cs, k)
          assignments.push({ id: aid, school_id: S, academic_year_id: Y, classroom_subject_id: cs, teacher_user_id: T, title,
            instructions: k === 'past' ? 'Kerjakan seluruh soal pada LKPD, foto jawaban dengan jelas lalu unggah.' : k === 'now'
              ? (s.major ? 'Ikuti langkah pada jobsheet, dokumentasikan hasil praktik dengan foto/screenshot, unggah laporan PDF.' : 'Kerjakan berkelompok sesuai pembagian, unggah hasil proyek beserta foto proses.')
              : 'Draf latihan, belum dibagikan.', due_at: at(due, '23:59'), max_score: 100, status, published_at: pub ? at(pub, '07:00') : null, created_at: at(pub ?? '2026-10-14', '06:30') })
          if (status !== 'published') continue
          for (const st of c.students) {
            const rr = rng(`sub:${aid}:${st.id}`)
            const submitted = k === 'past' ? rr.chance(0.92) : rr.chance(0.45)
            if (!submitted) continue
            const late = k === 'past' && rr.chance(0.1)
            const subAt = late ? at('2026-09-05', '19:30') : k === 'past' ? at(`2026-09-0${rr.int(1, 4)}`, `${rr.int(15, 21)}:${rr.int(10, 59)}`) : at(`2026-10-${rr.int(10, 14)}`, `${rr.int(15, 21)}:${rr.int(10, 59)}`)
            const resub = k === 'past' && !late && rr.chance(0.06)
            const graded = k === 'past' && !resub
            const scoreV = graded ? Math.min(100, Math.max(55, Math.round(70 + rr.next() * 28 - (late ? 8 : 0)))) : null
            submissions.push({ id: id('submission', aid, st.id), school_id: S, assignment_id: aid, student_user_id: st.id,
              answer_text: k === 'past' ? 'Jawaban terlampir pada foto LKPD.' : 'Laporan sementara terlampir; akan dilengkapi.', file_key: `academic/submissions/${aid}/${st.id}.jpg`,
              file_name: k === 'past' ? 'jawaban-lkpd.jpg' : 'laporan.pdf', file_mime: k === 'past' ? 'image/jpeg' : 'application/pdf', file_size: 180000 + rr.int(0, 900000),
              submitted_at: subAt, is_late: late, status: graded ? 'graded' : resub ? 'resubmit_requested' : 'submitted', resubmit_count: resub ? 1 : 0, score: scoreV,
              feedback: graded ? (scoreV >= 85 ? 'Sangat baik, langkah pengerjaan runtut.' : scoreV >= 70 ? 'Baik. Perhatikan kembali ketelitian pada beberapa soal.' : 'Pelajari lagi contoh di modul, lalu tanyakan bagian yang belum jelas.')
                : resub ? 'Foto jawaban buram; mohon unggah ulang dengan pencahayaan cukup.' : null,
              graded_at: graded ? at('2026-09-08', '13:00') : null, graded_by: graded ? T : null, created_at: subAt })
          }
        }
      }
    }
    acadOut.push(insert('learning_materials', materials), insert('learning_assignments', assignments),
      insert('learning_submissions', submissions, { conflict: '(assignment_id, student_user_id) DO NOTHING', chunk: 1000 }))
  }
  return merge({ teratai_evaluation: evalOut.join('\n\n'), teratai_academic: acadOut.join('\n\n') })
}
