package handler

import (
	"database/sql"
	"net/http"

	"skci/hr/internal/client"
	"skci/hr/internal/config"
	"skci/hr/internal/crypto"
	"skci/hr/internal/storage"
)

type App struct {
	DB      *sql.DB
	Config  *config.Config
	Crypto  *crypto.Service
	Storage storage.Storage
	Client  *client.Client
}

func NewApp(db *sql.DB, cfg *config.Config, cip *crypto.Service, st storage.Storage, cli *client.Client) *App {
	return &App{
		DB:      db,
		Config:  cfg,
		Crypto:  cip,
		Storage: st,
		Client:  cli,
	}
}

func (a *App) Healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}
