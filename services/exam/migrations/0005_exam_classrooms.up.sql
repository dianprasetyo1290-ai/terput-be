-- Bug fix: creating one exam for MULTIPLE classrooms (guru ticks several
-- "{Subject} - {Class}" checkboxes for the same subject in Buat Ujian Baru)
-- used to fan out into one exams row PER classroom, since exams only ever
-- carried a single classroom_id — duplicating the exam in Kelola Ujian and
-- splitting its question set. exam_classrooms is the real many-to-many: one
-- exam, several target classrooms, one shared question set.
--
-- exams.classroom_id is kept (still NOT NULL) as the "primary" classroom for
-- backward compat — report-service's upstream.ExamClient.Exam.ClassroomID
-- and the existing ?classroom_id= list filter both keep working unchanged.
CREATE TABLE exam_classrooms (
    exam_id       UUID NOT NULL REFERENCES exams(id) ON DELETE CASCADE,
    classroom_id  UUID NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (exam_id, classroom_id)
);
CREATE INDEX idx_exam_classrooms_classroom ON exam_classrooms(classroom_id);

-- Backfill: every existing exam already has exactly one classroom (the old
-- single-classroom-per-row behavior), so this is a lossless 1:1 copy.
INSERT INTO exam_classrooms (exam_id, classroom_id)
SELECT id, classroom_id FROM exams;
