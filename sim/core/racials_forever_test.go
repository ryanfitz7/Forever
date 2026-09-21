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
				for _, id := range []int32{900001, 900002, 900003, 900004} {
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
					if id == 900004 {
						config.Cast.DefaultCast.CastTime = 1500 * time.Millisecond
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
	drain := character.GetSpell(ActionID{SpellID: 1260198})
	dot.Apply(sim)
	for i := 0; i < 1000; i++ {
		dot.TickOnce(sim)
	}
	requireRacialValue(t, "periodic-triggered drain", drain.SpellMetrics[0].TotalDamage, 0)
	// Landed application events can trigger the Priest client's 10% chance.
	for i := 0; i < 1000; i++ {
		spell.CalcAndDealOutcome(sim, character.CurrentTarget, spell.OutcomeAlwaysHit)
	}
	if drain.SpellMetrics[0].TotalDamage <= 0 {
		t.Fatal("landed applications never triggered drain")
	}
}

func TestForeverPriestTouchOfTheGraveRecoveryAndChance(t *testing.T) {
	sim, character := setupRacialTestSim(proto.Race_RaceUndead, proto.Class_ClassPriest, proto.Ruleset_RulesetForever)
	drain := character.GetSpell(ActionID{SpellID: 1260198})
	if drain == nil || character.GetAura("Touch of the Grave").ActionID.SpellID != 1260201 {
		t.Fatal("Priest Touch of the Grave must use its client aura and damage IDs")
	}
	application := character.GetSpell(ActionID{SpellID: 900001})
	applyMany := func(count int) {
		for i := 0; i < count; i++ {
			application.CalcAndDealOutcome(sim, character.CurrentTarget, application.OutcomeAlwaysHit)
		}
	}
	applyMany(1000)
	if got := drain.SpellMetrics[0].Casts; got != 1 {
		t.Fatalf("same-timestamp applications procced %d times, want 1", got)
	}
	sim.CurrentTime += 999 * time.Millisecond
	applyMany(1000)
	if drain.SpellMetrics[0].Casts != 1 {
		t.Fatal("Touch of the Grave ignored its one-second recovery")
	}
	sim.CurrentTime += time.Millisecond
	applyMany(1000)
	if drain.SpellMetrics[0].Casts != 2 {
		t.Fatal("Touch of the Grave failed to recover at one second")
	}
	// A triggered damage proc must not create additional application opportunities.
	proc := character.GetSpell(ActionID{SpellID: 900003})
	proc.ProcMask = ProcMaskEmpty
	sim.CurrentTime += time.Second
	for i := 0; i < 1000; i++ {
		proc.CalcAndDealDamage(sim, character.CurrentTarget, 1, proc.OutcomeAlwaysHit)
	}
	if drain.SpellMetrics[0].Casts != 2 {
		t.Fatal("Touch of the Grave triggered from another passive damage proc")
	}
	before := drain.SpellMetrics[0].Casts
	for i := 0; i < 10000; i++ {
		sim.CurrentTime += time.Second
		applyMany(1)
	}
	// Fixed seed, wide bounds: distinguishes the caster variant's 10% from the
	// physical variant's 5% without depending on an exact PRNG sequence.
	if procs := drain.SpellMetrics[0].Casts - before; procs < 800 || procs > 1200 {
		t.Fatalf("got %d procs in 10000 eligible applications, expected about 1000", procs)
	}
	_, other := setupRacialTestSim(proto.Race_RaceUndead, proto.Class_ClassMage, proto.Ruleset_RulesetForever)
	if other.GetSpell(ActionID{SpellID: 460540}) == nil {
		t.Fatal("changed the unaudited non-Priest racial registration")
	}
}

func TestForeverElunesLightCritAndRecovery(t *testing.T) {
	sim, character := setupRacialTestSim(proto.Race_RaceNightElf, proto.Class_ClassPriest, proto.Ruleset_RulesetForever)
	spell := character.GetSpell(ActionID{SpellID: 1259799})
	aura := character.GetAura("Elune's Light")
	if spell == nil || aura == nil || aura.Duration != 15*time.Second || spell.CD.Duration != 3*time.Minute {
		t.Fatal("Elune's Light registration does not match the client ID and timings")
	}
	crit := character.GetStat(stats.SpellCrit)
	spell.Cast(sim, character.CurrentTarget)
	requireRacialValue(t, "Elune crit", character.GetStat(stats.SpellCrit)-crit, 10*SpellCritRatingPerCritChance)
	if spell.IsReady(sim) {
		t.Fatal("Elune's Light did not start its cooldown")
	}
	aura.Deactivate(sim)
	requireRacialValue(t, "Elune crit after expiry", character.GetStat(stats.SpellCrit), crit)
	sim.CurrentTime = 3 * time.Minute
	if !spell.IsReady(sim) {
		t.Fatal("Elune's Light failed to recover after three minutes")
	}
}

func TestForeverPriestBerserkingSpeedCostAndRecovery(t *testing.T) {
	sim, character := setupRacialTestSim(proto.Race_RaceTroll, proto.Class_ClassPriest, proto.Ruleset_RulesetForever)
	berserking := character.GetSpell(ActionID{SpellID: 20554})
	if berserking == nil || berserking.Cost != nil || berserking.CD.Duration != 3*time.Minute {
		t.Fatal("Priest Berserking must use client ID 20554, no resource cost, and 180-second cooldown")
	}
	hardcast := character.GetSpell(ActionID{SpellID: 900004})
	before := hardcast.CastTime()
	mana := character.CurrentMana()
	berserking.Cast(sim, character.CurrentTarget)
	requireRacialValue(t, "Berserking mana", character.CurrentMana(), mana)
	if got, want := hardcast.CastTime(), time.Duration(float64(before)/1.1); got != want {
		t.Fatalf("1.5-second cast while Berserking: got %s, want %s", got, want)
	}
	aura := character.GetAura("Berserking (10)")
	if aura == nil || aura.Duration != 10*time.Second {
		t.Fatal("Berserking must last ten seconds")
	}
	aura.Deactivate(sim)
	if hardcast.CastTime() != before {
		t.Fatal("Berserking speed did not restore on expiry")
	}
	_, classic := setupRacialTestSim(proto.Race_RaceTroll, proto.Class_ClassPriest, proto.Ruleset_RulesetClassic)
	if classic.GetSpell(ActionID{SpellID: 26297}) == nil || classic.GetSpell(ActionID{SpellID: 20554}) != nil {
		t.Fatal("changed Classic Berserking registration")
	}
	_, other := setupRacialTestSim(proto.Race_RaceTroll, proto.Class_ClassMage, proto.Ruleset_RulesetForever)
	if other.GetSpell(ActionID{SpellID: 26297, Tag: 2}) == nil {
		t.Fatal("changed the unaudited non-Priest Berserking registration")
	}
}
