package api

import (
	"net/http"

	"github.com/t0mer/fulcrum/internal/store"
)

// listGroups refreshes the group list from the provider (best-effort) and
// returns the stored groups with their monitored/destination flags.
func (a *API) listGroups(w http.ResponseWriter, r *http.Request) {
	if a.provider != nil {
		if groups, err := a.provider.ListGroups(r.Context()); err != nil {
			a.log.Warn("refresh groups from provider", "err", err)
		} else {
			for _, g := range groups {
				if err := a.store.UpsertGroup(g.ProviderGroupID, g.Name); err != nil {
					a.log.Warn("upsert group", "err", err)
				}
			}
		}
	}
	groups, err := a.store.ListGroups()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if groups == nil {
		groups = []store.Group{}
	}
	writeJSON(w, http.StatusOK, groups)
}

type updateGroupReq struct {
	Monitored     *bool `json:"monitored"`
	IsDestination *bool `json:"is_destination"`
}

func (a *API) updateGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := a.pathID(w, r, "id")
	if !ok {
		return
	}
	var req updateGroupReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Monitored != nil {
		if err := a.store.SetMonitored(id, *req.Monitored); err != nil {
			a.respondLookup(w, err)
			return
		}
	}
	if req.IsDestination != nil {
		target := int64(0)
		if *req.IsDestination {
			target = id
		}
		if err := a.store.SetDestination(target); err != nil {
			a.respondLookup(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
