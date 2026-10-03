package shadow

import (
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Read the same named allocation used by the UI, so this exercises the default
// build rather than an independently maintained talent-string approximation.
func foreverRotationTalents(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../../../ui/shadow_priest/talents/forever-14-3-34.json")
	if err != nil {
		t.Fatal(err)
	}
	talents := &proto.PriestTalents{}
	if err := protojson.Unmarshal(data, talents); err != nil {
		t.Fatal(err)
	}
	message := talents.ProtoReflect()
	fields := message.Descriptor().Fields()
	treeSizes := []int{18, 17, 18}
	wantTotals := []int{14, 3, 34}
	trees := make([]string, len(treeSizes))
	offset := 0
	for tree, size := range treeSizes {
		var text strings.Builder
		total := 0
		for index := 1; index <= size; index++ {
			field := fields.ByNumber(protoreflect.FieldNumber(offset + index))
			points := 0
			if field.Kind() == protoreflect.BoolKind {
				if message.Get(field).Bool() {
					points = 1
				}
			} else {
				points = int(message.Get(field).Int())
			}
			fmt.Fprint(&text, points)
			total += points
		}
		if total != wantTotals[tree] {
			t.Fatalf("default talent tree %d has %d points, want %d", tree, total, wantTotals[tree])
		}
		trees[tree] = strings.TrimRight(text.String(), "0")
		offset += size
	}
	return strings.Join(trees, "-")
}

func foreverActionCasts(metrics *proto.UnitMetrics, spellID int32) int32 {
	var casts int32
	for _, action := range metrics.Actions {
		if action.Id.GetSpellId() == spellID {
			for _, target := range action.Targets {
				casts += target.Casts
			}
		}
	}
	return casts
}

// Require an actual two-tick channel ending before its scheduled third tick.
// Aggregate tick counts alone could confuse a clipped channel with a miss or
// the encounter boundary. Only inspect complete, earlier aura lifetimes.
func foreverTwoTickChannels(logs string, duration float64) int {
	auraLine := regexp.MustCompile(`^\[([0-9.]+)\].*Aura (gained|faded): \{SpellID: 18807\}$`)
	start := -1.0
	ticks := 0
	clipped := 0
	for _, line := range strings.Split(logs, "\n") {
		if strings.Contains(line, "{SpellID: 18807} tick ") {
			ticks++
		}
		match := auraLine.FindStringSubmatch(line)
		if len(match) == 0 {
			continue
		}
		at, _ := strconv.ParseFloat(match[1], 64)
		if match[2] == "gained" {
			start, ticks = at, 0
		} else {
			if start >= 0 && at < duration-0.01 && math.Abs(at-start-2) < 0.011 && ticks == 2 {
				clipped++
			}
			start = -1
		}
	}
	return clipped
}

func TestForeverRotationPresets(t *testing.T) {
	talents := foreverRotationTalents(t)
	races := []proto.Race{
		proto.Race_RaceHuman, proto.Race_RaceDwarf, proto.Race_RaceNightElf,
		proto.Race_RaceGnome, proto.Race_RaceUndead, proto.Race_RaceTroll,
	}
	for _, race := range races {
		presets := []string{"forever", "forever-clipped"}
		if race == proto.Race_RaceNightElf {
			presets = append(presets, "forever-starshards")
		}
		for _, preset := range presets {
			t.Run(race.String()+"/"+preset, func(t *testing.T) {
				player := &proto.Player{
					Name: "Forever Priest", Class: proto.Class_ClassPriest, Race: race,
					Equipment:     core.GetGearSet("../../../ui/shadow_priest/gear_sets", "p0.bis").GearSet,
					TalentsString: talents,
					Rotation:      core.GetAplRotation("../../../ui/shadow_priest/apls", preset).Rotation,
					Consumes:      P1Consumes.Consumes, Buffs: core.ForeverIndividualBuffs,
					ChannelClipDelayMs: 100, DistanceFromTarget: 30,
					Spec: &proto.Player_ShadowPriest{ShadowPriest: &proto.ShadowPriest{
						Options: &proto.ShadowPriest_Options{Armor: proto.ShadowPriest_Options_InnerFire},
					}},
				}
				encounter := core.MakeSingleTargetEncounter(0)
				encounter.Duration = 120
				raid := core.SinglePlayerRaidProto(player, nil, core.ForeverBuffs.Raid, core.ForeverBuffs.Debuffs)
				computed := core.ComputeStats(&proto.ComputeStatsRequest{
					Raid: raid, Encounter: encounter, Ruleset: proto.Ruleset_RulesetForever,
				})
				rotationStats := computed.RaidStats.Parties[0].Players[0].RotationStats
				if rotationStats == nil {
					t.Fatal("missing APL validation result")
				}
				for _, action := range append(rotationStats.PrepullActions, rotationStats.PriorityList...) {
					if len(action.Warnings) != 0 {
						t.Fatalf("APL warnings: %v", action.Warnings)
					}
				}
				result := core.RunRaidSim(&proto.RaidSimRequest{
					Raid: raid, Encounter: encounter,
					SimOptions: &proto.SimOptions{
						Iterations: 20, IsTest: true, RandomSeed: 12345,
						Ruleset: proto.Ruleset_RulesetForever, DebugFirstIteration: true,
					},
				})
				if result.Error != nil {
					t.Fatalf("sim failed: %s", result.Error.Message)
				}
				if result.IterationsDone != 20 {
					t.Fatalf("completed %d iterations, want 20", result.IterationsDone)
				}
				metrics := result.RaidMetrics.Parties[0].Players[0]
				if metrics.Dps.Avg <= 0 {
					t.Fatal("rotation dealt no damage")
				}
				for _, spellID := range []int32{10894, 19280, 10947, 1309636, 18807} {
					if foreverActionCasts(metrics, spellID) <= 0 {
						t.Errorf("rotation never cast spell %d", spellID)
					}
				}
				starshards := foreverActionCasts(metrics, 19305)
				if (starshards > 0) != (race == proto.Race_RaceNightElf && preset == "forever-starshards") {
					t.Errorf("unexpected Starshards casts: %d", starshards)
				}
				if race == proto.Race_RaceGnome && foreverActionCasts(metrics, 1259823) <= 0 {
					t.Error("Gnome rotation did not activate Priest Eureka (1259823)")
				}
				clipped := foreverTwoTickChannels(result.Logs, encounter.Duration)
				if (clipped > 0) != (preset == "forever-clipped") {
					t.Errorf("unexpected two-tick Flay channels in first iteration: %d", clipped)
				}
				t.Logf("%.2f DPS; DP=%d Death=%d Flay=%d Starshards=%d; two-tick channels in first iteration=%d", metrics.Dps.Avg,
					foreverActionCasts(metrics, 19280), foreverActionCasts(metrics, 1309636), foreverActionCasts(metrics, 18807), starshards, clipped)
			})
		}
	}
}
