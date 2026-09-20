package core

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/simsignals"
	"github.com/wowsims/classic/sim/core/stats"
)

func init() {
	RegisterAgentFactory(proto.Player_SmitePriest{}, proto.Spec_SpecSmitePriest,
		func(character *Character, _ *proto.Player) Agent {
			agent := &FakeAgent{Character: *character}
			agent.EnableManaBar()
			agent.AddStat(stats.Mana, 10000)
			agent.Init = func() {
				for _, id := range []int32{900001, 900002, 900003} {
					config := SpellConfig{
						ActionID: ActionID{SpellID: id}, SpellSchool: SpellSchoolShadow,
						DefenseType: DefenseTypeMagic, ProcMask: ProcMaskSpellDamage,
						Flags: SpellFlagIgnoreResists, DamageMultiplier: 1, ThreatMultiplier: 1,
						ManaCost: ManaCostOptions{FlatCost: 100},
						Cast:     CastConfig{DefaultCast: Cast{GCD: time.Nanosecond}},
						ApplyEffects: func(sim *Simulation, target *Unit, spell *Spell) {
							spell.CalcAndDealDamage(sim, target, 100, spell.OutcomeAlwaysHit)
						},
					}
					if id == 900002 {
						config.Dot = DotConfig{
							Aura: Aura{Label: "Eureka test periodic"}, NumberOfTicks: 6, TickLength: time.Second,
							OnSnapshot: func(sim *Simulation, target *Unit, dot *Dot, rollover bool) { dot.Snapshot(target, 100, rollover) },
							OnTick: func(sim *Simulation, target *Unit, dot *Dot) {
								dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
							},
						}
						config.ApplyEffects = func(sim *Simulation, target *Unit, spell *Spell) {
							spell.Dot(target).Apply(sim)
							spell.CalcAndDealOutcome(sim, target, spell.OutcomeAlwaysHit)
						}
					}
					if id == 900003 {
						// A damage proc has no mana cost and must neither benefit nor consume.
						config.ManaCost = ManaCostOptions{}
						config.Cast = CastConfig{}
					}
					agent.RegisterSpell(config)
				}
			}
			return agent
		},
		func(player *proto.Player, spec interface{}) { player.Spec = spec.(*proto.Player_SmitePriest) })
}

func setupRacialTestSim(race proto.Race, class proto.Class, ruleset proto.Ruleset) (*Simulation, *Character) {
	sim := NewSim(&proto.RaidSimRequest{
		SimOptions: &proto.SimOptions{Ruleset: ruleset, RandomSeed: 100, Interactive: true},
		Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{{
			Name: "Racial test", Class: class, Race: race,
			Spec:      &proto.Player_SmitePriest{SmitePriest: &proto.SmitePriest{}},
			Equipment: &proto.EquipmentSpec{},
		}}}}},
		Encounter: &proto.Encounter{Targets: []*proto.Target{{Name: "target", Level: 60}}, Duration: 180},
	}, simsignals.CreateSignals())
	sim.Reset()
	return sim, sim.Raid.Parties[0].Players[0].GetCharacter()
}

func requireRacialValue(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.000001 {
		t.Fatalf("%s: got %.9f, want %.9f", name, got, want)
	}
}

func TestForeverPriestEurekaChargesAndCost(t *testing.T) {
	sim, character := setupRacialTestSim(proto.Race_RaceGnome, proto.Class_ClassPriest, proto.Ruleset_RulesetForever)
	eureka := character.GetSpell(ActionID{SpellID: 1259823})
	if eureka == nil || character.GetSpell(ActionID{SpellID: 460550}) != nil {
		t.Fatal("Priest must use the class-specific Eureka spell")
	}
	damage := character.GetSpell(ActionID{SpellID: 900001})
	proc := character.GetSpell(ActionID{SpellID: 900003})
	aura := character.GetAura("Eureka!")
	// School and spell discounts must stack multiplicatively with Eureka.
	character.PseudoStats.SchoolCostMultiplier[stats.SchoolIndexShadow] = 50
	damage.Cost.Multiplier = 90
	if !eureka.Cast(sim, character.CurrentTarget) {
		t.Fatal("Eureka cast failed")
	}
	requireRacialValue(t, "discounted mana", damage.Cost.GetCurrentCost(), 38.25)
	// An Inner Focus style full discount remains free and restores correctly.
	damage.Cost.Multiplier -= 100
	requireRacialValue(t, "free spell", damage.Cost.GetCurrentCost(), 0)
	damage.Cost.Multiplier += 100
	proc.Cast(sim, character.CurrentTarget)
	if aura.GetStacks() != 3 {
		t.Fatal("ineligible proc spent a charge")
	}
	requireRacialValue(t, "ineligible proc damage", proc.SpellMetrics[0].TotalDamage, 100)
	for remaining := int32(2); remaining >= 0; remaining-- {
		if !damage.Cast(sim, character.CurrentTarget) {
			t.Fatal("damaging cast failed")
		}
		if aura.GetStacks() != remaining {
			t.Fatalf("charges: got %d, want %d", aura.GetStacks(), remaining)
		}
		sim.CurrentTime += 2 * time.Second
	}
	// Includes the instant cast at the activation timestamp and the final charged hit.
	requireRacialValue(t, "three charged hits", damage.SpellMetrics[0].TotalDamage, 330)
	requireRacialValue(t, "restored mana", damage.Cost.GetCurrentCost(), 45)
	requireRacialValue(t, "restored damage", damage.DynamicDamageMultiplier, 1)
	if aura.IsActive() {
		t.Fatal("Eureka remained active after its third charge")
	}
}

func TestForeverPriestEurekaPeriodicDamageIsDynamic(t *testing.T) {
	sim, character := setupRacialTestSim(proto.Race_RaceGnome, proto.Class_ClassPriest, proto.Ruleset_RulesetForever)
	spell := character.GetSpell(ActionID{SpellID: 900002})
	dot := spell.CurDot()
	aura := character.GetAura("Eureka!")
	activate := func() { aura.Activate(sim); aura.SetStacks(sim, 3) }
	dot.Apply(sim)
	expectDotTickDamage(t, sim, dot, 100)
	activate()
	expectDotTickDamage(t, sim, dot, 110)
	aura.Deactivate(sim)
	expectDotTickDamage(t, sim, dot, 100)

	// A DoT applied during Eureka loses only that bonus; ordinary snapshots stay.
	activate()
	spell.DamageMultiplier = 2
	dot.Apply(sim)
	expectDotTickDamage(t, sim, dot, 220)
	spell.DamageMultiplier = 1
	aura.Deactivate(sim)
	expectDotTickDamage(t, sim, dot, 200)
	dot.Rollover(sim)
	expectDotTickDamage(t, sim, dot, 200)
}

func TestForeverEurekaPreservesOtherClassesAndClassic(t *testing.T) {
	sim, character := setupRacialTestSim(proto.Race_RaceGnome, proto.Class_ClassMage, proto.Ruleset_RulesetForever)
	spell := character.GetSpell(ActionID{SpellID: 900001})
	eureka := character.GetSpell(ActionID{SpellID: 460550})
	if eureka == nil {
		t.Fatal("non-Priest Eureka missing")
	}
	eureka.Cast(sim, character.CurrentTarget)
	requireRacialValue(t, "legacy mana", spell.Cost.GetCurrentCost(), 50)
	requireRacialValue(t, "legacy damage", character.PseudoStats.DamageDealtMultiplier, 1.1)
	spell.Cast(sim, character.CurrentTarget)
	if character.GetAura("Eureka!").GetStacks() != 3 {
		t.Fatal("changed the inherited non-Priest same-timestamp charge behavior")
	}
	_, classic := setupRacialTestSim(proto.Race_RaceGnome, proto.Class_ClassPriest, proto.Ruleset_RulesetClassic)
	if classic.GetAura("Eureka!") != nil {
		t.Fatal("Classic received Eureka")
	}
}

func TestForeverTouchOfTheGraveExcludesPeriodicTicks(t *testing.T) {
	sim, character := setupRacialTestSim(proto.Race_RaceUndead, proto.Class_ClassPriest, proto.Ruleset_RulesetForever)
	spell := character.GetSpell(ActionID{SpellID: 900002})
	dot := spell.CurDot()
	drain := character.GetSpell(ActionID{SpellID: 460540})
	dot.Apply(sim)
	for i := 0; i < 1000; i++ {
		dot.TickOnce(sim)
	}
	requireRacialValue(t, "periodic-triggered drain", drain.SpellMetrics[0].TotalDamage, 0)
	// Landed application events can trigger the existing assumed 5% chance.
	for i := 0; i < 1000; i++ {
		spell.CalcAndDealOutcome(sim, character.CurrentTarget, spell.OutcomeAlwaysHit)
	}
	if drain.SpellMetrics[0].TotalDamage <= 0 {
		t.Fatal("landed applications never triggered drain")
	}
}
