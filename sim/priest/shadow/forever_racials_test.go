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
)

func racialRotationRequest(t *testing.T, race proto.Race, ruleset proto.Ruleset, iterations int32) *proto.RaidSimRequest {
	t.Helper()
	player := &proto.Player{
		Name: "Racial comparison", Class: proto.Class_ClassPriest, Race: race,
		Equipment:     core.GetGearSet("../../../ui/shadow_priest/gear_sets", "p0.bis").GearSet,
		TalentsString: foreverRotationTalents(t),
		Rotation:      core.GetAplRotation("../../../ui/shadow_priest/apls", "forever").Rotation,
		Consumes:      P1Consumes.Consumes, Buffs: core.ForeverIndividualBuffs,
		ChannelClipDelayMs: 100, DistanceFromTarget: 30,
		Spec: &proto.Player_ShadowPriest{ShadowPriest: &proto.ShadowPriest{
			Options: &proto.ShadowPriest_Options{Armor: proto.ShadowPriest_Options_InnerFire},
		}},
	}
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Duration = 120
	return &proto.RaidSimRequest{
		Raid:      core.SinglePlayerRaidProto(player, nil, core.ForeverBuffs.Raid, core.ForeverBuffs.Debuffs),
		Encounter: encounter,
		SimOptions: &proto.SimOptions{
			Iterations: iterations, IsTest: true, RandomSeed: 12345,
			Ruleset: ruleset, DebugFirstIteration: true,
		},
	}
}

func racialActionDamage(metrics *proto.UnitMetrics, id int32) float64 {
	var damage float64
	for _, action := range metrics.Actions {
		if action.Id.GetSpellId() == id {
			for _, target := range action.Targets {
				damage += target.Damage
			}
		}
	}
	return damage
}

func racialProcAttempts(metrics *proto.UnitMetrics, id int32) int32 {
	var attempts int32
	for _, action := range metrics.Actions {
		if action.Id.GetSpellId() == id {
			for _, target := range action.Targets {
				// Passive spells intentionally omit casts from exported metrics.
				// Resisted hit/crit counts are subsets of hits/crits.
				attempts += target.Hits + target.Crits + target.Misses
			}
		}
	}
	return attempts
}

func racialAuraUptime(metrics *proto.UnitMetrics, id int32) float64 {
	for _, aura := range metrics.Auras {
		if aura.Id.GetSpellId() == id {
			return aura.UptimeSecondsAvg
		}
	}
	return 0
}

func runRacialRequest(t *testing.T, request *proto.RaidSimRequest) *proto.RaidSimResult {
	t.Helper()
	result := core.RunRaidSim(request)
	if result.Error != nil {
		t.Fatal(result.Error.Message)
	}
	if result.IterationsDone != request.SimOptions.Iterations {
		t.Fatal("incomplete simulation")
	}
	return result
}

func TestForeverRacialEffectsInAutoRotation(t *testing.T) {
	const iterations = 1000
	for _, race := range []proto.Race{proto.Race_RaceHuman, proto.Race_RaceUndead, proto.Race_RaceGnome, proto.Race_RaceNightElf, proto.Race_RaceTroll} {
		t.Run(race.String(), func(t *testing.T) {
			request := racialRotationRequest(t, race, proto.Ruleset_RulesetForever, iterations)
			result := runRacialRequest(t, request)
			metrics := result.RaidMetrics.Parties[0].Players[0]
			var racialID, auraID int32
			switch race {
			case proto.Race_RaceGnome:
				racialID = 1259823
			case proto.Race_RaceNightElf:
				racialID = 1259799
			case proto.Race_RaceTroll:
				racialID = 20554
			case proto.Race_RaceUndead:
				racialID = 1260198
				auraID = 1260201
			}
			if auraID == 0 {
				auraID = racialID
			}
			casts := 0.0
			if racialID != 0 {
				casts = float64(foreverActionCasts(metrics, racialID)) / iterations
			}
			if race == proto.Race_RaceUndead {
				casts = float64(racialProcAttempts(metrics, racialID)) / iterations
			}
			uptime := racialAuraUptime(metrics, auraID)
			if racialID != 0 && casts == 0 {
				t.Fatalf("racial %d did not activate", racialID)
			}
			if race == proto.Race_RaceUndead && racialActionDamage(metrics, racialID) <= 0 {
				t.Fatal("Touch of the Grave dealt no damage")
			}
			if race == proto.Race_RaceGnome && uptime <= 0 {
				t.Fatal("Eureka had no active buff window")
			}
			if race == proto.Race_RaceTroll && math.Abs(uptime-10) > .001 {
				t.Fatalf("Berserking uptime %.4f, want 10 seconds", uptime)
			}
			if race == proto.Race_RaceNightElf && math.Abs(uptime-15) > .001 {
				t.Fatalf("Elune uptime %.4f, want 15 seconds", uptime)
			}
			t.Logf("DPS=%.4f stderr=%.4f OOM=%.2fs racial=%d uses-or-procs/fight=%.3f uptime=%.3fs racialDPS=%.4f MB=%.3f MF=%.3f",
				metrics.Dps.Avg, metrics.Dps.Stdev/math.Sqrt(iterations), metrics.SecondsOomAvg, racialID, casts, uptime,
				racialActionDamage(metrics, racialID)/(iterations*120), float64(foreverActionCasts(metrics, 10947))/iterations, float64(foreverActionCasts(metrics, 18807))/iterations)
			if race == proto.Race_RaceTroll {
				verifyBerserkingCastAndChannelTiming(t, result.Logs)
			}
			// Delay just the active racial beyond the encounter to isolate its effect
			// without changing race, stats, gear, spell ranks or the action priority.
			if race == proto.Race_RaceTroll || race == proto.Race_RaceGnome || race == proto.Race_RaceNightElf {
				id := &proto.ActionID{RawId: &proto.ActionID_SpellId{SpellId: racialID}}
				request.Raid.Parties[0].Players[0].Cooldowns = &proto.Cooldowns{Cooldowns: []*proto.Cooldown{{Id: id, Timings: []float64{1000}}}}
				delayed := runRacialRequest(t, request).RaidMetrics.Parties[0].Players[0]
				if foreverActionCasts(delayed, racialID) != 0 {
					t.Fatal("delayed racial still cast")
				}
				t.Logf("same-race delayed-active baseline=%.4f DPS; active effect=%+.4f DPS", delayed.Dps.Avg, metrics.Dps.Avg-delayed.Dps.Avg)
			}
		})
	}
}

func verifyBerserkingCastAndChannelTiming(t *testing.T, logs string) {
	t.Helper()
	castLine := regexp.MustCompile(`^\[([0-9.]+)\].*Casting \{SpellID: 10947\}.*Cast Time = ([^,]+),`)
	auraLine := regexp.MustCompile(`^\[([0-9.]+)\].*Aura (gained|faded): \{SpellID: 18807\}$`)
	tickLine := regexp.MustCompile(`^\[([0-9.]+)\].*\{SpellID: 18807\} tick `)
	fast, normal, fullEarlyChannel := false, false, false
	channelStart := -1.0
	var ticks []float64
	for _, line := range strings.Split(logs, "\n") {
		if match := castLine.FindStringSubmatch(line); match != nil {
			at, _ := strconv.ParseFloat(match[1], 64)
			duration, err := time.ParseDuration(match[2])
			if err != nil {
				t.Fatal(err)
			}
			if at < 10 && math.Abs(duration.Seconds()-1.5/1.1) < .00001 {
				fast = true
			}
			if at >= 10 && duration == 1500*time.Millisecond {
				normal = true
			}
		}
		if match := tickLine.FindStringSubmatch(line); match != nil && channelStart >= 0 {
			at, _ := strconv.ParseFloat(match[1], 64)
			ticks = append(ticks, at)
			if at <= 10 && len(ticks) == 3 {
				for i, tick := range ticks {
					if math.Abs(tick-channelStart-float64(i+1)) > .001 {
						t.Fatalf("Mind Flay tick spacing during Berserking: %v", ticks)
					}
				}
				fullEarlyChannel = true
			}
		}
		if match := auraLine.FindStringSubmatch(line); match != nil {
			at, _ := strconv.ParseFloat(match[1], 64)
			if match[2] == "gained" {
				channelStart = at
				ticks = nil
			}
		}
	}
	if !fast || !normal || !fullEarlyChannel {
		t.Fatalf("Berserking timing proof missing: fastMB=%v normalMB=%v fullMF=%v", fast, normal, fullEarlyChannel)
	}
	t.Log("Berserking: Mind Blast hard cast 1.363636s during aura, 1.500s after; full Mind Flay remains three 1s ticks")
}

func TestForeverRacialRegistrationDoesNotLeakIntoClassic(t *testing.T) {
	for _, race := range []proto.Race{proto.Race_RaceGnome, proto.Race_RaceNightElf, proto.Race_RaceUndead, proto.Race_RaceTroll} {
		t.Run(race.String(), func(t *testing.T) {
			request := racialRotationRequest(t, race, proto.Ruleset_RulesetClassic, 20)
			// Use a valid common three-spell control when checking Classic gating;
			// Forever's all-race Plague and Death are intentionally unavailable there.
			rotation := request.Raid.Parties[0].Players[0].Rotation
			var actions []*proto.APLListItem
			for _, item := range rotation.PriorityList {
				id := item.GetAction().GetCastSpell().GetSpellId().GetSpellId()
				if id != 19280 && id != 1309636 && id != 14751 {
					actions = append(actions, item)
				}
			}
			rotation.PriorityList = actions
			result := runRacialRequest(t, request)
			metrics := result.RaidMetrics.Parties[0].Players[0]
			for _, id := range []int32{1259823, 1259799, 1260198, 1260201, 20554, 1309636} {
				if foreverActionCasts(metrics, id) != 0 || racialProcAttempts(metrics, id) != 0 || racialAuraUptime(metrics, id) != 0 {
					t.Fatalf("Forever-only spell %d active in Classic", id)
				}
			}
			t.Logf("Classic gating control %.3f DPS, Berserking casts/fight %.2f; no Forever-only racial actions", metrics.Dps.Avg, float64(foreverActionCasts(metrics, 26297))/20)
		})
	}
}
