package priest

import (
	"fmt"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
)

const DarkSacrificeRanks = 5

var DarkSacrificeSpellId = [DarkSacrificeRanks + 1]int32{0, 1277324, 1277325, 1277326, 1277327, 1277328}
var darkSacrificeTickRanks = [DarkSacrificeRanks + 1]foreverDirectSpellRank{
	{},
	{80, 0, 1, 20, 28},
	{136, 0, 1, 30, 38},
	{196, 0, 1, 40, 48},
	{258, 0, 1, 50, 58},
	{320, 0, 1, 60, 68},
}

func (priest *Priest) registerDarkSacrifice() {
	if !priest.Env.IsForever() || priest.Race != proto.Race_RaceUndead {
		return
	}
	priest.DarkSacrifice = make([]*core.Spell, DarkSacrificeRanks+1)
	cdTimer := priest.NewTimer()
	for rank := 1; rank <= DarkSacrificeRanks; rank++ {
		data := darkSacrificeTickRanks[rank]
		if data.spellLevel > int(priest.Level) {
			continue
		}
		amount, _ := data.damageRange(int(priest.Level))
		actionID := core.ActionID{SpellID: DarkSacrificeSpellId[rank]}
		manaMetrics := priest.NewManaMetrics(actionID)
		priest.DarkSacrifice[rank] = priest.RegisterSpell(core.SpellConfig{
			SpellCode:     SpellCode_PriestDarkSacrifice,
			ActionID:      actionID,
			SpellSchool:   core.SpellSchoolShadow,
			Flags:         core.SpellFlagAPL | core.SpellFlagHelpful,
			Rank:          rank,
			RequiredLevel: data.spellLevel,
			Cast: core.CastConfig{
				DefaultCast: core.Cast{GCD: core.GCDDefault},
				CD:          core.Cooldown{Timer: cdTimer, Duration: 10 * time.Minute},
			},
			Hot: core.DotConfig{
				SelfOnly:      true,
				Aura:          core.Aura{Label: fmt.Sprintf("Dark Sacrifice (Rank %d)", rank)},
				NumberOfTicks: 5,
				TickLength:    3 * time.Second,
				OnTick: func(sim *core.Simulation, _ *core.Unit, _ *core.Dot) {
					// Fixed health-to-mana transfer: no spell power, crit or damage
					// multipliers. First payment is at 3 sec, the fifth at 15 sec.
					priest.RemoveHealth(sim, amount)
					priest.AddMana(sim, amount, manaMetrics)
				},
			},
			ApplyEffects: func(sim *core.Simulation, _ *core.Unit, spell *core.Spell) {
				spell.SelfHot().Apply(sim)
			},
		})
	}
	// Explicit APL usage only: health costs need encounter-specific planning.
}
