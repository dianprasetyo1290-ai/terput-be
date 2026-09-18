// Package docx renders a .docx mail-merge template: an admin-uploaded Word
// file whose text carries {{placeholder}} markers, filled with one report
// card's data and handed back as a .docx.
//
// This is the upgrade path report_template.go's doc comment named: page-1
// rapor layout stops being hardcoded Vue and becomes a file the school
// owns. A school that uploads nothing keeps the built-in print view, so
// this is purely additive.
//
// A .docx is a zip of XML parts; the text lives in word/document.xml plus
// any headers/footers. Nothing here parses the full OOXML schema -- it
// rewrites text nodes and duplicates table rows, which is all a rapor
// needs, and leaves every other byte of the document (styles, images, the
// school's own kop) exactly as Word wrote it.
//
// Syntax:
//
//	{{nama_siswa}}                     one value
//	{{#mapel}} ... {{/mapel}}          repeat the table row(s) in between,
//	                                   once per item, with that item's own
//	                                   fields in scope
//
// A loop's open and close markers may sit in the same table row (the
// common case: one row per mata pelajaran) or in different rows, in which
// case the whole span repeats.
package docx

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"fmt"
	"hash/crc32"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// Data is one rendering's inputs. Loops[name] supplies the rows for a
// {{#name}}...{{/name}} block; inside such a block an item's own keys win,
// falling back to Scalars so a loop row can still print e.g. the student
// name.
type Data struct {
	Scalars map[string]string
	Loops   map[string][]map[string]string
}

// mergeableParts are the document parts that carry visible text. Headers
// and footers are included because a rapor's kop and page numbering
// routinely live there.
func isMergeablePart(name string) bool {
	if name == "word/document.xml" {
		return true
	}
	return strings.HasPrefix(name, "word/header") || strings.HasPrefix(name, "word/footer")
}

// Render fills tmpl (raw .docx bytes) with d and returns the rendered
// .docx. The template is never modified.
func Render(tmpl []byte, d Data) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(tmpl), int64(len(tmpl)))
	if err != nil {
		return nil, fmt.Errorf("file bukan .docx yang bisa dibaca: %w", err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		if isMergeablePart(f.Name) {
			content = []byte(renderPart(string(content), d))
		}
		if err := writeEntry(zw, f, content); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeEntry appends one part to the output zip with its size and CRC in the
// local file header.
//
// zip.Writer.Create/CreateHeader always streams: it sets the
// data-descriptor flag and writes the CRC and sizes *after* the data. That
// is valid zip, and Go's own reader accepts it, but Word and LibreOffice
// both reject a .docx built that way outright ("source file could not be
// loaded") -- which is why the synthetic tests in this package passed while
// a real school's template came back unopenable. CreateRaw lets us compress
// first and fill the header in, exactly as Word writes it.
func writeEntry(zw *zip.Writer, f *zip.File, content []byte) error {
	var body bytes.Buffer
	method := f.Method
	if method == zip.Deflate {
		fw, err := flate.NewWriter(&body, flate.DefaultCompression)
		if err != nil {
			return err
		}
		if _, err := fw.Write(content); err != nil {
			return err
		}
		if err := fw.Close(); err != nil {
			return err
		}
	} else {
		method = zip.Store
		body.Write(content)
	}
	w, err := zw.CreateRaw(&zip.FileHeader{
		Name:               f.Name,
		Method:             method,
		Modified:           f.Modified,
		CRC32:              crc32.ChecksumIEEE(content),
		CompressedSize64:   uint64(body.Len()),
		UncompressedSize64: uint64(len(content)),
	})
	if err != nil {
		return err
	}
	_, err = w.Write(body.Bytes())
	return err
}

func renderPart(xml string, d Data) string {
	xml = healPlaceholders(xml)
	xml = expandLoops(xml, d)
	return substitute(xml, d.Scalars)
}

var (
	reParagraph = regexp.MustCompile(`(?s)<w:p[ >].*?</w:p>`)
	reTextNode  = regexp.MustCompile(`(?s)<w:t(?:\s[^>]*)?>.*?</w:t>`)
	rePlacehold = regexp.MustCompile(`\{\{[#/]?[a-zA-Z0-9_]+\}\}`)
	reTableRow  = regexp.MustCompile(`(?s)<w:tr[ >].*?</w:tr>`)
)

// textOf returns the character data inside a <w:t> node.
func textOf(node string) string {
	i := strings.Index(node, ">")
	j := strings.LastIndex(node, "<")
	if i < 0 || j < i {
		return ""
	}
	return unescapeXML(node[i+1 : j])
}

// withText rewrites a <w:t> node's character data, forcing
// xml:space="preserve" so leading/trailing spaces in merged values aren't
// silently dropped by Word.
func withText(node, text string) string {
	i := strings.Index(node, ">")
	if i < 0 {
		return node
	}
	open := node[:i]
	if !strings.Contains(open, "xml:space") {
		open += ` xml:space="preserve"`
	}
	return open + ">" + escapeXML(text) + "</w:t>"
}

func escapeXML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

// unescapeXML is escapeXML's inverse, applied when reading a text node back
// out so healing a split placeholder doesn't double-escape text that was
// already entity-encoded ("SD &amp; SMP" must not become "SD &amp;amp; SMP").
func unescapeXML(s string) string {
	r := strings.NewReplacer("&lt;", "<", "&gt;", ">", "&quot;", `"`, "&apos;", "'", "&amp;", "&")
	return r.Replace(s)
}

// healPlaceholders repairs placeholders that Word split across runs.
//
// Word freely breaks a typed "{{nama_siswa}}" into several <w:r>/<w:t>
// nodes (spellcheck, rsid tracking, an autocorrect mid-word), so the
// literal string is often not present in the XML at all -- the single most
// common reason a naive docx merge silently outputs nothing. Per
// paragraph, the text nodes are concatenated; any placeholder that spans
// more than one node is moved wholly into the first node it touches and
// erased from the rest. Nodes not touched by a split placeholder are left
// byte-for-byte alone, so the template's own formatting survives.
func healPlaceholders(xml string) string {
	return reParagraph.ReplaceAllStringFunc(xml, func(p string) string {
		nodes := reTextNode.FindAllStringIndex(p, -1)
		if len(nodes) < 2 {
			return p
		}
		texts := make([]string, len(nodes))
		var sb strings.Builder
		// offsets[i] = index in the concatenated string where node i starts
		offsets := make([]int, len(nodes))
		for i, loc := range nodes {
			offsets[i] = sb.Len()
			texts[i] = textOf(p[loc[0]:loc[1]])
			sb.WriteString(texts[i])
		}
		joined := sb.String()
		spans := rePlacehold.FindAllStringIndex(joined, -1)
		if len(spans) == 0 {
			return p
		}

		// Re-cut the paragraph's text across its existing nodes so that no
		// node boundary falls inside a placeholder. Each node keeps its own
		// slice of the text (and therefore its own formatting); only a cut
		// that would split "{{...}}" gets pushed forward to the end of that
		// placeholder, which pulls the whole marker into the earlier node.
		cuts := make([]int, len(nodes))
		for i := range nodes {
			if i+1 < len(nodes) {
				cuts[i] = offsets[i+1]
			} else {
				cuts[i] = len(joined)
			}
			for _, sp := range spans {
				if sp[0] < cuts[i] && cuts[i] < sp[1] {
					cuts[i] = sp[1]
					break
				}
			}
		}
		newTexts := make([]string, len(nodes))
		pos, changed := 0, false
		for i := range nodes {
			if cuts[i] < pos {
				cuts[i] = pos
			}
			newTexts[i] = joined[pos:cuts[i]]
			if newTexts[i] != texts[i] {
				changed = true
			}
			pos = cuts[i]
		}
		if !changed {
			return p
		}
		// Rebuild the paragraph, replacing node contents back-to-front so
		// earlier indexes stay valid.
		out := p
		for i := len(nodes) - 1; i >= 0; i-- {
			loc := nodes[i]
			out = out[:loc[0]] + withText(p[loc[0]:loc[1]], newTexts[i]) + out[loc[1]:]
		}
		return out
	})
}

// expandLoops replaces every {{#name}}...{{/name}} block with one copy of
// the block per item in Loops[name]. A loop whose name has no data (or an
// empty list) collapses to nothing, which is the right output for a rapor
// section the school didn't fill in -- an empty table with just its header.
func expandLoops(xml string, d Data) string {
	for {
		name, openIdx := "", -1
		// Find the first opening marker anywhere in the document.
		for _, loc := range rePlacehold.FindAllStringIndex(xml, -1) {
			m := xml[loc[0]:loc[1]]
			if strings.HasPrefix(m, "{{#") {
				name = m[3 : len(m)-2]
				openIdx = loc[0]
				break
			}
		}
		if openIdx < 0 {
			return xml
		}
		closeMark := "{{/" + name + "}}"
		closeIdx := strings.Index(xml[openIdx:], closeMark)
		if closeIdx < 0 {
			// Unbalanced marker: strip it so it can't print into the rapor
			// and keep going rather than failing the whole render.
			xml = strings.Replace(xml, "{{#"+name+"}}", "", 1)
			continue
		}
		closeIdx += openIdx

		blockStart, blockEnd := rowSpan(xml, openIdx, closeIdx+len(closeMark))
		block := xml[blockStart:blockEnd]
		block = strings.Replace(block, "{{#"+name+"}}", "", 1)
		block = strings.Replace(block, closeMark, "", 1)

		var sb strings.Builder
		for i, item := range d.Loops[name] {
			vals := make(map[string]string, len(item)+len(d.Scalars)+1)
			for k, v := range d.Scalars {
				vals[k] = v
			}
			for k, v := range item {
				vals[k] = v
			}
			if _, ok := vals["no"]; !ok {
				vals["no"] = strconv.Itoa(i + 1)
			}
			sb.WriteString(substitute(block, vals))
		}
		xml = xml[:blockStart] + sb.String() + xml[blockEnd:]
	}
}

// rowSpan widens [from,to) to whole <w:tr> elements when the markers sit
// inside a table, so repeating a loop repeats entire rows rather than
// slicing a row in half. Outside a table it widens to whole <w:p>
// paragraphs. Falls back to the raw span if neither encloses it.
func rowSpan(xml string, from, to int) (int, int) {
	for _, re := range []*regexp.Regexp{reTableRow, reParagraph} {
		var start, end = -1, -1
		for _, loc := range re.FindAllStringIndex(xml, -1) {
			if loc[0] <= from && from < loc[1] {
				start = loc[0]
			}
			if loc[0] < to && to <= loc[1] {
				end = loc[1]
			}
		}
		if start >= 0 && end >= start {
			return start, end
		}
	}
	return from, to
}

// substitute replaces every {{key}} with its value. An unknown placeholder
// is blanked rather than left on the page: a printed rapor showing a raw
// {{nisn}} to a parent is worse output than a blank.
func substitute(xml string, vals map[string]string) string {
	return rePlacehold.ReplaceAllStringFunc(xml, func(m string) string {
		if strings.HasPrefix(m, "{{#") || strings.HasPrefix(m, "{{/") {
			return m
		}
		return escapeXML(vals[m[2:len(m)-2]])
	})
}
