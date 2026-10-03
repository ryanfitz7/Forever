package priest

import (
	"fmt"
	"time"

	"github.com/wowsims/classic/sim/core"
)

const DevouringPlagueRanks = 6

var DevouringPlagueSpellId = [DevouringPlagueRanks + 1]int32{0, 2944, 19276, 19277, 19278, 19279, 19280}

// Forever beta client 1.60.1.69893.
var DevouringPlagueBaseDamage = [DevouringPlagueRanks + 1]float64{0, 128, 232, 344, 488, 656, 848}
var DevouringPlagueManaCost = [DevouringPlagueRanks + 1]float64{0, 215, 350, 495, 645, 810, 985}
var DevouringPlagueLevel = [DevouringPlagueRanks + 1]int{0, 20, 28, 36, 44, 52, 60}

func (priest *Priest) registerDevouringPlagueSpell() {
	//TO DO: Implement race requirement
	priest.DevouringPlague = make([]*core.Spell, DevouringPlagueRanks+1)
	cdTimer := priest.NewTimer()

	for rank := 1; rank <= DevouringPlagueRanks; rank++ {
		config := priest.getDevouringPlagueConfig(rank, cdTimer)

		if config.RequiredLevel <= int(priest.Level) {
			priest.DevouringPlague[rank] = priest.GetOrRegisterSpell(config)
		}
	}
}

func (priest *Priest) getDevouringPlagueConfig(rank int, cdTimer *core.Timer) core.SpellConfig {

	var ticks int32 = 8

	spellId := DevouringPlagueSpellId[rank]
	baseDotDamage := (DevouringPlagueBaseDamage[rank] / float64(ticks))
	manaCost := DevouringPlagueManaCost[rank]
	level := DevouringPlagueLevel[rank]

	spellCoeff := 0.1 // per tick
	healthMetrics := priest.NewHealthMetrics(core.ActionID{SpellID: spellId})
	manaOptions := core.ManaCostOptions{FlatCost: manaCost, Multiplier: 100 - 25*priest.Talents.DevouringContagion}
	if priest.Env.IsForever() {
		manaOptions = core.ManaCostOptions{FlatCost: manaCost * (1 - .25*float64(priest.Talents.DevouringContagion))}
	}

	return core.SpellConfig{
		SpellCode:   SpellCode_PriestDevouringPlague,
		ActionID:    core.ActionID{SpellID: spellId},
		SpellSchool: core.SpellSchoolShadow,
		DefenseType: core.DefenseTypeMagic,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       SpellFlagPriest | core.SpellFlagAPL | core.SpellFlagDisease | core.SpellFlagPureDot,

		Rank:          rank,
		RequiredLevel: level,

		// Devouring Contagion, 25/50% in the beta client. Apply to the base
		// cost so Shadowform's separate 50% discount cannot make Plague free.
		// Multiplicative stacking follows the current model, not verified rounding.
		ManaCost: manaOptions,
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    cdTimer,
				Duration: time.Minute, // 3 min in Classic
			},
		},

		DamageMultiplier: 1,
		ThreatMultiplier: 1,

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: fmt.Sprintf("Devouring Plague (Rank %d)", rank),
			},

			NumberOfTicks:    ticks,
			TickLength:       time.Second * 3,
			BonusCoefficient: spellCoeff,

			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				dot.Snapshot(target, baseDotDamage, isRollover)
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				result := dot.CalcSnapshotDamage(sim, target, dot.OutcomeTick)
				damage := result.Damage
				dot.Spell.DealPeriodicDamage(sim, result)
				if sim.IsForever() {
					// The tooltip returns the damage actually dealt, including crits
					// and mitigation, rather than a separate healing-power roll.
					priest.GainHealth(sim, damage, healthMetrics)
				}
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcOutcome(sim, target, spell.OutcomeMagicHitNoHitCounter)
			if result.Landed() {
				priest.AddShadowWeavingStack(sim)
				spell.Dot(target).Apply(sim)
			}
			spell.DealOutcome(sim, result)
		},
	}
}
