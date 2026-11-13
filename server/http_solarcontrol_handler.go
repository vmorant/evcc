package server

import (
	"errors"
	"io"
	"net/http"

	"github.com/evcc-io/evcc/core/solarcontrol"
	"github.com/evcc-io/evcc/util/auth"
	"github.com/gorilla/mux"
)

// solarControlSite is the part of the site used by the solar control handlers
type solarControlSite interface {
	SolarControl() (solarcontrol.Config, solarcontrol.Status)
	SetSolarControlConfig([]byte) error
	ResetSolarControl()
}

type solarControlResponse struct {
	Config solarcontrol.Config `json:"config"`
	Status solarcontrol.Status `json:"status"`
}

func solarControlGetHandler(site solarControlSite) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, status := site.SolarControl()
		jsonWrite(w, solarControlResponse{cfg, status})
	}
}

func solarControlConfigHandler(site solarControlSite) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
		if err != nil {
			jsonError(w, http.StatusBadRequest, err)
			return
		}

		if err := site.SetSolarControlConfig(b); err != nil {
			if ve, ok := errors.AsType[*solarcontrol.ValidationError](err); ok {
				w.WriteHeader(http.StatusBadRequest)
				jsonWrite(w, ve)
				return
			}
			jsonError(w, http.StatusBadRequest, err)
			return
		}

		solarControlGetHandler(site)(w, r)
	}
}

func solarControlResetHandler(site solarControlSite) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		site.ResetSolarControl()
		solarControlGetHandler(site)(w, r)
	}
}

// registerSolarControlHandlers adds the solar control api; writes need the same login as /config
func registerSolarControlHandlers(api *mux.Router, site solarControlSite, authObject auth.Auth) {
	api.Methods("GET").Path("/solarcontrol").Handler(solarControlGetHandler(site))

	ensureAuth := EnsureAuthHandler(authObject)
	api.Methods("POST").Path("/solarcontrol/config").Handler(ensureAuth(solarControlConfigHandler(site)))
	api.Methods("POST").Path("/solarcontrol/reset").Handler(ensureAuth(solarControlResetHandler(site)))
}
