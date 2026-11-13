package core

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/evcc-io/evcc/api"
	"github.com/evcc-io/evcc/core/loadpoint"
	"github.com/evcc-io/evcc/core/solarcontrol"
	"github.com/evcc-io/evcc/db/settings"
	"github.com/evcc-io/evcc/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// solarLoadpoint is a loadpoint mock whose readings the test changes between cycles.
// Setters are not expected unless a test adds them, so any write fails the test.
type solarLoadpoint struct {
	*loadpoint.MockAPI
	status       api.ChargeStatus
	mode         api.ChargeMode
	alwaysCharge api.AlwaysCharge
}

func newSolarLoadpoint(ctrl *gomock.Controller) *solarLoadpoint {
	lp := &solarLoadpoint{
		MockAPI:      loadpoint.NewMockAPI(ctrl),
		status:       api.StatusA,
		mode:         api.ModeSmart,
		alwaysCharge: api.AlwaysChargeOff,
	}

	lp.EXPECT().GetStatus().DoAndReturn(func() api.ChargeStatus { return lp.status }).AnyTimes()
	lp.EXPECT().GetMode().DoAndReturn(func() api.ChargeMode { return lp.mode }).AnyTimes()
	lp.EXPECT().GetAlwaysCharge().DoAndReturn(func() api.AlwaysCharge { return lp.alwaysCharge }).AnyTimes()
	lp.EXPECT().GetChargePower().Return(0.0).AnyTimes()
	lp.EXPECT().GetChargedEnergy().Return(0.0).AnyTimes()
	lp.EXPECT().GetSoc().Return(50.0).AnyTimes()

	return lp
}

func solarTestRuntime(t *testing.T, mode solarcontrol.Mode, loc *time.Location) *solarControlRuntime {
	cfg := solarcontrol.DefaultConfig()
	cfg.Mode = mode
	require.NoError(t, cfg.Validate())

	return &solarControlRuntime{
		ctrl: solarcontrol.New(cfg, solarcontrol.State{}, loc),
		log:  util.NewLogger("test"),
	}
}

func sunnyForecast(day time.Time) *solarcontrol.Forecast {
	return &solarcontrol.Forecast{
		EnergyNext2h:    8000,
		EnergyRestOfDay: 20000,
		SunStart:        day.Add(8 * time.Hour),
		SunEnd:          day.Add(18 * time.Hour),
	}
}

// runSolarDay drives cycles through connect, a sunny period, a cloudy period with
// always charge on, mode now and a disconnect: every situation that writes in active mode
func runSolarDay(site *Site, rt *solarControlRuntime, lp *solarLoadpoint, day time.Time) {
	fc := sunnyForecast(day)
	now := day.Add(9 * time.Hour)

	cycle := func(n int, pv float64) {
		for range n {
			site.runSolarControl(rt, lp, now, pv, 300, fc)
			now = now.Add(30 * time.Second)
		}
	}

	cycle(10, 4000) // warm up while disconnected
	lp.status = api.StatusB
	cycle(60, 4000) // 30 min sunny and connected
	lp.alwaysCharge = api.AlwaysChargeOn
	cycle(80, 200) // 40 min clouds with always charge on
	lp.alwaysCharge = api.AlwaysChargeOff
	lp.mode = api.ModeNow
	lp.alwaysCharge = api.AlwaysChargeOn
	cycle(2, 4000)
	lp.status = api.StatusA // disconnect with mode now and always charge on
	cycle(4, 4000)
}

func TestSolarControlOffNeverWrites(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Madrid")
	require.NoError(t, err)
	day := time.Date(2026, 6, 1, 0, 0, 0, 0, loc)

	for _, mode := range []solarcontrol.Mode{solarcontrol.ModeOff, solarcontrol.ModeShadow} {
		t.Run(string(mode), func(t *testing.T) {
			ctrl := gomock.NewController(t)
			lp := newSolarLoadpoint(ctrl) // no setter expectations: any write fails

			runSolarDay(new(Site), solarTestRuntime(t, mode, loc), lp, day)
		})
	}
}

func TestSolarControlActiveWrites(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Madrid")
	require.NoError(t, err)
	day := time.Date(2026, 6, 1, 0, 0, 0, 0, loc)

	ctrl := gomock.NewController(t)
	lp := newSolarLoadpoint(ctrl)

	var writes []string
	lp.EXPECT().SetAlwaysCharge(gomock.Any()).DoAndReturn(func(ac api.AlwaysCharge) error {
		writes = append(writes, "alwaysCharge "+string(ac))
		lp.alwaysCharge = ac
		return nil
	}).AnyTimes()
	lp.EXPECT().SetMode(gomock.Any()).Do(func(mode api.ChargeMode) {
		writes = append(writes, "mode "+string(mode))
		lp.mode = mode
	}).AnyTimes()

	runSolarDay(new(Site), solarTestRuntime(t, solarcontrol.ModeActive, loc), lp, day)

	assert.Equal(t, []string{
		"alwaysCharge on",  // sunny for longer than the start hold
		"alwaysCharge off", // clouds for longer than the stop hold
		"mode smart",       // disconnect resets mode now
		"alwaysCharge off", // and always charge
	}, writes)
}

func TestSolarControlSetConfig(t *testing.T) {
	site := new(Site)

	require.NoError(t, site.SetSolarControlConfig([]byte(`{"mode":"shadow","minChargePower":1500}`)))
	cfg, status := site.SolarControl()
	assert.Equal(t, solarcontrol.ModeShadow, cfg.Mode)
	assert.Equal(t, solarcontrol.ModeShadow, status.Mode)
	assert.Equal(t, 1500.0, cfg.MinChargePower)
	assert.Equal(t, solarcontrol.DefaultConfig().SocHigh, cfg.SocHigh)

	for body, field := range map[string]string{
		`{"socLow":90}`:          "socHigh",
		`{"foo":1}`:              "foo",
		`{"mode":1}`:             "mode",
		`{"minChargePower":"x"}`: "minChargePower",
	} {
		err := site.SetSolarControlConfig([]byte(body))
		ve, ok := err.(*solarcontrol.ValidationError)
		require.True(t, ok, body)
		assert.Equal(t, field, ve.Field, body)
	}

	cfg, _ = site.SolarControl()
	assert.Equal(t, 1500.0, cfg.MinChargePower, "rejected updates leave config unchanged")
}

func TestSolarControlStatusMarshal(t *testing.T) {
	b, err := solarControlStatus(solarcontrol.Status{Events: []solarcontrol.Event{}}).MarshalBytes()
	require.NoError(t, err)

	var res map[string]any
	require.NoError(t, json.Unmarshal(b, &res))
	assert.Contains(t, res, "reason")
	assert.Equal(t, []any{}, res["events"])
}

func TestSolarControlPanicDoesNotEscape(t *testing.T) {
	ctrl := gomock.NewController(t)
	lp := loadpoint.NewMockAPI(ctrl)
	lp.EXPECT().GetStatus().DoAndReturn(func() api.ChargeStatus { panic("boom") })

	rt := solarTestRuntime(t, solarcontrol.ModeActive, time.UTC)
	assert.NotPanics(t, func() {
		new(Site).runSolarControl(rt, lp, time.Now(), 1000, 300, nil)
	})
}

func TestSolarControlFailedWriteNotCounted(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Madrid")
	require.NoError(t, err)
	day := time.Date(2026, 6, 1, 0, 0, 0, 0, loc)

	ctrl := gomock.NewController(t)
	lp := newSolarLoadpoint(ctrl)
	lp.status = api.StatusB

	var attempts int
	lp.EXPECT().SetAlwaysCharge(api.AlwaysChargeOn).DoAndReturn(func(api.AlwaysCharge) error {
		attempts++
		return errors.New("not supported")
	}).AnyTimes()

	site := new(Site)
	rt := solarTestRuntime(t, solarcontrol.ModeActive, loc)
	fc := sunnyForecast(day)

	now := day.Add(9 * time.Hour)
	for range 120 { // 1h of sun, start due after ~22 min
		site.runSolarControl(rt, lp, now, 4000, 300, fc)
		now = now.Add(30 * time.Second)
	}

	status := rt.ctrl.Status()
	assert.Positive(t, attempts)
	assert.Less(t, attempts, 10, "failed start must back off")
	assert.Zero(t, status.StartsToday, "failed start must not count")
	assert.Empty(t, status.Events, "failed start must not be recorded")
}

func TestSolarControlPersistence(t *testing.T) {
	t.Cleanup(func() {
		_ = settings.Delete(solarControlConfigKey)
		_ = settings.Delete(solarControlStateKey)
	})

	rt := solarTestRuntime(t, solarcontrol.ModeShadow, time.UTC)
	cfg := rt.ctrl.Config()
	cfg.MinChargePower = 1600
	require.NoError(t, rt.ctrl.SetConfig(cfg))

	// shadow counts external always charge transitions
	ctrl := gomock.NewController(t)
	lp := newSolarLoadpoint(ctrl)
	lp.status = api.StatusB
	now := time.Now()
	new(Site).runSolarControl(rt, lp, now, 1000, 300, nil)
	lp.alwaysCharge = api.AlwaysChargeOn
	new(Site).runSolarControl(rt, lp, now.Add(30*time.Second), 1000, 300, nil)
	require.Equal(t, 1, rt.ctrl.Status().StartsToday)

	rt.persist(true)

	loaded := loadSolarControl(util.NewLogger("test"))
	assert.Equal(t, rt.ctrl.Config(), loaded.Config())

	st := loaded.State()
	assert.Equal(t, 1, st.StartsToday)
	assert.False(t, st.LastStartAt.IsZero())
	require.Len(t, st.Events, 1)
	assert.Equal(t, solarcontrol.EventExternal, st.Events[0].Type)
}
