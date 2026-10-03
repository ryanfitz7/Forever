package priest

import (
	"testing"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
)

// Exercise each real spell's ApplyEffects and OnTick functions. Calling those
// functions directly isolates hit/proc callbacks from mana and cooldown limits;
// no synthetic ProcMask-only spell stands in for the Priest spell under test.
func TestForeverTouchOfTheGraveRealSpellApplications(t *testing.T) {
	for _, check := range []struct {
		name string
		id   int32
	}{
		{"Pain", 10894}, {"Plague", 19280}, {"MindBlast", 10947},
		{"Death", 1309636}, {"MindFlay", 18807},
	} {
		t.Run(check.name, func(t *testing.T) {
			sim, p := newPriestTestSim(t, proto.Race_RaceUndead, 60, &proto.PriestTalents{MindFlay: true}, proto.Ruleset_RulesetForever)
			spell := p.GetSpell(core.ActionID{SpellID: check.id})
			drain := p.GetSpell(core.ActionID{SpellID: 1260198})
			aura := p.GetAura("Touch of the Grave")
			if spell == nil || drain == nil || aura == nil {
				t.Fatal("missing real spell or racial registration")
			}
			original := aura.OnSpellHitDealt
			landed, missed, callbacks, ticks := 0, 0, 0, 0
			aura.OnSpellHitDealt = func(aura *core.Aura, sim *core.Simulation, hitSpell *core.Spell, result *core.SpellResult) {
				before := drain.SpellMetrics[p.CurrentTarget.UnitIndex].Casts
				if hitSpell == spell {
					callbacks++
					if result.Landed() {
						landed++
					} else {
						missed++
					}
				}
				original(aura, sim, hitSpell, result)
				if hitSpell == spell && !result.Landed() && drain.SpellMetrics[p.CurrentTarget.UnitIndex].Casts != before {
					t.Fatal("a missed application triggered Touch of the Grave")
				}
			}
			const attempts = 10000
			for i := 0; i < attempts; i++ {
				// Separate applications by more than the racial's one-second recovery.
				sim.CurrentTime += 30 * time.Second
				spell.ApplyEffects(sim, p.CurrentTarget, spell)
				var dot *core.Dot
				if len(spell.Dots()) > 0 {
					dot = spell.CurDot()
				}
				if dot != nil && dot.IsActive() {
					before := drain.SpellMetrics[p.CurrentTarget.UnitIndex].Casts
					beforeCallbacks := callbacks
					for tick := int32(0); tick < dot.NumberOfTicks; tick++ {
						sim.CurrentTime += dot.TickLength
						dot.TickOnce(sim)
						ticks++
					}
					dot.Deactivate(sim)
					if drain.SpellMetrics[p.CurrentTarget.UnitIndex].Casts != before || callbacks != beforeCallbacks {
						t.Fatal("periodic damage supplied an extra racial application opportunity")
					}
				}
			}
			if callbacks != attempts || missed == 0 || landed == 0 {
				t.Fatalf("expected one callback per real spell application: callbacks=%d landed=%d missed=%d", callbacks, landed, missed)
			}
			procs := drain.SpellMetrics[p.CurrentTarget.UnitIndex].Casts
			rate := float64(procs) / float64(landed)
			if rate < .08 || rate > .12 {
				t.Fatalf("%d procs from %d landed applications (%.2f%%), expected approximately 10%%", procs, landed, rate*100)
			}
			t.Logf("applications=%d landed=%d missed=%d procs=%d rate=%.2f%% periodic ticks=%d (no extra procs)", attempts, landed, missed, procs, rate*100, ticks)
		})
	}
}

func TestForeverStarshardsApplicationCallbacksAndRace(t *testing.T) {
	_, undead := newPriestTestSim(t, proto.Race_RaceUndead, 60, &proto.PriestTalents{}, proto.Ruleset_RulesetForever)
	if undead.GetSpell(core.ActionID{SpellID: 19305}) != nil {
		t.Fatal("Undead must not gain Night Elf Starshards")
	}
	sim, p := newPriestTestSim(t, proto.Race_RaceNightElf, 60, &proto.PriestTalents{}, proto.Ruleset_RulesetForever)
	spell := p.GetSpell(core.ActionID{SpellID: 19305})
	if spell == nil || p.GetSpell(core.ActionID{SpellID: 1260198}) != nil {
		t.Fatal("Starshards and Touch of the Grave must remain race-exclusive")
	}
	// Reuse the registered, inactive racial aura as a test observer so the actual
	// Starshards application and tick callbacks can be checked without inventing
	// an impossible Night Elf/Undead racial combination.
	observer := p.GetAura("Elune's Light")
	applications, landed, periodic := 0, 0, 0
	observer.OnSpellHitDealt = func(_ *core.Aura, _ *core.Simulation, hitSpell *core.Spell, result *core.SpellResult) {
		if hitSpell == spell {
			applications++
			if result.Landed() {
				landed++
			}
		}
	}
	observer.OnPeriodicDamageDealt = func(_ *core.Aura, _ *core.Simulation, hitSpell *core.Spell, _ *core.SpellResult) {
		if hitSpell == spell {
			periodic++
		}
	}
	observer.Activate(sim)
	const attempts = 1000
	for i := 0; i < attempts; i++ {
		sim.CurrentTime += 10 * time.Second
		spell.ApplyEffects(sim, p.CurrentTarget, spell)
		dot := spell.CurDot()
		if dot.IsActive() {
			for tick := int32(0); tick < dot.NumberOfTicks; tick++ {
				dot.TickOnce(sim)
			}
			dot.Deactivate(sim)
		}
	}
	if applications != attempts || periodic != landed*6 {
		t.Fatalf("Starshards callbacks: applications=%d landed=%d ticks=%d", applications, landed, periodic)
	}
	t.Logf("Starshards: %d applications, %d landed, %d periodic callbacks; unavailable to Undead", applications, landed, periodic)
}

func TestForeverTouchOfTheGraveFixedHealthDamage(t *testing.T) {
	sim, p := newPriestTestSim(t, proto.Race_RaceUndead, 60, &proto.PriestTalents{}, proto.Ruleset_RulesetForever)
	p.AddStatDynamic(sim, stats.Health, 3477-p.MaxHealth())
	drain := p.GetSpell(core.ActionID{SpellID: 1260198})
	// Isolate the base damage from the separate, still-assumed hit/mitigation
	// model. A very high hit bonus retains the core's irreducible 1% miss floor.
	drain.Flags |= core.SpellFlagIgnoreResists
	drain.BonusHitRating = 1000 * core.SpellHitRatingPerHitChance
	for i := 0; i < 1000; i++ {
		drain.Cast(sim, p.CurrentTarget)
	}
	metrics := drain.SpellMetrics[p.CurrentTarget.UnitIndex]
	if metrics.Hits <= 0 || metrics.Crits != 0 {
		t.Fatal("health leech must land and cannot crit")
	}
	closeEnough(t, "caster maximum health", p.MaxHealth(), 3477)
	closeEnough(t, "fixed five-percent damage per landed proc", metrics.TotalDamage/float64(metrics.Hits), 173.85)
	t.Logf("3477 HP: %.2f damage per landed unmodified proc; hits=%d misses=%d", metrics.TotalDamage/float64(metrics.Hits), metrics.Hits, metrics.Misses)
}
