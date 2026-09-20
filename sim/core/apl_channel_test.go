package core

import (
	"testing"
	"time"

	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/simsignals"
)

func init() {
	RegisterAgentFactory(proto.Player_Mage{}, proto.Spec_SpecMage,
		func(character *Character, _ *proto.Player) Agent {
			agent := &FakeAgent{Character: *character}
			agent.Init = func() { registerChannelTestSpells(&agent.Character) }
			return agent
		},
		func(player *proto.Player, spec interface{}) { player.Spec = spec.(*proto.Player_Mage) })
}

func registerChannelTestSpells(character *Character) {
	character.RegisterSpell(SpellConfig{
		ActionID: ActionID{SpellID: 900011}, Flags: SpellFlagAPL,
		Cast: CastConfig{
			DefaultCast: Cast{GCD: GCDDefault},
			CD:          Cooldown{Timer: character.NewTimer(), Duration: 10 * time.Second},
		},
	})
	character.RegisterSpell(SpellConfig{
		ActionID: ActionID{SpellID: 900010}, Flags: SpellFlagAPL | SpellFlagChanneled | SpellFlagIgnoreResists,
		SpellSchool: SpellSchoolShadow, ProcMask: ProcMaskSpellDamage,
		DamageMultiplier: 1, ThreatMultiplier: 1,
		Cast: CastConfig{DefaultCast: Cast{GCD: GCDDefault}},
		Dot: DotConfig{
			Aura: Aura{Label: "Channel interruption test"}, NumberOfTicks: 3, TickLength: time.Second,
			OnSnapshot: func(sim *Simulation, target *Unit, dot *Dot, rollover bool) { dot.Snapshot(target, 100, rollover) },
			OnTick: func(sim *Simulation, target *Unit, dot *Dot) {
				dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
			},
		},
		ApplyEffects: func(sim *Simulation, target *Unit, spell *Spell) { spell.Dot(target).Apply(sim) },
	})
}

// Uses an ordinary three-tick channel and a higher-priority spell that becomes
// ready during it. The JSON matches the predicates used by the Priest preset.
func setupChannelInterruptTest(t *testing.T) (*Simulation, *Character, *Spell, *Spell) {
	t.Helper()
	sim := NewSim(&proto.RaidSimRequest{
		SimOptions: &proto.SimOptions{RandomSeed: 100},
		Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{{
			Name: "Channel test", Class: proto.Class_ClassMage,
			Spec: &proto.Player_Mage{Mage: &proto.Mage{}}, Equipment: &proto.EquipmentSpec{},
		}}}}},
		Encounter: &proto.Encounter{Targets: []*proto.Target{{Name: "target", Level: 60}}, Duration: 180},
	}, simsignals.CreateSignals())
	sim.Reset()
	character := sim.Raid.Parties[0].Players[0].GetCharacter()
	character.ChannelClipDelay = 250 * time.Millisecond
	higher := character.GetSpell(ActionID{SpellID: 900011})
	channel := character.GetSpell(ActionID{SpellID: 900010})
	character.Rotation = character.newAPLRotation(APLRotationFromJsonString(`{
		"type":"TypeAPL", "priorityList":[
			{"action":{"castSpell":{"spellId":{"spellId":900011}}}},
			{"action":{"channelSpell":{"spellId":{"spellId":900010},"interruptIf":{"and":{"vals":[
				{"cmp":{"op":"OpGe","lhs":{"spellChanneledTicks":{"spellId":{"spellId":900010}}},"rhs":{"const":{"val":"2"}}}},
				{"spellCanCast":{"spellId":{"spellId":900011}}}
			]}}}}}
		]
	}`))
	for _, warnings := range character.Rotation.priorityListWarnings {
		if len(warnings) != 0 {
			t.Fatalf("APL warnings: %v", warnings)
		}
	}
	higher.CD.Set(time.Second)
	character.Rotation.DoNextAction(sim)
	if character.ChanneledDot != channel.CurDot() {
		t.Fatal("rotation did not start the channel")
	}
	return sim, character, channel, higher
}

func TestAPLChannelInterruptAtWholeTick(t *testing.T) {
	sim, character, channel, higher := setupChannelInterruptTest(t)
	dot := channel.CurDot()
	higher.CD.Set(0)
	if character.Rotation.shouldInterruptChannel(sim) {
		t.Fatal("clipped before the first tick")
	}
	sim.CurrentTime = time.Second
	dot.tickAction.OnAction(sim)
	if character.ChanneledDot != dot || dot.TickCount != 1 {
		t.Fatal("clipped before the second tick")
	}
	if higher.CanCast(sim, character.CurrentTarget) {
		t.Fatal("ordinary casting bypassed the active channel")
	}
	sim.CurrentTime = 2 * time.Second
	dot.tickAction.OnAction(sim)
	if character.ChanneledDot != nil || dot.TickCount != 2 {
		t.Fatal("ready higher priority action did not clip after two ticks")
	}
	if character.Rotation.evaluatingChannelInterrupt {
		t.Fatal("prospective cast evaluation leaked outside the check")
	}
	if character.NextGCDAt() != sim.CurrentTime+character.ChannelClipDelay {
		t.Fatal("channel clip delay was not honored")
	}
	if higher.CanCast(sim, character.CurrentTarget) {
		t.Fatal("next action ignored the channel clip delay")
	}
	sim.CurrentTime += character.ChannelClipDelay
	character.Rotation.DoNextAction(sim)
	if higher.SpellMetrics[0].Casts != 1 {
		t.Fatal("higher priority action was not cast after the interruption")
	}
	requireRacialValue(t, "completed channel tick damage", channel.SpellMetrics[0].TotalDamage, 200)
}

func TestAPLChannelInterruptRespectsGCDAndCooldown(t *testing.T) {
	sim, character, channel, higher := setupChannelInterruptTest(t)
	dot := channel.CurDot()
	sim.CurrentTime = 2 * time.Second
	dot.TickCount = 2
	character.GCD.Set(3 * time.Second)
	if character.Rotation.shouldInterruptChannel(sim) {
		t.Fatal("interrupted while the next action was GCD blocked")
	}
	character.GCD.Set(0)
	higher.CD.Set(3 * time.Second)
	if character.Rotation.shouldInterruptChannel(sim) {
		t.Fatal("interrupted while the next action was on cooldown")
	}
	higher.CD.Set(0)
	if !character.Rotation.shouldInterruptChannel(sim) {
		t.Fatal("did not recognize the ready action")
	}
	if character.ChanneledDot != dot || higher.CanCast(sim, character.CurrentTarget) {
		t.Fatal("eligibility check changed channel state or leaked its bypass")
	}
}
