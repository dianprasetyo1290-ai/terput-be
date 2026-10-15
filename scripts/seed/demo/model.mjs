// The demo school population, built in memory once and shared by every
// generator: units, classes, subjects, teachers, students and their parents.
// Deterministic (see lib.mjs), so every generator sees the same people.
import { STUDENTS_PER_CLASS, UNITS, address, person, phone, place, religion, rng, slug, uid } from './lib.mjs'

// Subjects per level: [name, kelompok, hours/week].
const SUBJECTS = {
  SD: [['Pendidikan Agama dan Budi Pekerti', 'umum', 4], ['Pendidikan Pancasila', 'umum', 4], ['Bahasa Indonesia', 'umum', 6],
    ['Matematika', 'umum', 5], ['IPAS', 'umum', 4], ['Seni Budaya', 'umum', 2], ['PJOK', 'umum', 3], ['Bahasa Inggris', 'muatan_lokal', 2],
    ['Bahasa Sunda', 'muatan_lokal', 2]],
  // The first five exist already (ids below); the rest are added.
  SMP: [['Matematika', 'umum', 5, 'c2000000-0000-0000-0000-000000000001'], ['Bahasa Indonesia', 'umum', 5, 'c2000000-0000-0000-0000-000000000002'],
    ['Bahasa Inggris', 'umum', 4, '6ba9625f-4003-4bc5-baa9-64250ff0f76a'], ['Ilmu Pengetahuan Alam (IPA)', 'umum', 5, '42b39317-f489-4f7a-8db5-ba79768f8f78'],
    ['Informatika', 'umum', 2, 'bf033168-3200-479e-9679-9ad1a6e48524'], ['Ilmu Pengetahuan Sosial (IPS)', 'umum', 4],
    ['Pendidikan Pancasila', 'umum', 3], ['Pendidikan Agama dan Budi Pekerti', 'umum', 3], ['PJOK', 'umum', 3], ['Seni Budaya', 'umum', 2],
    ['Prakarya', 'muatan_lokal', 2]],
  SMK: [['Pendidikan Agama dan Budi Pekerti', 'umum', 3], ['Pendidikan Pancasila', 'umum', 2], ['Bahasa Indonesia', 'umum', 3],
    ['Matematika', 'umum', 4], ['Bahasa Inggris', 'umum', 3], ['PJOK', 'umum', 2], ['Informatika', 'umum', 3],
    ['Dasar-dasar Teknik Jaringan Komputer', 'kejuruan', 8, null, 'TKJ'], ['Administrasi Infrastruktur Jaringan', 'kejuruan', 6, null, 'TKJ'],
    ['Dasar-dasar Akuntansi dan Keuangan Lembaga', 'kejuruan', 8, null, 'AKL'], ['Akuntansi Keuangan', 'kejuruan', 6, null, 'AKL'],
    ['Projek Kreatif dan Kewirausahaan', 'kejuruan', 4]],
}

// Teachers already in the SMP unit (merged demo accounts).
const SMP_EXISTING_TEACHERS = [
  { id: '55555555-5555-5555-5555-555555555555', name: 'Guru Teratai', existing: true },
  { id: '66666666-6666-6666-6666-666666666666', name: 'Wali Kelas Teratai', existing: true },
]
const BUDI = { id: '44444444-4444-4444-4444-444444444444', parentId: '33333333-3333-3333-3333-333333333333' }

const NEW_TEACHERS_PER_UNIT = 10

function build() {
  const units = []
  let nisn = 3120000000
  for (const u of UNITS) {
    const r = rng('people:' + u.key)
    const unit = { ...u, teachers: [], subjects: [], classes: [], students: [], parents: [] }

    // Teachers.
    if (u.key === 'smp') unit.teachers.push(...SMP_EXISTING_TEACHERS)
    for (let i = 0; i < NEW_TEACHERS_PER_UNIT; i++) {
      const p = person(r)
      const title = p.gender === 'L' ? 'Pak' : 'Bu'
      unit.teachers.push({
        id: uid('teacher', u.key, i), name: p.name, gender: p.gender, title,
        email: `${slug(p.name)}.${u.key}${i}@guru.teratai.sch.id`,
        phone: phone(r), birthPlace: place(r), birthDate: `19${r.int(70, 96)}-${String(r.int(1, 12)).padStart(2, '0')}-${String(r.int(1, 28)).padStart(2, '0')}`,
        religion: religion(r), address: address(r), nik: `3275${String(r.int(10000000, 99999999))}${String(r.int(1000, 9999))}`,
      })
    }

    // Subjects, each with a teacher.
    SUBJECTS[u.level].forEach(([name, kelompok, hours, existingId, major], i) => {
      unit.subjects.push({ id: existingId ?? uid('subject', u.key, name), name, kelompok, hours, major: major ?? null,
        existing: !!existingId, teacher: unit.teachers[i % unit.teachers.length] })
    })

    // Classes with a homeroom teacher (walikelas) and students.
    u.classes.forEach(([name, grade, existingId], ci) => {
      const homeroom = unit.teachers[(ci + (u.key === 'smp' ? 1 : 0)) % unit.teachers.length]
      const cls = { id: existingId ?? uid('class', u.key, name), name, grade, existing: !!existingId, homeroom,
        major: name.includes('TKJ') ? 'TKJ' : name.includes('AKL') ? 'AKL' : null, students: [] }
      unit.classes.push(cls)
      for (let si = 0; si < STUDENTS_PER_CLASS; si++) {
        if (u.key === 'smp' && ci === 0 && si === 0) {
          // Budi Santoso, the demo student account, sits in Kelas VII A.
          cls.students.push({ id: BUDI.id, name: 'Budi Santoso', existing: true, classId: cls.id, parentId: BUDI.parentId })
          continue
        }
        // Every 7th student is the sibling of the previous one (same parent):
        // shows a parent with two children in the Parent Portal.
        const prev = cls.students[cls.students.length - 1]
        const sibling = si % 7 === 6 && prev && !prev.existing
        const p = person(r, sibling ? { last: prev.last } : {})
        const age = { SD: 6, SMP: 12, SMK: 15 }[u.level] + Number(grade) - ({ SD: 1, SMP: 7, SMK: 10 }[u.level])
        const st = {
          id: uid('student', u.key, ci, si), name: p.name, last: p.last, gender: p.gender, classId: cls.id,
          email: `${slug(p.name)}.${u.key}${ci}${String(si).padStart(2, '0')}@siswa.teratai.sch.id`,
          nisn: String(nisn++), birthPlace: place(r), birthDate: `${2026 - age}-${String(r.int(1, 12)).padStart(2, '0')}-${String(r.int(1, 28)).padStart(2, '0')}`,
          religion: religion(r), address: address(r),
        }
        if (sibling) {
          st.parentId = prev.parentId
          st.address = prev.address
          st.religion = prev.religion
        } else {
          const pg = r.chance(0.55) ? 'P' : 'L'
          const pp = person(r, { gender: pg, last: p.last })
          const parent = {
            id: uid('parent', u.key, ci, si), name: pp.name, gender: pg, relationship: pg === 'L' ? 'ayah' : 'ibu',
            email: `${slug(pp.name)}.${u.key}${ci}${String(si).padStart(2, '0')}@ortu.teratai.sch.id`, phone: phone(r),
            address: st.address, religion: st.religion, nik: `3275${String(r.int(10000000, 99999999))}${String(r.int(1000, 9999))}`,
          }
          unit.parents.push(parent)
          st.parentId = parent.id
        }
        cls.students.push(st)
      }
      unit.students.push(...cls.students)
    })
    units.push(unit)
  }
  return units
}

export const MODEL = build()
export const unitByKey = (k) => MODEL.find((u) => u.key === k)
