#!/usr/bin/env bash
set -euo pipefail

echo "=== Seeding Teratai Databases on shared-postgres ==="

DIR="$(cd "$(dirname "$0")" && pwd)"

# 1. Identity
echo "--> Seeding teratai_identity (features, roles, white-label branding)..."
docker exec -i shared-postgres psql -U postgres -d teratai_identity < "$DIR/seed/01_identity_features.sql"

# 2. Academic
echo "--> Seeding teratai_academic (classrooms, subjects, enrollments)..."
docker exec -i shared-postgres psql -U postgres -d teratai_academic < "$DIR/seed/02_academic.sql"

# 3. Billing
echo "--> Seeding teratai_billing (schemes, items, student bills)..."
docker exec -i shared-postgres psql -U postgres -d teratai_billing < "$DIR/seed/03_billing.sql"

# 4. Ops
echo "--> Seeding teratai_ops (facilities, inventory, assets, library, transport, 3d)..."
docker exec -i shared-postgres psql -U postgres -d teratai_ops < "$DIR/seed/04_ops.sql"

# 5. Studentlife
echo "--> Seeding teratai_studentlife (medicines, counseling, discipline, extracurriculars, PKL)..."
docker exec -i shared-postgres psql -U postgres -d teratai_studentlife < "$DIR/seed/05_studentlife.sql"

# 6. Quality
echo "--> Seeding teratai_quality (instruments, standards, items, indicators, evidence)..."
docker exec -i shared-postgres psql -U postgres -d teratai_quality < "$DIR/seed/06_quality.sql"

# 7. Insight (metrics)
echo "--> Seeding teratai_insight (BI metric snapshots)..."
docker exec -i shared-postgres psql -U postgres -d teratai_insight < "$DIR/seed/07_insight.sql"

# 8. Academic Schedules
echo "--> Seeding teratai_academic schedules & materials..."
docker exec -i shared-postgres psql -U postgres -d teratai_academic < "$DIR/seed/08_academic_schedules.sql"

# 9. Attendance
echo "--> Seeding teratai_attendance (sessions, attendances, leave requests)..."
docker exec -i shared-postgres psql -U postgres -d teratai_attendance < "$DIR/seed/09_attendance.sql"

# 10. Exam (CBT)
echo "--> Seeding teratai_exam (types, folders, questions, exams, tokens)..."
docker exec -i shared-postgres psql -U postgres -d teratai_exam < "$DIR/seed/10_exam.sql"

# 11. Evaluation
echo "--> Seeding teratai_evaluation (lesson plans, prota, prosem, evaluations)..."
docker exec -i shared-postgres psql -U postgres -d teratai_evaluation < "$DIR/seed/11_evaluation.sql"

# 12. Report (E-Rapor)
echo "--> Seeding teratai_report (report cards, subject grades, formative)..."
docker exec -i shared-postgres psql -U postgres -d teratai_report < "$DIR/seed/12_report.sql"

# 13. HR
echo "--> Seeding teratai_hr (departments, employees, attendance, leave, training, jobs)..."
docker exec -i shared-postgres psql -U postgres -d teratai_hr < "$DIR/seed/14_hr.sql"
echo "--> Seeding teratai_hr features (payroll, ATS, pelatihan, cuti, lembur, mutasi, onboarding)..."
docker exec -i shared-postgres psql -U postgres -d teratai_hr -v ON_ERROR_STOP=1 < "$DIR/seed/50_hr_features.sql"

# 14. Notification & Chat
echo "--> Seeding teratai_notification (in-app notifs, chat threads, messages)..."
docker exec -i shared-postgres psql -U postgres -d teratai_notification < "$DIR/seed/15_notification.sql"

# 15. Core (Service Desk & Approvals)
echo "--> Seeding teratai_core (service desk, tickets, approvals, audit logs)..."
docker exec -i shared-postgres psql -U postgres -d teratai_core < "$DIR/seed/16_core.sql"

# 16. Insight Extra (Dashboards, Reports, Risk Predictions)
echo "--> Seeding teratai_insight extra (dashboards, widgets, risk predictions, help)..."
docker exec -i shared-postgres psql -U postgres -d teratai_insight < "$DIR/seed/17_insight_extra.sql"

echo "=== ALL SEED DATA SUCCESSFULLY APPLIED ==="
