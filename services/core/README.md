# Core Service (SEKOCI LMS)

Service `core` menyediakan fondasi sistem:
- **Approval Center** (Menu 79, FR-79-*, BR-GEN-09): Alur approval terpusat, multi-tahap, batas nominal, delegasi, eskalasi otomatis SLA, dan maker-checker.
- **Integration Exception Queue** (Menu 80, FR-80-*, BR-INT-01): Monitoring kegagalan integrasi teknis, retry aman/idempoten, penugasan, dan penandaan selesai dengan alasan wajib.
- **Audit Trail Pusat** (Menu 06, FR-06-*): Pencatatan terpusat seluruh aksi sensitif dan event keamanan dengan masking data sensitif.

## Port
- Port: `8091`
- Database: `teratai_core`

## Kontrak Internal
- `POST /internal/approvals` (`X-Internal-Key`)
- `GET /internal/approvals?source_service=&source_id=` (`X-Internal-Key`)
- `POST /internal/approvals/{id}/cancel` (`X-Internal-Key`)
- `POST /internal/exceptions` (`X-Internal-Key`)
- `POST /internal/audit` (`X-Internal-Key`)
