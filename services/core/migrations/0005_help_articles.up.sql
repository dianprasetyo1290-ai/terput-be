-- BUG-165 (menu 50): admin-managed guides & FAQ for parents/students.
-- school_id NULL = platform default shown to every school; a school article
-- with replaces_id = <default id> replaces (or, unpublished, hides) it.
CREATE TABLE help_articles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id   UUID,
    audience    TEXT NOT NULL CHECK (audience IN ('orang_tua', 'siswa')),
    kind        TEXT NOT NULL CHECK (kind IN ('guide', 'faq')),
    title       TEXT NOT NULL CHECK (length(btrim(title)) > 0),
    body        TEXT NOT NULL CHECK (length(btrim(body)) > 0),
    sort_order  INT  NOT NULL DEFAULT 0,
    published   BOOLEAN NOT NULL DEFAULT true,
    replaces_id UUID REFERENCES help_articles(id) ON DELETE CASCADE,
    created_by  UUID,
    updated_by  UUID,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (replaces_id IS NULL OR school_id IS NOT NULL)
);
CREATE INDEX idx_help_articles_school ON help_articles (school_id, audience, sort_order);
CREATE UNIQUE INDEX uq_help_articles_replacement ON help_articles (school_id, replaces_id) WHERE replaces_id IS NOT NULL;

-- Neutral defaults describing the app itself; schools add their own payment
-- channels, deadlines and contacts.
INSERT INTO help_articles (school_id, audience, kind, title, body, sort_order) VALUES
(NULL, 'orang_tua', 'guide', 'Cara melihat dan membayar tagihan',
'1. Buka menu Tagihan / SPP.
2. Bila Anda memiliki lebih dari satu anak, pilih anak terlebih dahulu.
3. Daftar Tagihan Belum Lunas menampilkan setiap tagihan beserta jatuh tempo dan sisa tagihannya. Tekan tagihan untuk melihat rinciannya.
4. Tekan Bayar Sekarang, lalu pilih salah satu metode pembayaran yang ditampilkan. Metode yang muncul adalah metode yang diaktifkan oleh sekolah.
5. Ikuti instruksi pembayaran yang tampil di layar sampai selesai.
6. Bila sekolah menerima transfer manual, gunakan Unggah Bukti Pembayaran Manual. Status pembayaran menjadi Menunggu Verifikasi sampai petugas keuangan memeriksanya.
7. Pembayaran yang sudah diterima tampil di Riwayat Pembayaran.', 10),
(NULL, 'orang_tua', 'guide', 'Menghubungkan akun dengan data anak',
'Buka menu Klaim Anak dan ikuti langkah yang diminta. Setelah sekolah memverifikasi hubungan Anda dengan anak, data rapor, presensi, ujian, dan tagihan anak tampil di portal.', 20),
(NULL, 'orang_tua', 'guide', 'Notifikasi dan chat',
'Menu Notifikasi berisi pemberitahuan dari sekolah, termasuk tagihan baru dan hasil pembayaran. Menu Chat dipakai untuk berkomunikasi dengan guru atau wali kelas anak sesuai jam layanan yang ditetapkan sekolah.', 30),
(NULL, 'orang_tua', 'faq', 'Mengapa status pembayaran saya masih Menunggu Verifikasi?',
'Pembayaran dengan bukti transfer manual perlu diperiksa petugas keuangan sekolah. Status berubah setelah pemeriksaan selesai. Pembayaran melalui metode otomatis biasanya tercatat tanpa verifikasi manual.', 110),
(NULL, 'orang_tua', 'faq', 'Mengapa data anak saya belum tampil?',
'Data anak tampil setelah klaim anak disetujui sekolah. Bila klaim belum disetujui, hubungi sekolah melalui menu Layanan Sekolah.', 120),
(NULL, 'orang_tua', 'faq', 'Ke mana saya mengajukan pertanyaan atau permohonan lain?',
'Gunakan menu Layanan Sekolah untuk mengajukan permohonan dan memantau statusnya.', 130);
