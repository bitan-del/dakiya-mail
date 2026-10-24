package models

import (
	"encoding/json"
	"testing"
)

func TestLiveWriterShipsDarkAndClamps(t *testing.T) {
	def := DefaultWarmupGenerationSettings()
	if def.LiveWriterPercent != 0 || def.LiveWriterKillSwitch || def.LiveWriterOpens(0) {
		t.Errorf("the writer must default to off, got %+v", def)
	}

	for in, want := range map[int]int{-5: 0, 0: 0, 40: 40, 100: 100, 250: 100} {
		s := WarmupGenerationSettings{LiveWriterPercent: in}
		s.Normalize()
		if s.LiveWriterPercent != want {
			t.Errorf("percent %d normalized to %d, want %d", in, s.LiveWriterPercent, want)
		}
	}
}

func TestLiveWriterOpensHonoursPercentAndKillSwitch(t *testing.T) {
	s := WarmupGenerationSettings{LiveWriterPercent: 30}
	if !s.LiveWriterOpens(29) || s.LiveWriterOpens(30) {
		t.Error("a roll under the percent opens, a roll at it does not")
	}
	s.LiveWriterKillSwitch = true
	if s.LiveWriterOpens(0) {
		t.Error("the kill switch beats the percent")
	}
}

func TestLiveWriterSettingsSurviveTheStoredDocument(t *testing.T) {
	s := DefaultWarmupGenerationSettings()
	if err := json.Unmarshal([]byte(`{"live_writer_percent": 15, "live_writer_kill_switch": true}`), &s); err != nil {
		t.Fatal(err)
	}
	s.Normalize()
	if s.LiveWriterPercent != 15 || !s.LiveWriterKillSwitch {
		t.Errorf("settings = %+v", s)
	}
}
