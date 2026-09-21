package priest

import (
	"testing"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
)

func expectedTickTestSpell(t *testing.T, starshards bool, ruleset proto.Ruleset) (*core.Simulation, *Priest, *core.Spell) {
	t.Helper()
	sim, priest := newPriestTestSim(t, proto.Race_RaceNightElf, 60, &proto.PriestTalents{
		MindFlay: true, ImprovedMindFlay: 2, Darkness: 5, Shadowform: true,
	}, ruleset)
	priest.ShadowformAura.Activate(sim)
	priest.AddStatDynamic(sim, stats.SpellPower, 300)
	spell := priest.MindFlay[6][0]
	if starshards {
		spell = priest.Starshards[7][0]
	}
	spell.Flags |= core.SpellFlagIgnoreResists
	return sim, priest, spell
}

func dealtTestTick(sim *core.Simulation, spell *core.Spell, target *core.Unit) float64 {
	before := spell.SpellMetrics[target.UnitIndex].TotalDamage
	spell.Dot(target).TickOnce(sim)
	return spell.SpellMetrics[target.UnitIndex].TotalDamage - before
}

func TestForeverExpectedChannelTicksMatchActualDamage(t *testing.T) {
	for name, starshards := range map[string]bool{"MindFlay": false, "Starshards": true} {
		t.Run(name, func(t *testing.T) {
			sim, priest, spell := expectedTickTestSpell(t, starshards, proto.Ruleset_RulesetForever)
			target := priest.CurrentTarget
			dot := spell.Dot(target)
			dot.Apply(sim)
			baseTick := dealtTestTick(sim, spell, target)
			closeEnough(t, "fresh noncritical tick estimate", spell.ExpectedTickDamage(sim, target), baseTick)
			closeEnough(t, "stored noncritical tick estimate", spell.ExpectedTickDamageFromCurrentSnapshot(sim, target), baseTick)

			// Existing dots retain power and ordinary attacker buffs. A new cast
			// would use the larger power/buff values, so the two estimates must differ.
			priest.AddStatDynamic(sim, stats.SpellPower, 200)
			priest.PseudoStats.DamageDealtMultiplier *= 1.2
			// Eureka's separate dynamic multiplier affects even the existing snapshot.
			spell.DynamicDamageMultiplier *= 1.1
			normalTick := dealtTestTick(sim, spell, target)
			closeEnough(t, "ordinary snapshot with live damage multiplier", normalTick, baseTick*1.1)
			closeEnough(t, "stored estimate retains original power and buffs", spell.ExpectedTickDamageFromCurrentSnapshot(sim, target), normalTick)
			freshTick := spell.ExpectedTickDamage(sim, target)
			if freshTick <= normalTick {
				t.Fatal("fresh estimate did not use current power and ordinary damage buffs")
			}

			// Crit is live, not the zero chance stored when this dot was applied.
			priest.AddStatDynamic(sim, stats.SpellCrit, 100*core.SpellCritRatingPerCritChance)
			criticalTick := dealtTestTick(sim, spell, target)
			closeEnough(t, "stored estimate uses current guaranteed crit", spell.ExpectedTickDamageFromCurrentSnapshot(sim, target), criticalTick)
			critMultiplier := 2.0
			if starshards {
				critMultiplier = 1.5
			}
			closeEnough(t, "school-specific critical multiplier", criticalTick, normalTick*critMultiplier)
			priest.AddStatDynamic(sim, stats.SpellCrit, -60*core.SpellCritRatingPerCritChance)
			closeEnough(t, "40 percent crit is weighted actual outcomes", spell.ExpectedTickDamageFromCurrentSnapshot(sim, target), .6*normalTick+.4*criticalTick)
			closeEnough(t, "fresh estimate also includes current crit", spell.ExpectedTickDamage(sim, target), freshTick*(.6+.4*critMultiplier))

			// Explicit noncritical periodic effects follow the actual tick flag.
			spell.Flags |= core.SpellFlagNoPeriodicCrit
			closeEnough(t, "noncritical flag stored estimate", spell.ExpectedTickDamageFromCurrentSnapshot(sim, target), normalTick)
			closeEnough(t, "noncritical flag actual tick", dealtTestTick(sim, spell, target), normalTick)
			closeEnough(t, "noncritical flag fresh estimate", spell.ExpectedTickDamage(sim, target), freshTick)

			dot.Apply(sim)
			closeEnough(t, "new application matches prospective estimate", dealtTestTick(sim, spell, target), freshTick)
		})
	}
}

func TestClassicExpectedChannelTickSemanticsRemainUnchanged(t *testing.T) {
	for name, starshards := range map[string]bool{"MindFlay": false, "Starshards": true} {
		t.Run(name, func(t *testing.T) {
			sim, priest, spell := expectedTickTestSpell(t, starshards, proto.Ruleset_RulesetClassic)
			target := priest.CurrentTarget
			normalTick := spell.ExpectedTickDamage(sim, target)
			spell.Dot(target).Apply(sim)
			priest.AddStatDynamic(sim, stats.SpellCrit, 100*core.SpellCritRatingPerCritChance)
			closeEnough(t, "Classic estimate excludes periodic crit", spell.ExpectedTickDamage(sim, target), normalTick)
			closeEnough(t, "Classic actual tick excludes periodic crit", dealtTestTick(sim, spell, target), normalTick)
			priest.AddStatDynamic(sim, stats.SpellPower, 200)
			freshTick := spell.ExpectedTickDamage(sim, target)
			if freshTick <= normalTick {
				t.Fatal("Classic prospective estimate did not use current spell power")
			}
			// Keep the pre-existing Classic estimate path, which ignores useSnapshot.
			closeEnough(t, "Classic snapshot argument preserves previous behavior", spell.ExpectedTickDamageFromCurrentSnapshot(sim, target), freshTick)
		})
	}
}
