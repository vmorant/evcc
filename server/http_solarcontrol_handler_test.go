package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/evcc-io/evcc/core/solarcontrol"
	"github.com/evcc-io/evcc/util/auth"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSolarSite applies partial json on top of the current config like the site does
type fakeSolarSite struct {
	cfg    solarcontrol.Config
	resets int
}

func (s *fakeSolarSite) SolarControl() (solarcontrol.Config, solarcontrol.Status) {
	return s.cfg, solarcontrol.Status{Mode: s.cfg.Mode, Strategy: s.cfg.Strategy, Events: []solarcontrol.Event{}}
}

func (s *fakeSolarSite) SetSolarControlConfig(b []byte) error {
	cfg := s.cfg
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return &solarcontrol.ValidationError{Field: "", Msg: err.Error()}
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	s.cfg = cfg
	return nil
}

func (s *fakeSolarSite) ResetSolarControl() { s.resets++ }

func solarControlRouter(site solarControlSite, mode auth.AuthMode) *mux.Router {
	router := mux.NewRouter()
	registerSolarControlHandlers(router.PathPrefix("/api").Subrouter(), site, fakeAuth{mode: mode})
	return router
}

func serveSolar(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

func TestSolarControlGetIsOpen(t *testing.T) {
	site := &fakeSolarSite{cfg: solarcontrol.DefaultConfig()}
	router := solarControlRouter(site, auth.Enabled)

	rec := serveSolar(router, http.MethodGet, "/api/solarcontrol", "")
	require.Equal(t, http.StatusOK, rec.Code)

	var res solarControlResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	assert.Equal(t, solarcontrol.ModeOff, res.Config.Mode)
	assert.Equal(t, solarcontrol.ModeOff, res.Status.Mode)
}

func TestSolarControlWritesRequireAuth(t *testing.T) {
	site := &fakeSolarSite{cfg: solarcontrol.DefaultConfig()}

	for _, path := range []string{"/api/solarcontrol/config", "/api/solarcontrol/reset"} {
		for _, mode := range []auth.AuthMode{auth.Enabled, auth.Locked} {
			rec := serveSolar(solarControlRouter(site, mode), http.MethodPost, path, `{"mode":"active"}`)
			assert.Equal(t, http.StatusUnauthorized, rec.Code, path)
		}
	}

	assert.Equal(t, solarcontrol.ModeOff, site.cfg.Mode)
	assert.Zero(t, site.resets)
}

func TestSolarControlConfig(t *testing.T) {
	tc := []struct {
		name  string
		body  string
		code  int
		field string
	}{
		{"partial", `{"mode":"shadow","minChargePower":1500}`, http.StatusOK, ""},
		{"out of range", `{"minChargePower":100}`, http.StatusBadRequest, "minChargePower"},
		{"soc order", `{"socLow":80}`, http.StatusBadRequest, "socHigh"},
		{"hour order", `{"startHourFrom":18}`, http.StatusBadRequest, "startHourTo"},
		{"invalid mode", `{"mode":"on"}`, http.StatusBadRequest, "mode"},
		{"unknown field", `{"foo":1}`, http.StatusBadRequest, ""},
		{"malformed", `{`, http.StatusBadRequest, ""},
	}

	for _, tc := range tc {
		t.Run(tc.name, func(t *testing.T) {
			site := &fakeSolarSite{cfg: solarcontrol.DefaultConfig()}
			rec := serveSolar(solarControlRouter(site, auth.Disabled), http.MethodPost, "/api/solarcontrol/config", tc.body)
			require.Equal(t, tc.code, rec.Code, rec.Body.String())

			if tc.code == http.StatusOK {
				var res solarControlResponse
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
				assert.Equal(t, solarcontrol.ModeShadow, res.Config.Mode)
				assert.Equal(t, 1500.0, res.Config.MinChargePower)
				assert.Equal(t, 60, res.Config.RestartCooldownMin, "untouched fields keep their value")
				return
			}

			var ve solarcontrol.ValidationError
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ve))
			assert.Equal(t, tc.field, ve.Field)
			assert.NotEmpty(t, ve.Msg)
			assert.Equal(t, solarcontrol.DefaultConfig(), site.cfg, "invalid config not applied")
		})
	}
}

func TestSolarControlReset(t *testing.T) {
	site := &fakeSolarSite{cfg: solarcontrol.DefaultConfig()}
	rec := serveSolar(solarControlRouter(site, auth.Disabled), http.MethodPost, "/api/solarcontrol/reset", "")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, site.resets)
}

func TestSolarControlWritesWithSession(t *testing.T) {
	site := &fakeSolarSite{cfg: solarcontrol.DefaultConfig()}
	router := solarControlRouter(site, auth.Enabled)

	req := httptest.NewRequest(http.MethodPost, "/api/solarcontrol/config", strings.NewReader(`{"mode":"shadow"}`))
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: "token"}) // fakeAuth accepts any jwt
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, solarcontrol.ModeShadow, site.cfg.Mode)
}
