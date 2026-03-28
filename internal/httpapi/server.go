package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/tomasz/srne-inverter-to-mqtt/internal/buildinfo"
	"github.com/tomasz/srne-inverter-to-mqtt/internal/config"
	"github.com/tomasz/srne-inverter-to-mqtt/internal/serialdetect"
	"github.com/tomasz/srne-inverter-to-mqtt/internal/state"
)

type ConfigStore interface {
	GetConfig() config.Config
	UpdateConfig(config.Config) error
}

type StatusProvider interface {
	GetStateSnapshot() state.Snapshot
}

type RegisterWriter interface {
	WriteRegister(id string, value any) error
}

type StatusSnapshot struct {
	StartedAt   time.Time `json:"startedAt"`
	ConfigPath  string    `json:"configPath"`
	ConfigReady bool      `json:"configReady"`
}

type Handler struct {
	build  buildinfo.Info
	state  StatusSnapshot
	store  ConfigStore
	status StatusProvider
	writer RegisterWriter
	assets fs.FS
}

func NewHandler(build buildinfo.Info, state StatusSnapshot, store ConfigStore, status StatusProvider, writer RegisterWriter, assets fs.FS) *Handler {
	return &Handler{
		build:  build,
		state:  state,
		store:  store,
		status: status,
		writer: writer,
		assets: assets,
	}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.handleHealth)
	mux.HandleFunc("GET /api/v1/status", h.handleStatus)
	mux.HandleFunc("GET /api/v1/config", h.handleGetConfig)
	mux.HandleFunc("PUT /api/v1/config", h.handleUpdateConfig)
	mux.HandleFunc("GET /api/v1/serial/ports", h.handleSerialPorts)
	mux.HandleFunc("POST /api/v1/registers/{id}/write", h.handleWriteRegister)
	mux.Handle("/", h.serveSPA())
	return h.withCommonHeaders(mux)
}

func (h *Handler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleStatus(w http.ResponseWriter, _ *http.Request) {
	cfg := h.store.GetConfig()
	snapshot := h.status.GetStateSnapshot()

	writeJSON(w, http.StatusOK, map[string]any{
		"build": h.build,
		"runtime": map[string]any{
			"startedAt":   h.state.StartedAt.UTC(),
			"uptime":      time.Since(h.state.StartedAt).Round(time.Second).String(),
			"configPath":  h.state.ConfigPath,
			"configReady": h.state.ConfigReady,
		},
		"service": snapshot.Services,
		"device": map[string]any{
			"name":    cfg.Device.Name,
			"slaveId": cfg.Device.SlaveID,
			"port":    cfg.Serial.Port,
		},
		"telemetry": snapshot.Telemetry,
	})
}

func (h *Handler) handleGetConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.store.GetConfig())
}

func (h *Handler) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	var cfg config.Config
	if err := json.Unmarshal(body, &cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	if err := h.store.UpdateConfig(cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (h *Handler) handleSerialPorts(w http.ResponseWriter, _ *http.Request) {
	ports, err := serialdetect.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ports": ports})
}

func (h *Handler) handleWriteRegister(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	id := r.PathValue("id")
	if strings.TrimSpace(id) == "" {
		writeError(w, http.StatusBadRequest, "missing register id")
		return
	}

	var payload struct {
		Value any `json:"value"`
	}

	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	if payload.Value == nil {
		writeError(w, http.StatusBadRequest, "value is required")
		return
	}

	if err := h.writer.WriteRegister(id, payload.Value); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "written"})
}

func (h *Handler) serveSPA() http.Handler {
	fileServer := http.FileServerFS(h.assets)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/healthz") {
			http.NotFound(w, r)
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		file, err := h.assets.Open(path)
		if err == nil {
			_ = file.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		if !errors.Is(err, fs.ErrNotExist) {
			http.Error(w, "failed to serve asset", http.StatusInternalServerError)
			return
		}

		index, err := fs.ReadFile(h.assets, "index.html")
		if err != nil {
			http.Error(w, "missing index.html", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(index)
	})
}

func (h *Handler) withCommonHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{
		"error": message,
	})
}
