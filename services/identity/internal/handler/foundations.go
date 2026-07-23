package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"skci/identity/internal/common"
	"skci/identity/internal/model"
)

// Data Yayasan (Foundation) CRUD — superadmin only.

type foundationRequest struct {
	Name        string `json:"name"`
	Address     string `json:"address"`
	YearFounded *int   `json:"year_founded"`
	YearJoined  *int   `json:"year_joined"`
}

func (a *App) ListFoundations(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleSuperadmin) {
		return
	}
	page, perPage := common.ParsePage(r)
	var total int
	if err := a.DB.QueryRow(`SELECT count(*) FROM foundations WHERE deleted_at IS NULL`).Scan(&total); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list foundations")
		return
	}
	rows, err := a.DB.Query(`SELECT id, name, address, year_founded, year_joined, created_at, updated_at
		FROM foundations WHERE deleted_at IS NULL ORDER BY created_at DESC LIMIT $1 OFFSET $2`, perPage, (page-1)*perPage)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list foundations")
		return
	}
	defer rows.Close()
	out := []model.Foundation{}
	for rows.Next() {
		var f model.Foundation
		if err := rows.Scan(&f.ID, &f.Name, &f.Address, &f.YearFounded, &f.YearJoined, &f.CreatedAt, &f.UpdatedAt); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to list foundations")
			return
		}
		out = append(out, f)
	}
	common.WriteJSON(w, http.StatusOK, out, common.Meta{Page: page, PerPage: perPage, Total: total})
}

func (a *App) CreateFoundation(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleSuperadmin) {
		return
	}
	var req foundationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		common.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}
	var f model.Foundation
	err := a.DB.QueryRow(`INSERT INTO foundations (name, address, year_founded, year_joined)
		VALUES ($1, NULLIF($2,''), $3, $4) RETURNING id, name, address, year_founded, year_joined, created_at, updated_at`,
		req.Name, req.Address, req.YearFounded, req.YearJoined).
		Scan(&f.ID, &f.Name, &f.Address, &f.YearFounded, &f.YearJoined, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to create foundation")
		return
	}
	common.WriteJSON(w, http.StatusCreated, f, nil)
}

func (a *App) GetFoundation(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleSuperadmin) {
		return
	}
	id := chi.URLParam(r, "id")
	var f model.Foundation
	err := a.DB.QueryRow(`SELECT id, name, address, year_founded, year_joined, created_at, updated_at
		FROM foundations WHERE id = $1 AND deleted_at IS NULL`, id).
		Scan(&f.ID, &f.Name, &f.Address, &f.YearFounded, &f.YearJoined, &f.CreatedAt, &f.UpdatedAt)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "foundation not found")
		return
	} else if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to get foundation")
		return
	}
	common.WriteJSON(w, http.StatusOK, f, nil)
}

func (a *App) UpdateFoundation(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleSuperadmin) {
		return
	}
	id := chi.URLParam(r, "id")
	var req foundationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		common.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}
	var f model.Foundation
	err := a.DB.QueryRow(`UPDATE foundations SET name=$1, address=NULLIF($2,''), year_founded=$3, year_joined=$4, updated_at=now()
		WHERE id=$5 AND deleted_at IS NULL
		RETURNING id, name, address, year_founded, year_joined, created_at, updated_at`,
		req.Name, req.Address, req.YearFounded, req.YearJoined, id).
		Scan(&f.ID, &f.Name, &f.Address, &f.YearFounded, &f.YearJoined, &f.CreatedAt, &f.UpdatedAt)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "foundation not found")
		return
	} else if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to update foundation")
		return
	}
	common.WriteJSON(w, http.StatusOK, f, nil)
}

func (a *App) DeleteFoundation(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleSuperadmin) {
		return
	}
	id := chi.URLParam(r, "id")
	res, err := a.DB.Exec(`UPDATE foundations SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to delete foundation")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "foundation not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
}
