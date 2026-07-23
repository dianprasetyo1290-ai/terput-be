# evaluation-service

Owns two "teacher-quality" features for the Sekoci Go+Vue rewrite:

1. **RPM (Rencana Pembelajaran Mendalam)** — lesson plans, guru-authored, admin read-only.
2. **Penilaian Guru** — teacher performance evaluation: admin-run periods, 6
   evaluator categories (Kepala Sekolah, Waka Kurikulum, Waka Kesiswaan,
   Guru-self, Guru-peer, Siswa), weighted final score, Promotif/Tidak
   Promotif labeling, printable per-guru report.

Port `8085`, database `skci_evaluation`. Follows `docs/CONVENTIONS.md`
(response envelope, JWT claims/middleware, multi-tenancy via `school_id`,
academic-year scoping via `academic_year_id`).

## Run locally

```bash
cp .env.example .env   # optional, docker-compose already has working defaults
docker compose up -d --build
curl http://localhost:8085/healthz   # {"status":"ok"}
docker compose down
```

Migrations run automatically on service startup (`golang-migrate`, files in
`migrations/`). No manual migration step needed.

To run outside Docker (e.g. against a local Postgres on port 5485 from
`docker compose up -d evaluation-db`):

```bash
go build ./... && DATABASE_URL="postgres://sekoci:change-me@localhost:5485/skci_evaluation?sslmode=disable" \
  MIGRATIONS_PATH=migrations ./evaluation
```

## Status derivation (Penilaian Guru periods)

Status is **never stored** — it's computed on every read from `now()` vs. the
period's `publish_start_date` / `publish_end_date` plus the `published` flag
(see `internal/domain/status.go: PeriodStatus`):

| Condition                                                        | Status               |
|--------------------------------------------------------------------|-----------------------|
| `now()` is after `publish_end_date` (end of that day)              | `Selesai`              |
| not `published`                                                    | `Draft`                |
| `published` and `now() < publish_start_date`                       | `Published`            |
| `published` and `publish_start_date <= now() <= publish_end_date`  | `Sedang Berlangsung`   |

`Selesai` wins over everything else — once the end date has passed the
period is finished regardless of the publish flag. Evaluators can only
submit (`POST /periods/{id}/evaluations`) while status is exactly
`Sedang Berlangsung`.

## Evaluator-category weighting

Sourced from the client-provided reference spreadsheet `RAPOT KINERJA GURU
SMK 2025-2026 (2).xlsx`, sheet `📊 RINGKASAN`:

| Form | Category                          | Weight |
|------|-------------------------------------|--------|
| F    | Self-Assessment (Guru sendiri)      | 25%    |
| E    | Penilaian Murid (Siswa)             | 22%    |
| A    | Kepala Sekolah                      | 20%    |
| B    | Waka Kurikulum                      | 18%    |
| C    | Waka Kesiswaan                      | 10%    |
| D    | Teman Sejawat (Peer)                | 5%     |

Sums to 100%. Each form's scoring items (with per-item weight fractions and
1–4 rubric labels) are also pulled directly from that spreadsheet and seeded
via `internal/store/default_forms.go` / `FormStore.BootstrapDefaults` — this
runs automatically (idempotent) the first time `GET /forms` or any period
create/update is called for a school.

**Scoring math**: a task (one evaluator's submission for one guru under one
form) gets `raw_score` = weighted average of that evaluator's item scores,
using each item's own weight, renormalized by the sum of weights of items
actually answered (so item weights are for rubric-detail/granularity, not
summed directly into the final score). A category's score = simple average
of `raw_score` across all evaluators who submitted under that category for
that guru. `final_score` = Σ (category average × category weight), scale
0.00–4.00. `Promotif` if `final_score >= 3.00`, else `Tidak Promotif` (per
spec). A bonus 4-tier `performance_category` label (Sangat Baik / Baik /
Cukup / Perlu Pembinaan) is also included, taken from the same spreadsheet,
as extra detail — not required by spec but free given the source data.

## API summary

All routes except `/healthz` require `Authorization: Bearer <JWT>`.

**RPM**
- `POST /me/lesson-plans` / `GET /me/lesson-plans` / `GET,PUT,DELETE /me/lesson-plans/{id}` — guru's own CRUD.
- `GET /lesson-plans?guru_id=` and `GET /lesson-plans/guru/{guru_id}` — admin read-only list/detail.

**Penilaian Guru**
- `GET /forms` — the 6 evaluator-category forms + items (bootstraps defaults for the caller's school if missing).
- `POST /periods`, `GET /periods`, `GET/PUT/DELETE /periods/{id}` — period CRUD. `POST`/`PUT` regenerate the eagerly-known tasks (see below).
- `GET /periods/{id}/confirm-delete` — returns `{message: "apakah anda yakin ingin menghapus <title>"}` for the frontend's confirm dialog; the actual delete is a separate `DELETE`.
- `POST /periods/{id}/publish` / `POST /periods/{id}/unpublish` — toggles the `published` flag.
- `GET /periods/{id}/available-peer-evaluators` — guru list for field K(b), server-side filtered to exclude anyone already in K(a) (`peer_evaluatee_ids`).
- `POST /periods/{id}/evaluations` — submit one evaluator's scores; body `{evaluator_type, target_teacher_id, classroom_id?, answers:[{item_id,score}]}`. Server validates the caller is actually authorized for that `evaluator_type` → `target_teacher_id` pairing under the period's config.
- `GET /periods/{id}/my-tasks` — caller's own assignment list (bonus convenience endpoint, not in spec but useful for a "to-do" UI).
- `GET /periods/{id}/results?guru_id=` — per-category progress + weighted final score.
- `GET /periods/{id}/guru/{guru_id}/report` — printable report payload.

## curl examples

Tokens are HS256 JWTs signed with `SEKOCI_JWT_SECRET` (dev default
`change-me`), shaped per CONVENTIONS.md. Since
identity-service issues these in the real system, for local testing you can
mint one with any small Go/Node/Python HS256 signer using the same secret
and claim shape:

```json
{"sub":"<user-uuid>","school_id":"<school-uuid>","active_role":"admin","roles":[],"exp":1234567890}
```

**Create a period (admin):**
```bash
curl -X POST http://localhost:8085/periods \
  -H "Authorization: Bearer $TOK_ADMIN" -H "Content-Type: application/json" \
  -d '{
    "title": "Penilaian Kinerja Guru Semester Genap 2025/2026",
    "publish_start_date": "2026-07-17",
    "publish_end_date": "2026-07-25",
    "academic_year_id": "<academic-year-uuid>",
    "semester": "Genap",
    "headmaster_user_id": "<kepsek-uuid>",
    "curriculum_vp_user_id": "<waka-kurikulum-uuid>",
    "student_affairs_vp_user_id": "<waka-kesiswaan-uuid>",
    "target_teacher_ids": ["<guru1-uuid>"],
    "peer_evaluatee_ids": ["<guru1-uuid>"],
    "peer_evaluator_ids": ["<guru2-uuid>"],
    "student_classroom_ids": ["<classroom-uuid>"],
    "lead_assessor_name": "Admin Sekolah"
  }'
```

**Publish it:**
```bash
curl -X POST http://localhost:8085/periods/<period-id>/publish -H "Authorization: Bearer $TOK_ADMIN"
```

**Submit a self-assessment (guru):**
```bash
curl -X POST http://localhost:8085/periods/<period-id>/evaluations \
  -H "Authorization: Bearer $TOK_GURU1" -H "Content-Type: application/json" \
  -d '{"evaluator_type":"self","target_teacher_id":"<guru1-uuid>","answers":[
    {"item_id":"<F-1 item id>","score":4}, {"item_id":"<F-2 item id>","score":4}, "...all 7 F items..."
  ]}'
```

**Submit a siswa evaluation (siswa):**
```bash
curl -X POST http://localhost:8085/periods/<period-id>/evaluations \
  -H "Authorization: Bearer $TOK_SISWA" -H "Content-Type: application/json" \
  -d '{"evaluator_type":"siswa","target_teacher_id":"<guru1-uuid>","classroom_id":"<classroom-uuid>","answers":[
    "...all 8 E items..."
  ]}'
```

**Fetch results:**
```bash
curl "http://localhost:8085/periods/<period-id>/results?guru_id=<guru1-uuid>" -H "Authorization: Bearer $TOK_ADMIN"
```

## What was actually tested (2026-07-18, local Docker)

- `go build ./...` and `go vet ./...` clean.
- `docker compose up -d --build` → both containers healthy, migrations applied automatically on boot (log: `migrations applied`), `GET /healthz` → `200 {"status":"ok"}`.
- Full happy path with minted JWTs for 7 personas (admin, 2 guru, kepsek, waka-kurikulum, waka-kesiswaan, siswa):
  1. `POST /periods` → created in `Draft` (verified `published:false`, computed `status:"Draft"` even though today was inside the publish window — confirms the flag, not just dates, gates Draft vs Published).
  2. `POST /periods/{id}/publish` → status flipped to `Sedang Berlangsung`.
  3. `GET /periods/{id}/available-peer-evaluators` → correctly returned an empty list because the only in-scope guru was already in `peer_evaluatee_ids` (also exercised the identity-service-unreachable fallback path, `source` field reported it).
  4. `GET /forms` → bootstrapped all 6 forms + 47 items on first call, weights matched the spreadsheet (A totals 19% due to a rounding artifact in the source sheet itself — see Assumptions).
  5. Submitted all 6 evaluator-category evaluations (self=4, peer=3, kepala_sekolah=4, waka_kurikulum=3, waka_kesiswaan=4, siswa=3) via `POST /periods/{id}/evaluations`.
  6. `GET /periods/{id}/results?guru_id=` → `final_score: 3.55` = `0.25*4 + 0.22*3 + 0.20*4 + 0.18*3 + 0.10*4 + 0.05*3`, hand-verified correct; `promotif_label: "Promotif"`.
  7. `GET /periods/{id}/guru/{guru_id}/report` → same score rekap plus title/guru/school placeholder fields (identity/academic unreachable in this standalone test, so name/NUPTK/kelas fell back to documented placeholders).
  8. Re-submitted self-assessment with different scores (2 instead of 4) → confirmed idempotent overwrite (`raw_score` updated in place, not duplicated) and `final_score` dropped by exactly `0.25*(4-2)=0.5` to `3.05`, still `Promotif`.
  9. Business-rule rejection: creating a period with the same guru in both `peer_evaluatee_ids` and `peer_evaluator_ids` → `400` with a clear message.
  10. Authorization rejection: guru2 attempting to submit `evaluator_type:"self"` for guru1 → `403`.
  11. `GET /periods/{id}/confirm-delete` → returned `{"message":"apakah anda yakin ingin menghapus <title>"}`.
  12. A second period with `publish_end_date` in the past → after publish, status correctly computed as `Selesai`; `DELETE /periods/{id}` removed it (soft delete), subsequent `GET` → `404`.
  13. `POST/PUT/GET/DELETE /me/lesson-plans` (RPM) as guru, plus admin read-only `GET /lesson-plans` and `GET /lesson-plans/guru/{id}` — all returned expected data; guru could not be blocked from creating/editing their own, admin views were read-only (no admin write endpoint exists for RPM content, by design).
  14. `GET /healthz` unauthenticated → `200`; any other route without a token → `401`.
- `docker compose down` after testing.

## Assumptions

Ambiguities in the spec were resolved as follows (simplest reasonable call
in each case, since this service was built in parallel with 5 siblings and
couldn't block on clarification):

1. **identity-service / academic-service routes are not final.** All cross-service
   calls (`internal/client/clients.go`) are best-effort with a 2s timeout and
   fail soft (placeholder values, empty lists, or a documented fallback) so
   this service is fully testable standalone. Expected route shapes are
   documented as comments at the top of `clients.go`:
   - `GET /internal/users/{id}`, `GET /internal/users?role=guru&school_id=`, `GET /internal/users?jabatan=<jabatan>&school_id=` (identity)
   - `GET /internal/classrooms/{id}/students`, `GET /internal/academic-years?school_id=`, `GET /internal/teaching-assignments?guru_id=` (academic)
   These need reconciling once identity/academic ship their real routes.
2. **"J" (target guru) vs "K(a)/K(b)" (peer cross-assign) are independent lists**, not required to be subsets of each other, per the spec's literal wording. The cross-filter rule (`available-peer-evaluators` excludes K(a) from K(b)'s pool) is enforced both server-side on that read endpoint AND validated at period create/update time (rejects overlapping IDs outright, stricter than "just filter the dropdown" but prevents bad data entering the DB).
3. **Peer assignment is a full cross-product**: every guru in `peer_evaluator_ids` gets a peer-evaluation task against every guru in `peer_evaluatee_ids` (self-pairs skipped). The old Laravel schema stored this as an opaque `peer_assignments` JSON blob with no documented shape, so this is the simplest faithful interpretation of "select-all checkboxes on both sides."
4. **Task generation is eager for 5 of the 6 categories** (kepala_sekolah, waka_kurikulum, waka_kesiswaan, self, peer) — computable entirely from IDs already in the period config, regenerated (idempotently, via `ON CONFLICT DO NOTHING`-style upsert) on every `POST`/`PUT /periods`. **Siswa tasks are lazy** — created on first `POST .../evaluations` from a student — because the actual student roster belongs to academic-service and periodically re-fetching + eagerly materializing a task per student on every period edit felt like unnecessary coupling/cost for a local-dev service. The tradeoff: `expected_count` for the siswa category in `/results` is `null` with an explanatory note whenever academic-service's roster endpoint is unreachable (rather than a guessed number), and `is_complete` conservatively becomes `false` in that case.
5. **Evaluator-type is client-supplied, not purely inferred**, in `POST /periods/{id}/evaluations` (`evaluator_type` field in the body). The frontend already knows which form/tab a user is filling (e.g. a "Self-Assessment" screen vs a "Peer" screen), so this avoids a genuinely ambiguous inference problem (e.g. a Kepala Sekolah who is also in `target_teacher_ids` could plausibly submit either a `self` or `kepala_sekolah` task for themselves — the intent has to come from the UI). The server still fully re-validates the caller is authorized for the claimed `evaluator_type` → `target_teacher_id` pair against the period's stored config (see `authorizeEvaluation` in `internal/handler/evaluation.go`) — the client can't just claim any type.
6. **A submission must answer every item in the form** (rejects partial submissions with a 400 listing missing item codes), and **re-submission overwrites** previous answers for that same task (upsert) rather than erroring — simplest way to let someone fix a mistake before the period ends, since the spec doesn't say submissions are one-shot/immutable.
7. **`final_score` is computed from whatever categories have submissions**, using each category's raw weight (not renormalized to 100% over just the completed categories) — so a partially-complete period shows a real but provisional score, and `is_complete` tells the caller whether it's final. This was chosen over blocking `/results` entirely until 100% completion, since "grafik/progress data" implies the endpoint should be usable mid-flight.
8. **Publish/unpublish is not hard-blocked outside Draft/Published** — the flag can technically be toggled anytime; it just has no visible effect once `Selesai` is reached (date always wins in `PeriodStatus`). Simpler than adding a second layer of state-transition guards on top of the date-driven derivation.
9. **RPM sections are freeform JSONB** (`section_a/b/c`) with the guru's own field structure — spec says "essentially free-form structured content," so no fixed schema was imposed beyond the outer 3-section split. RPM supports multiple entries per guru (list, not a single row) since real-world guru author RPM per lesson/topic repeatedly, and clicking a guru in the admin list is expected to show all of their entries.
10. **`academic_year_id` on a guru's own lesson-plan create** falls back to academic-service's "current year" lookup (`is_current: true`) if not supplied in the request body; if academic-service is unreachable and none was supplied, the request is rejected with 400 rather than silently writing a null/garbage year — same principle applied to the report's `school_name` (defaults to a placeholder string, or accepts `?school_name=` until identity exposes a school-detail route).
11. **Item weight is per-form-relative and re-normalized at scoring time** (see "Scoring math" above), not summed directly into the final 0–4 score — this cleanly absorbs a small rounding artifact in the source spreadsheet where Form A's 11 items sum to 19% instead of the displayed 20% (the form-level weight, which is what actually rolls up into `final_score`, is unaffected and the six form weights sum to exactly 100%).
12. **Multi-tenancy / academic-year scoping**: implemented exactly per CONVENTIONS.md — every table carries `school_id` + `academic_year_id`; every handler scopes to the caller's `claims.SchoolID` unless `active_role == superadmin` and `?school_id=` is given; period listing accepts `?academic_year_id=` to view other years.
