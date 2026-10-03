package priest

import (
	"testing"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
)

func directBonusTestPriest(t *testing.T, race proto.Race, ruleset proto.Ruleset) (*core.Simulation, *Priest) {
	t.Helper()
	sim, p := newPriestTestSim(t, race, 60, &proto.PriestTalents{MindFlay: true, InnerFocus: true, Shadowform: true}, ruleset)
	p.AddStatDynamic(sim, stats.SpellPower, 300)
	p.ShadowformAura.Activate(sim)
	return sim, p
}

func directBonusTestSpell(p *Priest, name string) *core.Spell {
	switch name {
	case "Pain":
		return p.ShadowWordPain[8]
	case "Plague":
		return p.DevouringPlague[6]
	case "MindFlay":
		return p.MindFlay[6][0]
	case "Starshards":
		return p.Starshards[7][0]
	case "HolyFire":
		return p.HolyFire[8]
	case "MindBlast":
		return p.MindBlast[9]
	case "Death":
		return p.ShadowWordDeath[4]
	default:
		panic("unknown test spell: " + name)
	}
}

func activateTestEureka(t *testing.T, sim *core.Simulation, p *Priest) *core.Aura {
	t.Helper()
	spell := p.GetSpell(core.ActionID{SpellID: 1259823})
	if spell == nil || !spell.Cast(sim, p.CurrentTarget) {
		t.Fatal("Eureka activation failed")
	}
	return p.GetAura("Eureka!")
}

func TestForeverDirectBonusesExcludePeriodicDamageAndCrit(t *testing.T) {
	for _, name := range []string{"Pain", "Plague", "MindFlay", "HolyFire", "Starshards"} {
		t.Run(name, func(t *testing.T) {
			race := proto.Race_RaceGnome
			if name == "Starshards" {
				race = proto.Race_RaceNightElf
			}
			sim, p := directBonusTestPriest(t, race, proto.Ruleset_RulesetForever)
			spell, target := directBonusTestSpell(p, name), p.CurrentTarget
			spell.Flags |= core.SpellFlagIgnoreResists
			dot := spell.Dot(target)
			dot.Apply(sim)
			normalTick := dealtTestTick(sim, spell, target)
			baseCost := spell.Cost.GetCurrentCost()
			var eureka *core.Aura
			if race == proto.Race_RaceGnome {
				eureka = activateTestEureka(t, sim, p)
				closeEnough(t, "periodic spell still receives mana discount", spell.Cost.GetCurrentCost(), baseCost*.85)
			}
			if !p.InnerFocus.Cast(sim, target) {
				t.Fatal("Inner Focus activation failed")
			}
			closeEnough(t, "Inner Focus still makes periodic spell free", spell.Cost.GetCurrentCost(), 0)
			closeEnough(t, "Inner Focus direct crit remains active", p.MindBlast[9].SpellCritChance(target), .25)
			closeEnough(t, "periodic crit excludes Inner Focus", spell.PeriodicSpellCritChance(target), 0)
			// Include both a pre-existing DoT and a fresh application while the
			// buffs are held; application must not snapshot either direct bonus.
			for application := 0; application < 2; application++ {
				if application == 1 {
					dot.Apply(sim)
				}
				closeEnough(t, "snapshot excludes Inner Focus crit", dot.SnapshotCritChance, 0)
				for tick := 0; tick < 16; tick++ {
					closeEnough(t, "actual periodic damage with direct bonuses held", dealtTestTick(sim, spell, target), normalTick)
				}
				if name == "MindFlay" || name == "Starshards" {
					closeEnough(t, "fresh channel estimate excludes direct bonuses", spell.ExpectedTickDamage(sim, target), normalTick)
					closeEnough(t, "stored channel estimate excludes direct bonuses", spell.ExpectedTickDamageFromCurrentSnapshot(sim, target), normalTick)
				}
			}
			if spell.SpellMetrics[target.UnitIndex].CritTicks != 0 || !p.InnerFocusAura.IsActive() {
				t.Fatal("ticks gained Inner Focus crit or consumed its aura")
			}
			if eureka != nil && eureka.GetStacks() != 3 {
				t.Fatal("periodic ticks consumed Eureka charges")
			}
			// Ordinary crit remains live on an existing DoT and still uses the
			// appropriate Shadowform or ordinary magic critical multiplier.
			p.AddStatDynamic(sim, stats.SpellCrit, 100*core.SpellCritRatingPerCritChance)
			criticalTick := normalTick * spell.CritMultiplier(p.AttackTables[target.UnitIndex][spell.CastType])
			closeEnough(t, "ordinary periodic crit retained", dealtTestTick(sim, spell, target), criticalTick)
			if name == "MindFlay" || name == "Starshards" {
				closeEnough(t, "channel estimate retains ordinary crit", spell.ExpectedTickDamageFromCurrentSnapshot(sim, target), criticalTick)
			}
			p.InnerFocusAura.Deactivate(sim)
			if eureka != nil {
				eureka.Deactivate(sim)
			}
			closeEnough(t, "periodic crit after direct buffs expire", dealtTestTick(sim, spell, target), criticalTick)
			closeEnough(t, "mana cost restored", spell.Cost.GetCurrentCost(), baseCost)
		})
	}
}

func TestForeverDirectBonusesRetainDirectDamageAndCrit(t *testing.T) {
	for _, name := range []string{"MindBlast", "Death", "HolyFire"} {
		t.Run(name, func(t *testing.T) {
			sim, p := directBonusTestPriest(t, proto.Race_RaceGnome, proto.Ruleset_RulesetForever)
			spell, target := directBonusTestSpell(p, name), p.CurrentTarget
			spell.Flags |= core.SpellFlagIgnoreResists
			damage := func() float64 { return spell.CalcDamage(sim, target, 200, spell.OutcomeExpectedMagicCrit).Damage }
			baseline := damage()
			eureka := activateTestEureka(t, sim, p)
			closeEnough(t, "direct Eureka damage", damage(), baseline*1.1)
			if !p.InnerFocus.Cast(sim, target) {
				t.Fatal("Inner Focus activation failed")
			}
			critMultiplier := spell.CritMultiplier(p.AttackTables[target.UnitIndex][spell.CastType])
			closeEnough(t, "direct Eureka damage and Inner Focus crit", damage(), baseline*1.1*(1+.25*(critMultiplier-1)))
			p.InnerFocusAura.Deactivate(sim)
			closeEnough(t, "direct crit expires independently", damage(), baseline*1.1)
			eureka.Deactivate(sim)
			closeEnough(t, "direct damage returns to baseline", damage(), baseline)
		})
	}
}

func TestForeverDirectBonusesFreeCastConsumesAurasAtCompletion(t *testing.T) {
	for _, name := range []string{"Pain", "Plague", "MindFlay", "MindBlast", "Death", "HolyFire"} {
		t.Run(name, func(t *testing.T) {
			sim, p := directBonusTestPriest(t, proto.Race_RaceGnome, proto.Ruleset_RulesetForever)
			spell, target := directBonusTestSpell(p, name), p.CurrentTarget
			baseCost, mana := spell.Cost.GetCurrentCost(), p.CurrentMana()
			eureka := activateTestEureka(t, sim, p)
			if !p.InnerFocus.Cast(sim, target) || !spell.Cast(sim, target) {
				t.Fatal("free spell cast failed")
			}
			if spell.DefaultCast.CastTime > 0 {
				if !p.InnerFocusAura.IsActive() || eureka.GetStacks() != 3 {
					t.Fatal("bonuses consumed before hard cast completed")
				}
				for p.InnerFocusAura.IsActive() {
					if sim.Step() {
						t.Fatal("hard cast never completed")
					}
				}
			}
			if p.InnerFocusAura.IsActive() || eureka.GetStacks() != 2 {
				t.Fatal("completed cast did not consume Inner Focus and one Eureka charge")
			}
			closeEnough(t, "actual cast spends no mana", p.CurrentMana(), mana)
			closeEnough(t, "remaining Eureka mana discount", spell.Cost.GetCurrentCost(), baseCost*.85)
			closeEnough(t, "Inner Focus crit removed after consumption", spell.SpellCritChance(target), 0)
			if p.InnerFocus.TimeToReady(sim) != 3*time.Minute {
				t.Fatalf("Inner Focus cooldown at consumption: %s", p.InnerFocus.TimeToReady(sim))
			}
		})
	}
}

func TestClassicInnerFocusBonusesRemainUnchanged(t *testing.T) {
	sim, p := directBonusTestPriest(t, proto.Race_RaceGnome, proto.Ruleset_RulesetClassic)
	if p.GetSpell(core.ActionID{SpellID: 1259823}) != nil {
		t.Fatal("Forever Eureka registered in Classic")
	}
	spell, target := p.ShadowWordPain[8], p.CurrentTarget
	spell.Flags |= core.SpellFlagIgnoreResists
	spell.Dot(target).Apply(sim)
	normalTick := dealtTestTick(sim, spell, target)
	if !p.InnerFocus.Cast(sim, target) {
		t.Fatal("Inner Focus activation failed")
	}
	closeEnough(t, "Classic general crit bonus retained", spell.BonusCritRating, 25*core.SpellCritRatingPerCritChance)
	closeEnough(t, "Classic direct-only field unused", spell.BonusDirectCritRating, 0)
	closeEnough(t, "Classic direct crit bonus retained", p.MindBlast[9].SpellCritChance(target), .25)
	closeEnough(t, "Classic periodic ticks remain noncritical", dealtTestTick(sim, spell, target), normalTick)
	closeEnough(t, "Classic free spell retained", spell.Cost.GetCurrentCost(), 0)
	if !spell.Cast(sim, target) || p.InnerFocusAura.IsActive() {
		t.Fatal("Classic Inner Focus was not consumed by cast")
	}
	closeEnough(t, "Classic crit removed after consumption", spell.BonusCritRating, 0)
}
