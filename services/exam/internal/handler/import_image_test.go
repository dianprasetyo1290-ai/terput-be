package handler

import (
	"bytes"
	"encoding/base64"
	_ "image/png"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// TestParseXLSXExtractsFloatingImageIntoRow guards against the "pasted
// screenshot doesn't show up" import gap (Report sekoci: math questions
// pasted via Snipping Tool into a cell, since Excel won't accept an image as
// cell text). A floating picture anchored in a data row must become that
// question's image_url as a data: URI.
func TestParseXLSXExtractsFloatingImageIntoRow(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	sheet := f.GetSheetName(0)
	header := []string{"no_kd", "konten_kd", "indikator_pencapaian", "level_kognitif", "poin", "content", "option_a", "option_b", "option_c", "option_d", "option_e", "correct_option"}
	for i, h := range header {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}
	data := []string{"3.1", "Matriks", "Siswa dapat menentukan determinan", "C3", "10", "Perhatikan matriks berikut (lihat gambar):", "5", "6", "7", "8", "9", "a"}
	for i, v := range data {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		_ = f.SetCellValue(sheet, cell, v)
	}

	// 1x1 transparent PNG, anchored near the "content" cell (F2) like a
	// pasted screenshot would be.
	pngBytes, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err := f.AddPictureFromBytes(sheet, "F2", &excelize.Picture{Extension: ".png", File: pngBytes}); err != nil {
		t.Fatalf("test setup: AddPictureFromBytes: %v", err)
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("test setup: write xlsx: %v", err)
	}

	records, rowImages, err := parseXLSX(&buf)
	if err != nil {
		t.Fatalf("parseXLSX: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("want 2 records (header + 1 row), got %d", len(records))
	}

	img, ok := rowImages[1]
	if !ok {
		t.Fatal("expected an extracted image for row index 1 (the first data row), got none")
	}
	if !strings.HasPrefix(img, "data:image/png;base64,") {
		t.Fatalf("unexpected image data URI prefix: %.40s", img)
	}

	idx := map[string]int{}
	for i, col := range header {
		idx[col] = i
	}
	req, msg := rowToQuestionReq("pilihan_ganda", records[1], idx, "ay-1", img)
	if msg != "" {
		t.Fatalf("rowToQuestionReq returned an error: %s", msg)
	}
	if req.ImageURL == nil || *req.ImageURL != img {
		t.Fatal("expected the question's ImageURL to be populated from the extracted picture")
	}
}
