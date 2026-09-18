package docx

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"strings"
	"testing"
)

// run wraps text in the <w:r><w:t> nesting Word actually emits.
func run(text string) string {
	return `<w:r><w:rPr><w:b/></w:rPr><w:t>` + text + `</w:t></w:r>`
}

func para(runs ...string) string {
	return `<w:p w:rsidR="00A">` + strings.Join(runs, "") + `</w:p>`
}

func row(cells ...string) string {
	var sb strings.Builder
	sb.WriteString(`<w:tr w:rsidR="00B">`)
	for _, c := range cells {
		sb.WriteString(`<w:tc>` + para(run(c)) + `</w:tc>`)
	}
	sb.WriteString(`</w:tr>`)
	return sb.String()
}

func docFrom(body string) []byte {
	xml := `<?xml version="1.0"?><w:document xmlns:w="x"><w:body>` + body + `</w:body></w:document>`
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("word/document.xml")
	w.Write([]byte(xml))
	// A part that must survive untouched, byte for byte.
	w2, _ := zw.Create("word/media/image1.png")
	w2.Write([]byte("\x89PNG-not-really"))
	zw.Close()
	return buf.Bytes()
}

func docText(t *testing.T, b []byte) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatalf("output is not a readable zip: %v", err)
	}
	for _, f := range zr.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		rc.Close()
		return string(data)
	}
	t.Fatal("output has no word/document.xml")
	return ""
}

// visible strips the XML tags so a test can assert on what a reader of the
// printed rapor would actually see.
func visible(xml string) string {
	var sb strings.Builder
	for {
		i := strings.Index(xml, "<w:t")
		if i < 0 {
			break
		}
		j := strings.Index(xml[i:], ">")
		k := strings.Index(xml[i:], "</w:t>")
		if j < 0 || k < 0 {
			break
		}
		sb.WriteString(xml[i+j+1 : i+k])
		sb.WriteString("|")
		xml = xml[i+k+6:]
	}
	return sb.String()
}

func TestRenderScalars(t *testing.T) {
	doc := docFrom(para(run("Nama: {{nama_siswa}}"), run(" NISN: {{nisn}}")))
	out, err := Render(doc, Data{Scalars: map[string]string{
		"nama_siswa": "Rizky Akbar",
		"nisn":       "0051234567",
	}})
	if err != nil {
		t.Fatal(err)
	}
	got := visible(docText(t, out))
	if !strings.Contains(got, "Nama: Rizky Akbar") || !strings.Contains(got, "NISN: 0051234567") {
		t.Errorf("got %q", got)
	}
}

// The regression this whole package hinges on: Word splits a typed
// placeholder across runs, so the literal "{{nama_siswa}}" is nowhere in
// the XML. A merge that only does strings.Replace outputs nothing here.
func TestRenderHealsPlaceholderSplitAcrossRuns(t *testing.T) {
	doc := docFrom(para(run("Nama: {{nam"), run("a_sis"), run("wa}}")))
	if strings.Contains(string(doc), "{{nama_siswa}}") {
		t.Fatal("test setup is wrong -- the placeholder must start out split")
	}
	out, err := Render(doc, Data{Scalars: map[string]string{"nama_siswa": "Rizky Akbar"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := visible(docText(t, out)); !strings.Contains(got, "Nama: Rizky Akbar") {
		t.Errorf("split placeholder not merged: got %q", got)
	}
}

func TestRenderKeepsSurroundingRunFormatting(t *testing.T) {
	// Two placeholders in one paragraph, each split, with literal text
	// between them that must stay put.
	doc := docFrom(para(run("{{kel"), run("as}} semester "), run("{{seme"), run("ster}}")))
	out, err := Render(doc, Data{Scalars: map[string]string{"kelas": "XII DKV", "semester": "Genap"}})
	if err != nil {
		t.Fatal(err)
	}
	got := visible(docText(t, out))
	if !strings.Contains(got, "XII DKV") || !strings.Contains(got, "semester ") || !strings.Contains(got, "Genap") {
		t.Errorf("got %q", got)
	}
	// Formatting runs must not have been collapsed into one.
	if n := strings.Count(docText(t, out), "<w:rPr>"); n != 4 {
		t.Errorf("expected 4 runs preserved, got %d", n)
	}
}

func TestRenderLoopRepeatsTableRow(t *testing.T) {
	body := row("NO", "MATA PELAJARAN", "NILAI") +
		row("{{#mapel}}{{no}}", "{{nama}}", "{{nilai_akhir}}{{/mapel}}")
	out, err := Render(docFrom(body), Data{
		Scalars: map[string]string{"nama_siswa": "Rizky"},
		Loops: map[string][]map[string]string{"mapel": {
			{"nama": "Pendidikan Agama Islam", "nilai_akhir": "88"},
			{"nama": "Pendidikan Pancasila", "nilai_akhir": "85"},
			{"nama": "Bahasa Indonesia", "nilai_akhir": "90"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	xml := docText(t, out)
	if n := strings.Count(xml, "<w:tr"); n != 4 {
		t.Errorf("expected header + 3 mapel rows = 4, got %d", n)
	}
	got := visible(xml)
	for _, want := range []string{"1|", "Pendidikan Agama Islam", "88", "3|", "Bahasa Indonesia", "90"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	// Row order must follow the data, not the template.
	if strings.Index(got, "Pendidikan Agama Islam") > strings.Index(got, "Bahasa Indonesia") {
		t.Error("mapel rows came out in the wrong order")
	}
}

func TestRenderEmptyLoopRemovesRow(t *testing.T) {
	body := row("No", "Kegiatan") + row("{{#ekskul}}{{no}}", "{{kegiatan}}{{/ekskul}}")
	out, err := Render(docFrom(body), Data{Loops: map[string][]map[string]string{"ekskul": {}}})
	if err != nil {
		t.Fatal(err)
	}
	xml := docText(t, out)
	if n := strings.Count(xml, "<w:tr"); n != 1 {
		t.Errorf("empty loop should leave only the header row, got %d rows", n)
	}
	if strings.Contains(xml, "{{") {
		t.Errorf("loop markers left in output: %s", xml)
	}
}

func TestRenderUnknownPlaceholderBlanked(t *testing.T) {
	out, err := Render(docFrom(para(run("Fase: {{fase}} / {{tidak_ada}}"))), Data{
		Scalars: map[string]string{"fase": "F"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := visible(docText(t, out))
	if strings.Contains(got, "{{") {
		t.Errorf("a raw placeholder reached the printed rapor: %q", got)
	}
	if !strings.Contains(got, "Fase: F / ") {
		t.Errorf("got %q", got)
	}
}

func TestRenderEscapesValuesAndPreservesEntities(t *testing.T) {
	doc := docFrom(para(run("SD &amp; SMP {{nama_sekolah}}")))
	out, err := Render(doc, Data{Scalars: map[string]string{"nama_sekolah": `A & B <"x">`}})
	if err != nil {
		t.Fatal(err)
	}
	xml := docText(t, out)
	if strings.Contains(xml, "&amp;amp;") {
		t.Error("existing entity got double-escaped")
	}
	if strings.Contains(xml, `<"x">`) {
		t.Error("value was injected unescaped -- would corrupt the document")
	}
	if !strings.Contains(xml, "A &amp; B") {
		t.Errorf("value not escaped as expected: %s", xml)
	}
}

func TestRenderLeavesOtherPartsUntouched(t *testing.T) {
	out, err := Render(docFrom(para(run("{{nama_siswa}}"))), Data{
		Scalars: map[string]string{"nama_siswa": "X"},
	})
	if err != nil {
		t.Fatal(err)
	}
	zr, _ := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	var found bool
	for _, f := range zr.File {
		if f.Name != "word/media/image1.png" {
			continue
		}
		found = true
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		rc.Close()
		if string(data) != "\x89PNG-not-really" {
			t.Error("a non-text part was modified")
		}
	}
	if !found {
		t.Error("a non-text part was dropped from the output")
	}
}

func TestRenderRejectsNonDocx(t *testing.T) {
	if _, err := Render([]byte("ini bukan docx"), Data{}); err == nil {
		t.Error("expected an error for a non-.docx upload")
	}
}

// TestRenderRealWordFile runs the client's own rapor template
// (testdata/rapor_gpm_template.docx -- the SMK GPM "Template 1 Lembar"
// form, written by Word itself, with its kop image, identity table,
// 6-column intrakulikuler table looped per kelompok, ekskul table and
// signature block). Word nests runs, splits text, and carries
// styles/relationships that hand-built XML doesn't, so this is the check
// that the merge survives a file an actual school uploads.
func TestRenderRealWordFile(t *testing.T) {
	tmpl, err := os.ReadFile("testdata/rapor_gpm_template.docx")
	if err != nil {
		t.Skipf("testdata missing: %v", err)
	}
	out, err := Render(tmpl, Data{
		Scalars: map[string]string{
			"nama_siswa": "Rizky Akbar Pratama", "nisn": "0051234567",
			"kelas": "XII DKV 1", "semester": "Ganjil",
			"nama_sekolah": "SMK GLOBAL PERSADA MANDIRI",
			"jumlah_nilai": "784", "rata_rata": "87.11",
			"kepala_sekolah": "Tiorida Ervina, S.Pd.", "wali_kelas": "Dewi Lestari, S.Kom.",
		},
		Loops: map[string][]map[string]string{
			"mapel_a": {
				{"no_urut": "1", "nama": "Pendidikan Agama dan Budi Pekerti", "nilai_akhir": "88"},
				{"no_urut": "2", "nama": "Pendidikan Pancasila", "nilai_akhir": "85"},
				{"no_urut": "3", "nama": "Bahasa Indonesia", "nilai_akhir": "90"},
			},
			"mapel_b": {{"no_urut": "4", "nama": "Bahasa Jepang", "nilai_akhir": "80"}},
			"mapel_c": {{"no_urut": "5", "nama": "Bahasa Sunda", "nilai_akhir": "84"}},
			"ekskul":  {{"kegiatan": "Futsal", "predikat": "A", "keterangan": "Sangat aktif"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	xml := docText(t, out)
	if strings.Contains(xml, "{{") {
		t.Error("a placeholder survived into the rendered rapor")
	}
	got := visible(xml)
	for _, want := range []string{
		"Rizky Akbar Pratama", "0051234567", "XII DKV 1", "87.11",
		"Pendidikan Agama dan Budi Pekerti", "Bahasa Indonesia", "Bahasa Sunda", "Futsal",
		"Tiorida Ervina, S.Pd.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in rendered output", want)
		}
	}
	// 4 identitas + (header + 3 kelompok headers + 5 mapel) + (header + 1
	// ekskul) + 4 tanda tangan = 19. Count </w:tr>, not "<w:tr": the real
	// file's rows carry <w:trPr>/<w:trHeight> children too.
	if n := strings.Count(xml, "</w:tr>"); n != 19 {
		t.Errorf("expected 19 table rows after expansion, got %d", n)
	}
	if strings.Index(got, "Pendidikan Agama") > strings.Index(got, "Bahasa Indonesia") {
		t.Error("mapel rows rendered out of order")
	}
}

// TestRenderNoDataDescriptors pins the container bug that shipped once:
// zip.Writer.Create streams every entry, setting the data-descriptor flag
// (bit 3) and writing CRC/sizes after the data. Go's own zip reader accepts
// that, so every test in this file passed -- but Word and LibreOffice
// refuse such a .docx outright ("source file could not be loaded"), so a
// real school's merged rapor could not be opened at all. Every local file
// header must carry its CRC and sizes up front.
func TestRenderNoDataDescriptors(t *testing.T) {
	tmpl, err := os.ReadFile("testdata/rapor_gpm_template.docx")
	if err != nil {
		t.Skipf("testdata missing: %v", err)
	}
	out, err := Render(tmpl, Data{})
	if err != nil {
		t.Fatal(err)
	}
	var n int
	for off := 0; off+30 <= len(out); {
		if string(out[off:off+4]) != "PK\x03\x04" {
			break
		}
		flags := binary.LittleEndian.Uint16(out[off+6:])
		crc := binary.LittleEndian.Uint32(out[off+14:])
		csize := binary.LittleEndian.Uint32(out[off+18:])
		name := binary.LittleEndian.Uint16(out[off+26:])
		extra := binary.LittleEndian.Uint16(out[off+28:])
		if flags&0x8 != 0 {
			t.Fatalf("entry %d uses a data descriptor -- Word cannot open this file", n)
		}
		if crc == 0 || csize == 0 {
			t.Fatalf("entry %d has no CRC/size in its local header", n)
		}
		n++
		off += 30 + int(name) + int(extra) + int(csize)
	}
	if n < 5 {
		t.Fatalf("only walked %d entries -- the scan is wrong, not the writer", n)
	}
}
