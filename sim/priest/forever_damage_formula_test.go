package priest

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
)

type foreverDamageCase struct {
	name        string
	spellID     int32
	base        float64
	coefficient float64
	periodic    bool
	arcane      bool
	flay        bool
	instant     bool
}

// Values are independent of the implementation's damage/coefficient arrays.
// Periodic bases are per tick from the captured rank tooltips. Direct bases are
// fixed means at level 60 under the documented, unverified linear-growth model:
// MB R9 = 485 + 2*2.6; Death R4 = 448 + 4*2.5. Low ranks use their growth caps.
// The separate source test pins these coefficients to archived client records,
// including the September 20 review of low periodic ranks from build 69913.
// Identical client coefficients do not establish absence of a server downrank rule.
var foreverDamageCases = []foreverDamageCase{
	{"MindBlastRank1", 8092, 42, .429, false, false, false, false},
	{"MindBlastRank9", 10947, 490.2, .429, false, false, false, false},
	{"DeathRank1", 1309595, 302.5, .429, false, false, false, true},
	{"DeathRank4", 1309636, 458, .429, false, false, false, true},
	{"MindFlayRank1", 15407, 21, .167, true, false, true, false},
	{"MindFlayRank6", 18807, 130, .167, true, false, true, false},
	{"PainRank1", 589, 5, .20, true, false, false, true},
	{"PainRank8", 10894, 127, .20, true, false, false, true},
	{"PlagueRank1", 2944, 16, .10, true, false, false, true},
	{"PlagueRank6", 19280, 106, .10, true, false, false, true},
	{"StarshardsRank1", 10797, 27, .167, true, true, false, false},
	{"StarshardsRank7", 19305, 300, .167, true, true, false, false},
}

func TestForeverDamageFormulaPowerBeforeMultipliers(t *testing.T) {
	for _, talents := range []struct {
		name string
		flay int32
		twin int32
	}{{"Baseline", 0, 0}, {"ImprovedFlay", 2, 0}, {"TwinDisciplines", 0, 5}, {"BothTalents", 2, 5}} {
		for _, check := range foreverDamageCases {
			for _, genericPower := range []float64{0, 200} {
				t.Run(fmt.Sprintf("%s/%s/SP%.0f", talents.name, check.name, genericPower), func(t *testing.T) {
					sim, p := newPriestTestSim(t, proto.Race_RaceNightElf, 60, &proto.PriestTalents{
						Darkness: 5, ShadowWeaving: 3, Shadowform: true, MindFlay: true,
						ImprovedMindFlay: talents.flay, TwinDisciplines: talents.twin,
					}, proto.Ruleset_RulesetForever)
					p.ShadowformAura.Activate(sim)
					p.ShadowWeavingAura.Activate(sim)
					p.ShadowWeavingAura.SetStacks(sim, 5)
					closeEnough(t, "stacked Shadow multiplier", p.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexShadow], 1.331)
					closeEnough(t, "Arcane multiplier", p.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexArcane], 1)

					// Deliberately different school totals expose cross-school leakage.
					p.AddStatDynamic(sim, stats.SpellPower, genericPower)
					p.AddStatDynamic(sim, stats.SpellDamage, 50)
					p.AddStatDynamic(sim, stats.ShadowPower, 100)
					p.AddStatDynamic(sim, stats.ArcanePower, 250)
					p.AddStatDynamic(sim, stats.HolyPower, 3000)
					p.AddStatDynamic(sim, stats.FirePower, 4000)
					spell := p.GetSpell(core.ActionID{SpellID: check.spellID})
					if spell == nil {
						t.Fatal("missing modeled spell")
					}
					spell.Flags |= core.SpellFlagIgnoreResists
					power, multiplier := genericPower+50+100, 1.331
					if check.arcane {
						power, multiplier = genericPower+50+250, 1
					}
					if check.flay && talents.flay == 2 {
						multiplier *= 1.20
					}
					if check.instant && talents.twin == 5 {
						multiplier *= 1.05
					}
					baseWithPower := check.base + check.coefficient*power
					want := baseWithPower * multiplier
					if check.periodic {
						dot := spell.Dot(p.CurrentTarget)
						closeEnough(t, "registered tick coefficient", dot.BonusCoefficient, check.coefficient)
						dot.Apply(sim)
						closeEnough(t, "base plus relevant power before multipliers", dot.SnapshotBaseDamage, baseWithPower)
						closeEnough(t, "periodic attacker multiplier", dot.SnapshotAttackerMultiplier, multiplier)
						before := spell.SpellMetrics[p.CurrentTarget.UnitIndex].TotalDamage
						dot.TickOnce(sim)
						got := spell.SpellMetrics[p.CurrentTarget.UnitIndex].TotalDamage - before
						closeEnough(t, "actual noncritical unresisted tick", got, want)
					} else {
						closeEnough(t, "registered direct coefficient", spell.BonusCoefficient, check.coefficient)
						result := spell.CalcDamage(sim, p.CurrentTarget, check.base, spell.OutcomeAlwaysHit)
						closeEnough(t, "actual noncritical unresisted hit", result.Damage, want)
					}
				})
			}
		}
	}
}

func TestForeverDamageFormulaWeavingExpiry(t *testing.T) {
	sim, p := newPriestTestSim(t, proto.Race_RaceNightElf, 60, &proto.PriestTalents{
		Darkness: 5, ShadowWeaving: 3, Shadowform: true,
	}, proto.Ruleset_RulesetForever)
	p.ShadowformAura.Activate(sim)
	p.ShadowWeavingAura.Activate(sim)
	p.ShadowWeavingAura.SetStacks(sim, 5)
	p.AddStatDynamic(sim, stats.SpellPower, 200)
	p.AddStatDynamic(sim, stats.ShadowPower, 100)
	spell := p.MindBlast[9]
	spell.Flags |= core.SpellFlagIgnoreResists
	closeEnough(t, "full stacks hit", spell.CalcDamage(sim, p.CurrentTarget, 490.2, spell.OutcomeAlwaysHit).Damage, (490.2+.429*300)*1.331)
	for sim.CurrentTime < 15*time.Second {
		if sim.Step() {
			t.Fatal("encounter ended before Weaving expired")
		}
	}
	if p.ShadowWeavingAura.IsActive() || p.ShadowWeavingAura.GetStacks() != 0 {
		t.Fatal("Weaving did not expire after 15 seconds")
	}
	closeEnough(t, "Shadowform and Darkness remain", p.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexShadow], 1.21)
	closeEnough(t, "hit after expiry", spell.CalcDamage(sim, p.CurrentTarget, 490.2, spell.OutcomeAlwaysHit).Damage, (490.2+.429*300)*1.21)
}

func TestForeverDamageCoefficientsMatchArchivedClient(t *testing.T) {
	_, p := newPriestTestSim(t, proto.Race_RaceNightElf, 60, &proto.PriestTalents{MindFlay: true}, proto.Ruleset_RulesetForever)
	wanted := map[int32]bool{594: true, 970: true, 992: true, 17311: true, 19296: true}
	for _, check := range foreverDamageCases {
		wanted[check.spellID] = true
	}
	for _, spell := range append(p.MindBlast[1:], p.ShadowWordDeath[1:]...) {
		wanted[spell.ActionID.SpellID] = true
	}
	seen := make(map[int32]bool)
	for _, filename := range []string{
		"testdata/forever_direct_spell_records.json",
		"testdata/forever_coefficient_audit_69913.json",
		"../../docs/evidence/priest-2026-09/rotation-client-records.json",
		"../../docs/evidence/priest-2026-09/SpellEffect-SpellID-19305.json",
	} {
		raw, err := os.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
		var document struct {
			Tables []struct {
				Table   string
				Records []map[string]json.RawMessage
			}
			Records []map[string]json.RawMessage
		}
		if err := json.Unmarshal(raw, &document); err != nil {
			t.Fatal(err)
		}
		rows := document.Records
		for _, table := range document.Tables {
			if table.Table == "SpellEffect" {
				rows = append(rows, table.Records...)
			}
		}
		for _, row := range rows {
			readNumber := func(key string) float64 {
				text := string(row[key])
				var quoted string
				if json.Unmarshal(row[key], &quoted) == nil {
					text = quoted
				}
				value, err := strconv.ParseFloat(text, 64)
				if err != nil {
					t.Fatalf("%s: invalid %s: %s", filename, key, text)
				}
				return value
			}
			spellID := int32(readNumber("SpellID"))
			if !wanted[spellID] || readNumber("EffectIndex") != 0 {
				continue
			}
			spell := p.GetSpell(core.ActionID{SpellID: spellID})
			coefficient := spell.BonusCoefficient
			if len(spell.Dots()) > 0 {
				coefficient = spell.Dot(p.CurrentTarget).BonusCoefficient
			}
			closeEnough(t, fmt.Sprintf("client coefficient for %d", spellID), coefficient, readNumber("EffectBonusCoefficient"))
			seen[spellID] = true
		}
	}
	for spellID := range wanted {
		if !seen[spellID] {
			t.Errorf("no archived coefficient checked for %d", spellID)
		}
	}
}
