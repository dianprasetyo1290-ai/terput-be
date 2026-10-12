package handler

import (
	"context"
	"errors"

	"skci/notification/internal/client"
)

// fakeRelations is an in-memory Relations (identity + academic fake).
type fakeRelations struct {
	roles           map[string][]string        // userID -> roles
	teacherStudents map[string]map[string]bool // teacherID -> student set
	guardianOf      map[string][]string        // guardianID -> children
	studentTeachers map[string][]client.StudentTeacher
	heads           []client.UserProfile
	fail            bool
}

func newFakeRelations() *fakeRelations {
	return &fakeRelations{
		roles: map[string][]string{}, teacherStudents: map[string]map[string]bool{},
		guardianOf: map[string][]string{}, studentTeachers: map[string][]client.StudentTeacher{},
	}
}

var errFake = errors.New("downstream unavailable")

func (f *fakeRelations) UserRoles(_ context.Context, _, userID string) ([]string, error) {
	if f.fail {
		return nil, errFake
	}
	return f.roles[userID], nil
}

func (f *fakeRelations) UserName(_ context.Context, userID string) string { return "Nama " + userID[:4] }

func (f *fakeRelations) TeacherStudentIDs(_ context.Context, _, teacherID string) (map[string]bool, error) {
	if f.fail {
		return nil, errFake
	}
	if s := f.teacherStudents[teacherID]; s != nil {
		return s, nil
	}
	return map[string]bool{}, nil
}

func (f *fakeRelations) GuardianHasAccess(_ context.Context, _, guardianID, studentID string) (bool, error) {
	if f.fail {
		return false, errFake
	}
	for _, c := range f.guardianOf[guardianID] {
		if c == studentID {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeRelations) GuardianChildren(_ context.Context, _, guardianID string) ([]string, error) {
	if f.fail {
		return nil, errFake
	}
	return f.guardianOf[guardianID], nil
}

func (f *fakeRelations) StudentGuardians(_ context.Context, _, studentID string) ([]client.StudentGuardian, error) {
	if f.fail {
		return nil, errFake
	}
	var out []client.StudentGuardian
	for g, children := range f.guardianOf {
		for _, c := range children {
			if c == studentID {
				out = append(out, client.StudentGuardian{GuardianUserID: g, Relationship: "ayah", IsPrimary: true})
			}
		}
	}
	return out, nil
}

func (f *fakeRelations) StudentTeachers(_ context.Context, _, studentID string) ([]client.StudentTeacher, error) {
	if f.fail {
		return nil, errFake
	}
	return f.studentTeachers[studentID], nil
}

func (f *fakeRelations) UsersByRole(_ context.Context, _, role string) ([]client.UserProfile, error) {
	if role == roleKepalaSekolah {
		return f.heads, nil
	}
	return nil, nil
}
