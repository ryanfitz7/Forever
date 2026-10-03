package core

import (
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
)

// Simulation embeds Environment, so sim.IsForever() resolves here too.
func (env *Environment) IsForever() bool {
	return env.Ruleset == proto.Ruleset_RulesetForever
}

// Periodic damage rolls for crits under the Forever ruleset. Spells that
// should keep ticking for flat damage opt out with SpellFlagNoPeriodicCrit.
func (dot *Dot) canCrit(sim *Simulation) bool {
	return sim.IsForever() && !dot.Spell.Flags.Matches(SpellFlagNoPeriodicCrit)
}

// Ticks roll against the caster's crit chance at the time of the tick instead of the
// chance snapshotted when the dot went up. Dots that are applied by hand, like Deep
// Wounds, never snapshot one at all, so rolling live is also the only way for them to
// crit at the right rate.
func (dot *Dot) critCheck(sim *Simulation, target *Unit, attackTable *AttackTable) bool {
	if dot.Spell.SchoolIndex == stats.SchoolIndexPhysical {
		return dot.Spell.PhysicalCritCheck(sim, attackTable)
	}
	return sim.RandomFloat("Magical Crit Roll") < dot.Spell.PeriodicSpellCritChance(target)
}

// Bonus healing on Forever gear carries a damage component with it, so that healing
// gear is not dead weight outside a raid. Hide of the Wild reads 42 healing and 14
// damage, which is the only published pair, so a third is the rate used here. It feeds
// SpellDamage rather than SpellPower because the damage half does not heal.
const ForeverHealingToSpellDamage = 1.0 / 3.0

func (character *Character) addHealingSpellDamage(equipStats stats.Stats) stats.Stats {
	equipStats[stats.SpellDamage] += equipStats[stats.HealingPower] * ForeverHealingToSpellDamage
	return equipStats
}

// Forever pays out hit and critical strike from gear against every kind of attack
// rather than splitting them into a melee and a spell pool. Attribute conversions are
// untouched: only the hit and crit an item spells out become universal.
func (character *Character) unifyEquipHitAndCrit(equipStats stats.Stats) stats.Stats {
	hit := equipStats[stats.MeleeHit] + equipStats[stats.SpellHit]
	crit := equipStats[stats.MeleeCrit] + equipStats[stats.SpellCrit]

	equipStats[stats.MeleeHit] = hit
	equipStats[stats.SpellHit] = hit
	equipStats[stats.MeleeCrit] = crit
	equipStats[stats.SpellCrit] = crit

	return equipStats
}

// Rage from a landed auto attack is flat on Forever: set by the weapon's speed and nothing
// else. Classic pays 7.5 x damage / conversion, so a crit is worth double a normal hit and a
// geared warrior is worth several times a levelling one; on Forever a swing is a swing.
//
// Measured from public beta combat logs by BrawnyBravo (issue #252): 63 clean pairs across
// nine warriors at levels 10-15, taking only consecutive auto-attack snapshots 0-4s apart
// with no ability used and no damage taken between them. Damage in the sample ran from 15 to
// 64 a hit, crits included, with no effect on the rage gained.
//
//	~2.1s one-hand   7.2-7.3      2.1 x 3.5 = 7.35
//	~2.5s one-hand   8.6-8.7      2.5 x 3.5 = 8.75
//	~3.2s two-hand  14.4          3.2 x 4.5 = 14.4
//	~3.3s two-hand  14.9          3.3 x 4.5 = 14.85
//	~3.5s two-hand  15.7          3.5 x 4.5 = 15.75
//
// The two-hand column lands on 4.5 x speed to within a tenth. The one-hand column sits about
// 1.5% under 3.5 x speed at both speeds, which is either a slightly lower multiplier or
// weapon speeds that were not exactly the round numbers they were bucketed as. 3.5 is used
// because the report's stated rule is the thing being modelled, and a second decimal place
// invented from two buckets would be worse than the honest round number.
const (
	ForeverRagePerSecondOneHand = 3.5
	ForeverRagePerSecondTwoHand = 4.5
)

// What a landed swing is worth. Base weapon speed, not the hasted interval: otherwise haste
// would buy swings and lose exactly as much rage per swing, which would make it rage-neutral.
// The sample was taken at a level where nobody had haste, so it cannot tell the two apart -
// this is the assumption, and it is the one worth re-measuring first.
func ForeverWhiteHitRage(weapon *Weapon) float64 {
	if weapon == nil || weapon.SwingSpeed == 0 {
		return 0
	}
	if weapon.TwoHand {
		return weapon.SwingSpeed * ForeverRagePerSecondTwoHand
	}
	return weapon.SwingSpeed * ForeverRagePerSecondOneHand
}
