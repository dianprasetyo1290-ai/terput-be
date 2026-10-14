package model

// CorrectionRequestError validates a payment correction request (menu 29,
// AC-29-04) and returns the user-facing reason it is refused, or "". Only
// cashier payments are corrected, by cancelling (reversing) them; a
// successful online payment is returned to the payer through a refund, and
// a payment recorded for the wrong student is cancelled and re-recorded.
func CorrectionRequestError(source, kind string) string {
	switch source {
	case "cashier":
	case "gateway":
		return "pembayaran online yang sudah berhasil tidak dapat dibatalkan lewat koreksi; ajukan refund di menu Refund"
	default:
		return "sumber koreksi harus pembayaran kasir (cashier)"
	}
	switch kind {
	case "cancel":
		return ""
	case "reassign":
		return "pindah siswa tidak didukung: batalkan pembayaran ini lalu catat ulang pada siswa yang benar"
	default:
		return "jenis koreksi harus cancel"
	}
}
