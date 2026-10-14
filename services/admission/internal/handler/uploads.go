package handler

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"

	"skci/admission/internal/model"
)

// sniffUpload validates an uploaded file by extension whitelist and content
// (magic bytes) instead of trusting the client Content-Type (BUG-042). It
// returns the trusted MIME type and a reader that replays the sniffed bytes.
func sniffUpload(file multipart.File, header *multipart.FileHeader) (string, io.Reader, error) {
	head := make([]byte, 512)
	n, err := io.ReadFull(file, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", nil, &httpError{http.StatusBadRequest, "berkas tidak dapat dibaca"}
	}
	head = head[:n]
	mime, err := model.DetectUpload(header.Filename, head)
	if err != nil {
		return "", nil, &httpError{http.StatusUnprocessableEntity, err.Error()}
	}
	return mime, io.MultiReader(bytes.NewReader(head), file), nil
}
