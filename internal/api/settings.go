package api

import (
	"net/http"
	"strconv"

	"github.com/t0mer/fulcrum/internal/store"
)

func parseThreshold(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f <= 0 || f >= 1 {
		return 0
	}
	return f
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// settingsView is the effective settings (stored value or config fallback).
type settingsView struct {
	GlobalThreshold float64 `json:"global_threshold"`
	SinkMode        string  `json:"sink_mode"`
	Provider        string  `json:"provider"`
}

func (a *API) effectiveSettings() settingsView {
	v := settingsView{
		GlobalThreshold: a.defaultThreshold,
		SinkMode:        a.defaultSinkMode,
		Provider:        a.provName,
	}
	if s, ok, _ := a.store.GetSetting(store.SettingGlobalThreshold); ok {
		if f := parseThreshold(s); f > 0 {
			v.GlobalThreshold = f
		}
	}
	if s, ok, _ := a.store.GetSetting(store.SettingSinkMode); ok && validSinkMode(s) {
		v.SinkMode = s
	}
	return v
}

func (a *API) getSettings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.effectiveSettings())
}

type updateSettingsReq struct {
	GlobalThreshold *float64 `json:"global_threshold"`
	SinkMode        *string  `json:"sink_mode"`
}

func (a *API) updateSettings(w http.ResponseWriter, r *http.Request) {
	var req updateSettingsReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.GlobalThreshold != nil {
		if *req.GlobalThreshold <= 0 || *req.GlobalThreshold >= 1 {
			writeError(w, http.StatusBadRequest, "global_threshold must be in (0,1)")
			return
		}
		if err := a.store.SetSetting(store.SettingGlobalThreshold, formatFloat(*req.GlobalThreshold)); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}
	if req.SinkMode != nil {
		if !validSinkMode(*req.SinkMode) {
			writeError(w, http.StatusBadRequest, "sink_mode must be storage-only|forward-only|both")
			return
		}
		if err := a.store.SetSetting(store.SettingSinkMode, *req.SinkMode); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}
	writeJSON(w, http.StatusOK, a.effectiveSettings())
}

func validSinkMode(m string) bool {
	return m == "storage-only" || m == "forward-only" || m == "both"
}
