package priest

import (
	"time"

	"github.com/wowsims/classic/sim/core"
)

const ShadowWordDeathRanks = 4

var ShadowWordDeathSpellId = [ShadowWordDeathRanks + 1]int32{0, 1309595, 1309633, 1309635, 1309636}
var ShadowWordDeathManaCost = [ShadowWordDeathRanks + 1]float64{0, 175, 205, 250, 340}
var ShadowWordDeathLevel = [ShadowWordDeathRanks + 1]int{0, 32, 40, 48, 56}

func (priest *Priest) registerShadowWordDeath() {
	if !priest.Env.IsForever() {
		return
	}

	priest.ShadowWordDeath = make([]*core.Spell, ShadowWordDeathRanks+1)
	cdTimer := priest.NewTimer()
	for rank := 1; rank <= ShadowWordDeathRanks; rank++ {
		if ShadowWordDeathLevel[rank] <= int(priest.Level) {
			priest.ShadowWordDeath[rank] = priest.RegisterSpell(priest.getShadowWordDeathConfig(rank, cdTimer))
		}
	}
}

func (priest *Priest) shadowWordDeathCritBonus(sim *core.Simulation) float64 {
	if sim.IsExecutePhase20() {
		return 15 * float64(priest.Talents.EarlyDemise) * core.SpellCritRatingPerCritChance
	}
	return 0
}

func (priest *Priest) getShadowWordDeathConfig(rank int, cdTimer *core.Timer) core.SpellConfig {
	low, high := shadowWordDeathForeverRanks[rank].damageRange(int(priest.Level))
	return core.SpellConfig{
		SpellCode:     SpellCode_PriestShadowWordDeath,
		ActionID:      core.ActionID{SpellID: ShadowWordDeathSpellId[rank]},
		SpellSchool:   core.SpellSchoolShadow,
		DefenseType:   core.DefenseTypeMagic,
		ProcMask:      core.ProcMaskSpellDamage,
		Flags:         SpellFlagPriest | core.SpellFlagAPL,
		RequiredLevel: ShadowWordDeathLevel[rank],
		Rank:          rank,
		ManaCost:      core.ManaCostOptions{FlatCost: ShadowWordDeathManaCost[rank]},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{GCD: core.GCDDefault},
			CD:          core.Cooldown{Timer: cdTimer, Duration: 15 * time.Second},
		},
		DamageMultiplier: 1,
		ThreatMultiplier: 1,
		BonusCoefficient: .429,
		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			bonusCrit := priest.shadowWordDeathCritBonus(sim)
			spell.BonusCritRating += bonusCrit
			result := spell.CalcDamage(sim, target, sim.Roll(low, high), spell.OutcomeMagicHitAndCrit)
			spell.BonusCritRating -= bonusCrit
			landed := result.Landed()
			// Duration-based and multi-target encounters do not track individual enemy
			// deaths. Treat their targets as surviving; only a single-target health
			// encounter can establish a killing blow and suppress the backlash.
			killingBlow := len(sim.Encounter.TargetUnits) == 1 && sim.Encounter.EndFightAtHealth > 0 &&
				sim.Encounter.DamageTaken+result.Damage >= sim.Encounter.EndFightAtHealth
			if landed {
				priest.AddShadowWeavingStack(sim)
			}
			spell.DealDamage(sim, result)
			if landed && !killingBlow {
				priest.RemoveHealth(sim, .10*priest.MaxHealth())
			}
		},
		ExpectedInitialDamage: func(sim *core.Simulation, target *core.Unit, spell *core.Spell, _ bool) *core.SpellResult {
			bonusCrit := priest.shadowWordDeathCritBonus(sim)
			spell.BonusCritRating += bonusCrit
			result := spell.CalcDamage(sim, target, (low+high)/2, spell.OutcomeExpectedMagicHitAndCrit)
			spell.BonusCritRating -= bonusCrit
			return result
		},
	}
}
