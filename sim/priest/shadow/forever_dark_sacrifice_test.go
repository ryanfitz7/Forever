package shadow

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/simsignals"
	"github.com/wowsims/classic/sim/core/stats"
)

const darkSacrificeRank5ID int32 = 1277328

func darkSacrificeRotationRequest(t *testing.T) *proto.RaidSimRequest {
	t.Helper()
	request := racialRotationRequest(t, proto.Race_RaceUndead, proto.Ruleset_RulesetForever, 1)
	player := request.Raid.Parties[0].Players[0]
	player.Rotation = core.GetAplRotation("../../../ui/shadow_priest/apls", "forever-dark-sacrifice").Rotation
	// Remove external regeneration and consumable recovery to exercise the
	// optional health-to-mana action in an otherwise ordinary damage rotation.
	player.Consumes = &proto.Consumes{}
	player.Buffs = &proto.IndividualBuffs{}
	request.Raid.Buffs = &proto.RaidBuffs{}
	request.Raid.Debuffs = &proto.Debuffs{}
	return request
}

func validateDarkSacrificeRotation(t *testing.T, request *proto.RaidSimRequest) {
	t.Helper()
	computed := core.ComputeStats(&proto.ComputeStatsRequest{Raid: request.Raid, Encounter: request.Encounter, Ruleset: proto.Ruleset_RulesetForever})
	rotation := computed.RaidStats.Parties[0].Players[0].RotationStats
	if rotation == nil {
		t.Fatal("missing APL validation")
	}
	for _, action := range append(rotation.PrepullActions, rotation.PriorityList...) {
		if len(action.Warnings) != 0 {
			t.Fatalf("saved Dark Sacrifice preset has APL warnings: %v", action.Warnings)
		}
	}
}

func TestForeverDarkSacrificeSavedRotation(t *testing.T) {
	for _, preset := range []string{"forever-dark-sacrifice", "forever-dark-sacrifice-clipped"} {
		for _, scenario := range []string{"mana-limited", "high-mana", "short-fight"} {
			t.Run(preset+"/"+scenario, func(t *testing.T) {
				request := darkSacrificeRotationRequest(t)
				request.Raid.Parties[0].Players[0].Rotation = core.GetAplRotation("../../../ui/shadow_priest/apls", preset).Rotation
				wantCasts := int32(1)
				switch scenario {
				case "high-mana":
					bonus := stats.Stats{stats.MP5: 1000}
					request.Raid.Parties[0].Players[0].BonusStats = &proto.UnitStats{Stats: bonus.ToFloatArray()}
					wantCasts = 0
				case "short-fight":
					request.Encounter.Duration = 15
					wantCasts = 0
				}
				validateDarkSacrificeRotation(t, request)
				result := runRacialRequest(t, request)
				metrics := result.RaidMetrics.Parties[0].Players[0]
				if got := foreverActionCasts(metrics, darkSacrificeRank5ID); got != wantCasts {
					t.Fatalf("Dark Sacrifice casts=%d, want %d", got, wantCasts)
				}
				var events int32
				var restored, actualRestored float64
				for _, resource := range metrics.Resources {
					if resource.Id.GetSpellId() == darkSacrificeRank5ID && resource.Type == proto.ResourceType_ResourceTypeMana {
						events += resource.Events
						restored += resource.Gain
						actualRestored += resource.ActualGain
					}
				}
				if events != 5*wantCasts || math.Abs(restored-float64(wantCasts)*1600) > 1e-6 || math.Abs(actualRestored-restored) > 1e-6 {
					t.Fatalf("Dark Sacrifice mana: ticks=%d gain=%f actual=%f", events, restored, actualRestored)
				}
				if wantCasts > 0 {
					verifyDarkSacrificeTickTiming(t, result.Logs)
				}
				clipped := foreverTwoTickChannels(result.Logs, request.Encounter.Duration)
				if scenario != "short-fight" && (clipped > 0) != strings.HasSuffix(preset, "-clipped") {
					t.Fatalf("two-tick Mind Flay channels=%d for preset %s", clipped, preset)
				}
				t.Logf("Dark Sacrifice casts=%d ticks=%d restored=%.0f; two-tick Flays=%d; damage rotation %.2f DPS", wantCasts, events, actualRestored, clipped, metrics.Dps.Avg)
			})
		}
	}
}

func verifyDarkSacrificeTickTiming(t *testing.T, logs string) {
	t.Helper()
	castLine := regexp.MustCompile(`^\[([0-9.]+)\].*Casting \{SpellID: 1277328\}`)
	manaLine := regexp.MustCompile(`^\[([0-9.]+)\].*Gained 320.000 mana from \{SpellID: 1277328\}`)
	start := -1.0
	var ticks []float64
	for _, line := range strings.Split(logs, "\n") {
		if match := castLine.FindStringSubmatch(line); match != nil {
			start, _ = strconv.ParseFloat(match[1], 64)
		}
		if match := manaLine.FindStringSubmatch(line); match != nil {
			at, _ := strconv.ParseFloat(match[1], 64)
			ticks = append(ticks, at)
		}
	}
	if start < 0 || len(ticks) != 5 {
		t.Fatalf("missing Dark Sacrifice cast/ticks: cast=%f ticks=%v", start, ticks)
	}
	for index, at := range ticks {
		if math.Abs(at-start-float64(index+1)*3) > .011 {
			t.Fatalf("Dark Sacrifice ticks must occur at +3,+6,+9,+12,+15s: cast=%f ticks=%v", start, ticks)
		}
	}
}

func TestForeverDarkSacrificeSavedConditionBoundaries(t *testing.T) {
	for _, check := range []struct {
		name                             string
		maxMana, mana, maxHealth, health float64
		remaining                        time.Duration
		wantCast                         bool
	}{
		{"at-mana-and-health-threshold", 5000, 2500, 4000, 2400, 16 * time.Second, true},
		{"above-half-mana", 5000, 2501, 4000, 4000, 16 * time.Second, false},
		{"exact-missing-mana", 3000, 1400, 4000, 4000, 16 * time.Second, true},
		{"insufficient-missing-mana", 3000, 1401, 4000, 4000, 16 * time.Second, false},
		{"exact-health-reserve", 5000, 2000, 2000, 1600, 16 * time.Second, false},
		{"above-health-reserve", 5000, 2000, 2000, 1601, 16 * time.Second, true},
		{"below-health-percent", 5000, 2000, 4000, 2399, 16 * time.Second, false},
		{"exact-duration", 5000, 2000, 4000, 4000, 15 * time.Second, false},
		{"enough-duration", 5000, 2000, 4000, 4000, 15*time.Second + time.Millisecond, true},
	} {
		t.Run(check.name, func(t *testing.T) {
			request := darkSacrificeRotationRequest(t)
			rotation := request.Raid.Parties[0].Players[0].Rotation
			// Keep the condition and action from the saved file, but isolate it
			// from damage spells that would change the boundary being tested.
			var sacrifice *proto.APLListItem
			for _, item := range rotation.PriorityList {
				if item.GetAction().GetCastSpell().GetSpellId().GetSpellId() == darkSacrificeRank5ID {
					sacrifice = item
				}
			}
			if sacrifice == nil {
				t.Fatal("saved rotation lacks Dark Sacrifice")
			}
			rotation.PrepullActions = nil
			rotation.PriorityList = []*proto.APLListItem{sacrifice}
			request.Encounter.Duration = check.remaining.Seconds()
			sim := core.NewSim(request, simsignals.CreateSignals())
			sim.Reset()
			p := sim.Raid.Parties[0].Players[0].(*ShadowPriest)
			p.AddStatDynamic(sim, stats.Mana, check.maxMana-p.MaxMana())
			p.AddStatDynamic(sim, stats.Health, check.maxHealth-p.MaxHealth())
			p.AddMana(sim, check.maxMana, p.NewManaMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionManaGain}))
			p.SpendMana(sim, p.CurrentMana()-check.mana, p.NewManaMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionManaGain}))
			p.GainHealth(sim, check.maxHealth, p.NewHealthMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionHealingModel}))
			p.RemoveHealth(sim, p.CurrentHealth()-check.health)
			p.Rotation.DoNextAction(sim)
			spell := p.GetSpell(core.ActionID{SpellID: darkSacrificeRank5ID})
			if got := spell.SelfHot().IsActive(); got != check.wantCast {
				t.Fatalf("Dark Sacrifice active=%v, want %v; mana %.3f/%.3f health %.3f/%.3f remaining %s", got, check.wantCast, p.CurrentMana(), p.MaxMana(), p.CurrentHealth(), p.MaxHealth(), sim.GetRemainingDuration())
			}
		})
	}
}
