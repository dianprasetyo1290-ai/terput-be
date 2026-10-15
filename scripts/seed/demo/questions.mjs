// Small multiple-choice banks per subject family for the demo CBT exams:
// [question, [options], index of the correct option]. Mathematics is
// generated (different numbers per grade).
const BANK = {
  indonesia: [
    ['Bagian teks narasi yang berisi pengenalan tokoh dan latar disebut...', ['Orientasi', 'Komplikasi', 'Resolusi', 'Koda'], 0],
    ['Kalimat utama dalam paragraf deduktif terletak di...', ['Awal paragraf', 'Tengah paragraf', 'Akhir paragraf', 'Seluruh paragraf'], 0],
    ['Teks yang menggambarkan suatu objek secara rinci disebut teks...', ['Deskripsi', 'Prosedur', 'Eksplanasi', 'Negosiasi'], 0],
    ['Kata baku dari "apotik" adalah...', ['Apotek', 'Apotik', 'Apoteik', 'Apotiek'], 0],
    ['Tanda baca yang tepat untuk mengakhiri kalimat tanya adalah...', ['Tanda tanya (?)', 'Titik (.)', 'Seru (!)', 'Koma (,)'], 0],
    ['Pesan moral yang ingin disampaikan pengarang disebut...', ['Amanat', 'Tema', 'Alur', 'Latar'], 0],
    ['Langkah-langkah membuat sesuatu ditulis dalam teks...', ['Prosedur', 'Narasi', 'Deskripsi', 'Laporan'], 0],
    ['Sinonim kata "cerdas" adalah...', ['Pandai', 'Malas', 'Lambat', 'Lemah'], 0],
  ],
  inggris: [
    ['"Good morning" is used to greet someone in the...', ['Morning', 'Afternoon', 'Evening', 'Night'], 0],
    ['She ___ to school every day.', ['goes', 'go', 'going', 'gone'], 0],
    ['The opposite of "big" is...', ['small', 'tall', 'long', 'wide'], 0],
    ['What time is it? It is 07.30 = ...', ['half past seven', 'seven fifteen', 'a quarter to eight', 'seven o\'clock'], 0],
    ['"Saya suka membaca buku" in English is...', ['I like reading books', 'I likes read book', 'I am like reading', 'I liked reads books'], 0],
    ['My father\'s sister is my...', ['aunt', 'uncle', 'cousin', 'niece'], 0],
    ['Which one is a fruit?', ['Mango', 'Carrot', 'Spinach', 'Potato'], 0],
    ['They ___ playing football now.', ['are', 'is', 'am', 'be'], 0],
  ],
  ipa: [
    ['Satuan SI untuk massa adalah...', ['Kilogram', 'Gram', 'Ons', 'Newton'], 0],
    ['Perubahan wujud dari cair menjadi gas disebut...', ['Menguap', 'Membeku', 'Mencair', 'Mengembun'], 0],
    ['Organ tumbuhan tempat terjadinya fotosintesis adalah...', ['Daun', 'Akar', 'Batang', 'Bunga'], 0],
    ['Alat untuk mengukur suhu adalah...', ['Termometer', 'Barometer', 'Neraca', 'Mistar'], 0],
    ['Energi yang dimiliki benda karena geraknya disebut energi...', ['Kinetik', 'Potensial', 'Kimia', 'Listrik'], 0],
    ['Makhluk hidup yang dapat membuat makanannya sendiri disebut...', ['Autotrof', 'Heterotrof', 'Pengurai', 'Konsumen'], 0],
    ['Air mendidih pada suhu ... °C (tekanan normal).', ['100', '50', '0', '212'], 0],
    ['Hewan yang berkembang biak dengan bertelur disebut...', ['Ovipar', 'Vivipar', 'Ovovivipar', 'Fragmentasi'], 0],
  ],
  ips: [
    ['Kegiatan menghasilkan barang atau jasa disebut...', ['Produksi', 'Distribusi', 'Konsumsi', 'Investasi'], 0],
    ['Ibu kota Provinsi Jawa Barat adalah...', ['Bandung', 'Bogor', 'Bekasi', 'Cirebon'], 0],
    ['Kerajaan Hindu tertua di Indonesia adalah...', ['Kutai', 'Majapahit', 'Sriwijaya', 'Mataram'], 0],
    ['Hubungan timbal balik antarindividu disebut...', ['Interaksi sosial', 'Mobilitas sosial', 'Stratifikasi', 'Asimilasi'], 0],
    ['Garis khayal yang membagi bumi menjadi utara dan selatan adalah...', ['Khatulistiwa', 'Garis bujur', 'Meridian', 'Garis balik'], 0],
    ['Pasar yang mempertemukan penjual dan pembeli secara langsung disebut pasar...', ['Konkret', 'Abstrak', 'Modal', 'Tenaga kerja'], 0],
    ['Sumpah Pemuda diikrarkan pada tahun...', ['1928', '1908', '1945', '1926'], 0],
    ['Peta yang menggambarkan ketinggian tempat disebut peta...', ['Topografi', 'Politik', 'Iklim', 'Penduduk'], 0],
  ],
  pancasila: [
    ['Lambang sila pertama Pancasila adalah...', ['Bintang', 'Rantai', 'Pohon beringin', 'Kepala banteng'], 0],
    ['Semboyan Bhinneka Tunggal Ika berarti...', ['Berbeda-beda tetapi tetap satu', 'Bersatu kita teguh', 'Gotong royong', 'Satu nusa satu bangsa'], 0],
    ['Musyawarah untuk mufakat sesuai dengan sila ke...', ['Empat', 'Satu', 'Dua', 'Lima'], 0],
    ['Hari Lahir Pancasila diperingati setiap tanggal...', ['1 Juni', '17 Agustus', '1 Oktober', '28 Oktober'], 0],
    ['Contoh sikap toleransi di sekolah adalah...', ['Menghormati teman yang berbeda agama', 'Memilih teman sesuku', 'Mengejek teman', 'Tidak mau berkelompok'], 0],
    ['Kewajiban seorang siswa di sekolah adalah...', ['Menaati tata tertib', 'Bermain saat pelajaran', 'Datang terlambat', 'Mencontek'], 0],
    ['Norma yang berasal dari Tuhan disebut norma...', ['Agama', 'Hukum', 'Kesopanan', 'Kesusilaan'], 0],
    ['Dasar negara Indonesia adalah...', ['Pancasila', 'UUD 1945', 'Bhinneka Tunggal Ika', 'Sumpah Pemuda'], 0],
  ],
  agama: [
    ['Sikap jujur dalam perkataan dan perbuatan termasuk akhlak...', ['Terpuji', 'Tercela', 'Netral', 'Buruk'], 0],
    ['Menolong teman yang kesulitan adalah contoh sikap...', ['Peduli', 'Sombong', 'Iri', 'Malas'], 0],
    ['Berbakti kepada orang tua hukumnya...', ['Wajib', 'Sunah', 'Mubah', 'Makruh'], 0],
    ['Sikap menerima segala ketentuan Tuhan disebut...', ['Tawakal / berserah diri', 'Putus asa', 'Takabur', 'Riya'], 0],
    ['Lawan dari sifat sabar adalah...', ['Pemarah', 'Pemaaf', 'Penyayang', 'Rendah hati'], 0],
    ['Mengucapkan terima kasih atas pemberian orang lain mencerminkan sikap...', ['Bersyukur', 'Kikir', 'Dengki', 'Boros'], 0],
    ['Menepati janji merupakan ciri orang yang...', ['Amanah', 'Munafik', 'Pemalas', 'Pembohong'], 0],
    ['Kebersihan adalah sebagian dari...', ['Iman', 'Harta', 'Ilmu', 'Waktu'], 0],
  ],
  pjok: [
    ['Jumlah pemain sepak bola dalam satu tim di lapangan adalah...', ['11', '9', '7', '5'], 0],
    ['Gerakan awal sebelum berolahraga disebut...', ['Pemanasan', 'Pendinginan', 'Inti', 'Relaksasi'], 0],
    ['Teknik dasar menembak dalam bola basket disebut...', ['Shooting', 'Passing', 'Dribbling', 'Pivot'], 0],
    ['Lari jarak pendek disebut juga...', ['Sprint', 'Maraton', 'Estafet', 'Lari lintas alam'], 0],
    ['Guling depan dalam senam lantai disebut...', ['Forward roll', 'Backward roll', 'Handstand', 'Kayang'], 0],
    ['Manfaat olahraga teratur bagi tubuh adalah...', ['Menjaga kebugaran', 'Mudah lelah', 'Menambah stres', 'Menurunkan imunitas'], 0],
    ['Permainan bola voli dimainkan oleh ... orang per tim.', ['6', '5', '7', '11'], 0],
    ['Makanan bergizi seimbang mengandung karbohidrat, protein, lemak, vitamin, dan...', ['Mineral', 'Pewarna', 'Pengawet', 'Pemanis buatan'], 0],
  ],
  seni: [
    ['Alat musik angklung berasal dari daerah...', ['Jawa Barat', 'Bali', 'Papua', 'Sumatra Utara'], 0],
    ['Tari Saman berasal dari...', ['Aceh', 'Bali', 'Jawa Tengah', 'Maluku'], 0],
    ['Warna primer terdiri dari merah, kuning, dan...', ['Biru', 'Hijau', 'Ungu', 'Jingga'], 0],
    ['Lagu "Ampar-Ampar Pisang" berasal dari...', ['Kalimantan Selatan', 'Jawa Timur', 'Sulawesi', 'Riau'], 0],
    ['Gambar yang hanya menggunakan garis disebut...', ['Sketsa', 'Lukisan', 'Kolase', 'Mozaik'], 0],
    ['Tempo dalam musik berarti...', ['Cepat lambatnya lagu', 'Tinggi rendah nada', 'Keras lembut suara', 'Panjang pendek nada'], 0],
    ['Karya seni dari tempelan potongan kertas disebut...', ['Kolase', 'Patung', 'Batik', 'Relief'], 0],
    ['Wayang kulit dimainkan oleh seorang...', ['Dalang', 'Sinden', 'Penabuh', 'Penari'], 0],
  ],
  informatika: [
    ['Langkah-langkah logis untuk menyelesaikan masalah disebut...', ['Algoritma', 'Program', 'Data', 'Jaringan'], 0],
    ['Perangkat keras untuk memasukkan data adalah...', ['Keyboard', 'Monitor', 'Printer', 'Speaker'], 0],
    ['Satuan terkecil data digital adalah...', ['Bit', 'Byte', 'Kilobyte', 'Megabyte'], 0],
    ['Contoh sistem operasi adalah...', ['Windows', 'Microsoft Word', 'Google Chrome', 'Photoshop'], 0],
    ['Kata sandi yang kuat sebaiknya terdiri dari...', ['Kombinasi huruf, angka, dan simbol', 'Tanggal lahir', 'Nama sendiri', '123456'], 0],
    ['Jaringan komputer dalam satu gedung disebut...', ['LAN', 'WAN', 'MAN', 'Internet'], 0],
    ['Memecah masalah besar menjadi bagian kecil disebut...', ['Dekomposisi', 'Abstraksi', 'Pengenalan pola', 'Debugging'], 0],
    ['Aplikasi untuk mengolah angka dan tabel adalah...', ['Spreadsheet', 'Pengolah kata', 'Presentasi', 'Peramban'], 0],
  ],
  jaringan: [
    ['Perangkat yang menghubungkan beberapa jaringan berbeda adalah...', ['Router', 'Hub', 'Repeater', 'Kabel UTP'], 0],
    ['Topologi dengan satu perangkat pusat disebut topologi...', ['Star', 'Bus', 'Ring', 'Mesh'], 0],
    ['Urutan kabel T568B diawali warna...', ['Putih oranye', 'Putih hijau', 'Biru', 'Coklat'], 0],
    ['IP address 192.168.1.10 termasuk kelas...', ['C', 'A', 'B', 'D'], 0],
    ['Subnet mask default kelas C adalah...', ['255.255.255.0', '255.0.0.0', '255.255.0.0', '255.255.255.255'], 0],
    ['Perintah untuk menguji koneksi ke host lain adalah...', ['ping', 'ipconfig', 'tracert', 'netstat'], 0],
    ['Protokol untuk memberikan IP otomatis adalah...', ['DHCP', 'DNS', 'FTP', 'HTTP'], 0],
    ['Alat untuk memasang konektor RJ-45 adalah...', ['Crimping tool', 'Obeng', 'Tang potong', 'Multimeter'], 0],
  ],
  akuntansi: [
    ['Persamaan dasar akuntansi adalah...', ['Aset = Liabilitas + Ekuitas', 'Aset = Pendapatan - Beban', 'Liabilitas = Aset + Ekuitas', 'Ekuitas = Aset + Liabilitas'], 0],
    ['Pencatatan transaksi pertama kali dilakukan di...', ['Jurnal umum', 'Buku besar', 'Neraca', 'Laporan laba rugi'], 0],
    ['Akun kas bertambah dicatat di sisi...', ['Debit', 'Kredit', 'Saldo', 'Ekuitas'], 0],
    ['Utang usaha termasuk kelompok akun...', ['Liabilitas', 'Aset', 'Pendapatan', 'Beban'], 0],
    ['Laporan yang menunjukkan laba atau rugi disebut...', ['Laporan laba rugi', 'Neraca', 'Arus kas', 'Neraca saldo'], 0],
    ['Pembelian perlengkapan secara tunai akan...', ['Menambah perlengkapan, mengurangi kas', 'Menambah kas', 'Menambah utang', 'Mengurangi modal'], 0],
    ['Daftar saldo akun-akun buku besar disebut...', ['Neraca saldo', 'Jurnal', 'Kertas kerja', 'Laporan ekuitas'], 0],
    ['Prive adalah pengambilan untuk keperluan...', ['Pribadi pemilik', 'Operasional', 'Gaji karyawan', 'Pembayaran utang'], 0],
  ],
  umum: [
    ['Kegiatan merencanakan usaha disebut...', ['Perencanaan bisnis', 'Pemasaran', 'Produksi', 'Evaluasi'], 0],
    ['Sikap pantang menyerah merupakan ciri seorang...', ['Wirausaha', 'Penonton', 'Konsumen', 'Pembeli'], 0],
    ['Bekerja sama dalam kelompok membutuhkan sikap...', ['Saling menghargai', 'Egois', 'Acuh', 'Malas'], 0],
    ['Langkah pertama memecahkan masalah adalah...', ['Mengidentifikasi masalah', 'Menyimpulkan', 'Mengevaluasi', 'Mempresentasikan'], 0],
    ['Hasil karya yang baru disebut juga...', ['Inovasi', 'Imitasi', 'Duplikasi', 'Repetisi'], 0],
    ['Promosi produk bertujuan untuk...', ['Memperkenalkan produk', 'Menaikkan biaya', 'Mengurangi pembeli', 'Menghentikan produksi'], 0],
    ['Presentasi yang baik harus...', ['Jelas dan runtut', 'Panjang dan berbelit', 'Tanpa persiapan', 'Membaca seluruh teks'], 0],
    ['Kemampuan mengatur waktu disebut...', ['Manajemen waktu', 'Manajemen risiko', 'Manajemen keuangan', 'Manajemen produksi'], 0],
  ],
}

function family(subject) {
  const n = subject.toLowerCase()
  if (n.includes('indonesia')) return 'indonesia'
  if (n.includes('inggris')) return 'inggris'
  if (n.includes('alam') || n === 'ipas') return 'ipa'
  if (n.includes('sosial')) return 'ips'
  if (n.includes('pancasila')) return 'pancasila'
  if (n.includes('agama')) return 'agama'
  if (n.includes('pjok')) return 'pjok'
  if (n.includes('seni') || n.includes('sunda') || n.includes('prakarya')) return 'seni'
  if (n.includes('informatika')) return 'informatika'
  if (n.includes('jaringan')) return 'jaringan'
  if (n.includes('akuntansi')) return 'akuntansi'
  if (n.includes('matematika')) return 'matematika'
  return 'umum'
}

function mathQuestions(grade) {
  const g = Number(grade)
  const out = []
  for (let i = 0; i < 8; i++) {
    const a = 3 + ((g * 7 + i * 5) % 17)
    const b = 2 + ((g * 3 + i * 11) % 13)
    let qText, ans
    if (g <= 3) { qText = `Hasil dari ${a} + ${b} adalah...`; ans = a + b }
    else if (g <= 6) { qText = `Hasil dari ${a} × ${b} adalah...`; ans = a * b }
    else if (g <= 9) { qText = `Jika x = ${b}, nilai dari ${a}x - ${a + b} adalah...`; ans = a * b - (a + b) }
    else { qText = `Nilai x yang memenuhi ${a}x + ${b} = ${a * (i + 2) + b} adalah...`; ans = i + 2 }
    out.push([qText, [String(ans), String(ans + 2), String(ans - 1), String(ans + 5)], 0])
  }
  return out
}

/** 8 questions for a subject and grade; the correct option is moved around. */
export function questionsFor(subject, grade) {
  const fam = family(subject)
  const list = fam === 'matematika' ? mathQuestions(grade) : BANK[fam]
  return list.map(([text, opts, correct], i) => {
    const shift = i % opts.length
    const rotated = opts.map((_, k) => opts[(k + opts.length - shift) % opts.length])
    return { text, options: rotated, correct: (correct + shift) % opts.length }
  })
}
