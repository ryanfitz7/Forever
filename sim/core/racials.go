package core

import (
	"fmt"
	"slices"
	"time"

	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
)

func applyRaceEffects(agent Agent) {
	character := agent.GetCharacter()

	// Forever drops every +10 resistance racial and turns the weapon skill racials into
	// critical strike while that weapon is held. Races also pick up a new passive each.
	forever := character.Env.IsForever()

	switch character.Race {
	case proto.Race_RaceDwarf:
		if forever {
			character.AddWeaponSpecializationCrit(1, proto.WeaponType_WeaponTypeMace)
			character.beastSlayingAura(1.05)
		} else {
			character.AddStat(stats.FrostResistance, 10)
			character.GunSpecializationAura()
		}

		actionID := ActionID{SpellID: 20594}

		statDep := character.NewDynamicMultiplyStat(stats.Armor, 1.1)
		stoneFormAura := character.NewTemporaryStatsAuraWrapped("Stoneform", actionID, stats.Stats{}, time.Second*8, func(aura *Aura) {
			aura.ApplyOnGain(func(aura *Aura, sim *Simulation) {
				aura.Unit.EnableDynamicStatDep(sim, statDep)
			})
			aura.ApplyOnExpire(func(aura *Aura, sim *Simulation) {
				aura.Unit.DisableDynamicStatDep(sim, statDep)
			})
		})

		spell := character.RegisterSpell(SpellConfig{
			ActionID: actionID,
			Flags:    SpellFlagNoOnCastComplete,
			Cast: CastConfig{
				CD: Cooldown{
					Timer:    character.NewTimer(),
					Duration: time.Minute * 3,
				},
			},
			ApplyEffects: func(sim *Simulation, _ *Unit, _ *Spell) {
				stoneFormAura.Activate(sim)
			},
		})

		character.AddMajorCooldown(MajorCooldown{
			Spell: spell,
			Type:  CooldownTypeSurvival,
			ShouldActivate: func(s *Simulation, c *Character) bool {
				// Only castable with manual APL Action
				return false
			},
		})
	case proto.Race_RaceGnome:
		if forever {
			// Expansive Mind raises the resource pool itself now rather than Intellect.
			// Only the mana half is modelled; rage and energy have no max stat here.
			character.MultiplyStat(stats.Mana, 1.05)
			character.registerEureka()
		} else {
			character.AddStat(stats.ArcaneResistance, 10)
			character.MultiplyStat(stats.Intellect, 1.05)
		}
	case proto.Race_RaceHuman:
		character.MultiplyStat(stats.Spirit, 1.05)
		if forever {
			// Mace Specialization moved to the Dwarves.
			character.AddWeaponSpecializationCrit(2, proto.WeaponType_WeaponTypeSword)
		} else {
			character.SwordSpecializationAura()
			character.MaceSpecializationAura()
		}
	case proto.Race_RaceNightElf:
		if forever {
			character.registerElunesLight()
		} else {
			character.AddStat(stats.NatureResistance, 10)
		}
		character.AddStat(stats.Dodge, 1)
		// TODO: Shadowmeld?
	case proto.Race_RaceOrc:
		if forever {
			character.AddWeaponSpecializationCrit(1, proto.WeaponType_WeaponTypeAxe)
		} else {
			character.AxeSpecializationAura()
		}

		// Command is gone under Forever; Shatter Curse takes its place in the Orc's four,
		// and dispelling a curse is nothing the sim measures.
		if !forever && (character.Class == proto.Class_ClassHunter || character.Class == proto.Class_ClassWarlock) {
			// Command Damage dealt by Hunter and Warlock pets increased by 5%
			for _, pet := range character.Pets {
				if !pet.IsGuardian() {
					pet.PseudoStats.DamageDealtMultiplier *= 1.05
				}
			}
		}

		// Blood Fury
		actionID := ActionID{SpellID: 20572}
		var bloodFuryAP, bloodFurySP float64
		bloodFuryAura := character.RegisterAura(Aura{
			Label:    "Blood Fury",
			ActionID: actionID,
			Duration: time.Second * 15,
			// Tooltip is misleading; ap bonus is base AP plus AP from current strength, does not include +attackpower on items/buffs
			OnGain: func(aura *Aura, sim *Simulation) {
				if forever {
					// Forever reads plainly off everything the orc has, and pays spell power
					// on the same terms, so a caster orc gets something out of it at last.
					bloodFuryAP = character.GetStat(stats.AttackPower) * 0.1
					bloodFurySP = character.GetStat(stats.SpellPower) * 0.1
				} else {
					bloodFuryAP = (character.GetBaseStats()[stats.AttackPower] + (character.GetStat(stats.Strength) * APPerStrength[character.Class]) + (character.GetStat(stats.Agility) * APPerAgility[character.Class])) * 0.25
				}
				character.AddStatDynamic(sim, stats.AttackPower, bloodFuryAP)
				character.AddStatDynamic(sim, stats.SpellPower, bloodFurySP)
			},

			OnExpire: func(aura *Aura, sim *Simulation) {
				character.AddStatDynamic(sim, stats.AttackPower, -bloodFuryAP)
				character.AddStatDynamic(sim, stats.SpellPower, -bloodFurySP)
			},
		})

		spell := character.RegisterSpell(SpellConfig{
			ActionID: actionID,
			Flags:    SpellFlagNoOnCastComplete,
			Cast: CastConfig{
				DefaultCast: Cast{
					GCD: GCDDefault,
				},
				CD: Cooldown{
					Timer:    character.NewTimer(),
					Duration: time.Minute * 2,
				},
			},
			ApplyEffects: func(sim *Simulation, _ *Unit, _ *Spell) {
				bloodFuryAura.Activate(sim)
			},
		})

		character.AddMajorCooldown(MajorCooldown{
			Spell: spell,
			Type:  CooldownTypeDPS,
		})
	case proto.Race_RaceTauren:
		character.MultiplyStat(stats.Health, 1.05)
		if forever {
			// Endurance carries a point of hit alongside the health.
			character.AddStat(stats.MeleeHit, 1*MeleeHitRatingPerHitChance)
			character.AddStat(stats.SpellHit, 1*SpellHitRatingPerHitChance)
		} else {
			character.AddStat(stats.NatureResistance, 10)
		}
	case proto.Race_RaceTroll:
		// Forever's troll has four racials and neither ranged weapon specialization is
		// among them; Rapid Regeneration and Regeneration took their place.
		if !forever {
			character.BowSpecializationAura()
			character.ThrownSpecializationAura()
		}

		character.beastSlayingAura(1.05)

		// Berserking
		berserkingTimer := character.NewTimer()
		if forever {
			// Berserking is a flat 10% now instead of scaling up as health drops.
			makeBerserkingCooldown(character, .1, berserkingTimer)
		} else {
			// Baseline cooldown
			makeBerserkingCooldown(character, 0, berserkingTimer)
			// Hard-coded percentage cooldown options
			makeBerserkingCooldown(character, .1, berserkingTimer)
			makeBerserkingCooldown(character, .15, berserkingTimer)
			makeBerserkingCooldown(character, .2, berserkingTimer)
			makeBerserkingCooldown(character, .25, berserkingTimer)
			makeBerserkingCooldown(character, .3, berserkingTimer)
		}
	case proto.Race_RaceUndead:
		if !forever {
			character.AddStat(stats.ShadowResistance, 10)
		} else {
			character.registerTouchOfTheGrave()
		}
	case proto.Race_RaceSkyborneHighOrder, proto.Race_RaceSkyborneWindshaper:
		// The Skyborne are a Forever race, so under Classic rules they have no racials to
		// grant. A saved setting can still name one - the race picker offers whatever the
		// spec allows - and without this guard that saved race would carry Forever's haste
		// and elemental damage into a Classic sim.
		if !forever {
			break
		}

		// Wind Blessed
		character.PseudoStats.MeleeSpeedMultiplier *= 1.01
		character.PseudoStats.RangedSpeedMultiplier *= 1.01
		character.PseudoStats.CastSpeedMultiplier *= 1.01

		// Elemental Insight
		character.mobTypeDamageAura(proto.MobType_MobTypeElemental, 1.05)

		if character.Race == proto.Race_RaceSkyborneWindshaper {
			character.registerWindshaper()
		}
		// The High Order racial is a health and mana regeneration cooldown, which does
		// nothing the sim measures, so it is left out.
	}
}

// Eureka! has class-specific client spells. Priest's 1259823 grants 15% lower
// mana cost, 10% more damage/healing and three charges (beta 1.60.1.69913).
// Its damage bonus is dynamic for Priest DoTs, confirmed by beta observation.
// Channel charges are consumed at application; Death remains eligible under
// the generic damaging-cast assumption despite its class-mask mismatch. Both
// charge details still need a live test. Other classes retain the fork's model.
func (character *Character) registerEureka() {
	actionID := ActionID{SpellID: 460550}
	priestEureka := character.Class == proto.Class_ClassPriest
	if priestEureka {
		actionID = ActionID{SpellID: 1259823}
	}

	var affected []*Spell
	aura := character.RegisterAura(Aura{
		Label:     "Eureka!",
		ActionID:  actionID,
		Duration:  time.Minute,
		MaxStacks: 3,
		OnInit: func(aura *Aura, sim *Simulation) {
			// Anything that costs mana and deals damage is a candidate; a charge is spent
			// by whichever of them is cast first.
			for _, spell := range character.Spellbook {
				if spell.Cost != nil && spell.ProcMask.Matches(ProcMaskSpellDamage) &&
					(!priestEureka || spell.Cost.CostType() == CostTypeMana) {
					affected = append(affected, spell)
				}
			}
		},
		OnGain: func(aura *Aura, sim *Simulation) {
			if !priestEureka {
				character.PseudoStats.DamageDealtMultiplier *= 1.1
			}
			for _, spell := range affected {
				if priestEureka {
					spell.Cost.FinalMultiplier *= 0.85
					spell.DynamicDamageMultiplier *= 1.1
				} else {
					spell.Cost.Multiplier -= 50
				}
			}
		},
		OnExpire: func(aura *Aura, sim *Simulation) {
			if !priestEureka {
				character.PseudoStats.DamageDealtMultiplier /= 1.1
			}
			for _, spell := range affected {
				if priestEureka {
					spell.Cost.FinalMultiplier /= 0.85
					spell.DynamicDamageMultiplier /= 1.1
				} else {
					spell.Cost.Multiplier += 50
				}
			}
		},
		OnStacksChange: func(aura *Aura, sim *Simulation, _ int32, newStacks int32) {
			if newStacks == 0 {
				aura.Deactivate(sim)
			}
		},
		OnCastComplete: func(aura *Aura, sim *Simulation, spell *Spell) {
			if !priestEureka {
				// Preserve the inherited model for classes outside this Priest review.
				if aura.RemainingDuration(sim) != aura.Duration && aura.GetStacks() > 0 && spell.ProcMask.Matches(ProcMaskSpellDamage) {
					aura.RemoveStack(sim)
				}
				return
			}
			// The activation has NoOnCastComplete and is not in affected. A damaging
			// instant cast at the same timestamp must still spend its charge.
			if aura.GetStacks() > 0 && slices.Contains(affected, spell) {
				aura.RemoveStack(sim)
			}
		},
	})

	spell := character.RegisterSpell(SpellConfig{
		ActionID: actionID,
		Flags:    SpellFlagNoOnCastComplete,
		Cast: CastConfig{
			CD: Cooldown{
				Timer:    character.NewTimer(),
				Duration: time.Minute * 2,
			},
		},
		ApplyEffects: func(sim *Simulation, _ *Unit, _ *Spell) {
			aura.Activate(sim)
			aura.SetStacks(sim, aura.MaxStacks)
		},
	})

	character.AddMajorCooldown(MajorCooldown{
		Spell: spell,
		Type:  CooldownTypeDPS,
	})
}

// Priest's Touch of the Grave uses client aura 1260201: a 10% chance and 1 second
// proc recovery. The 5% aura 1260189 belongs to physical classes. Other classes
// retain the inherited model pending their own review. Damage spell 1260198 is
// a health leech whose client description specifies 5% of caster maximum health.
// Its separate hit roll and Shadow mitigation remain assumptions. Beta observation excludes periodic tick triggers;
// channel application eligibility remains assumed.
func (character *Character) registerTouchOfTheGrave() {
	actionID := ActionID{SpellID: 460540}
	auraID := actionID
	priestRacial := character.Class == proto.Class_ClassPriest
	procChance := 0.05
	var procCD Cooldown
	if priestRacial {
		actionID = ActionID{SpellID: 1260198}
		auraID = ActionID{SpellID: 1260201}
		procChance = 0.10
		procCD = Cooldown{Timer: character.NewTimer(), Duration: time.Second}
	}
	healthMetrics := character.NewHealthMetrics(actionID)

	drain := character.RegisterSpell(SpellConfig{
		ActionID:    actionID,
		SpellSchool: SpellSchoolShadow,
		DefenseType: DefenseTypeMagic,
		ProcMask:    ProcMaskEmpty,
		// The drain is a proc off another hit, so it must not feed the procs that spawned
		// it or two undead attacks would chain into each other.
		Flags: SpellFlagNoOnCastComplete | SpellFlagPassiveSpell,

		DamageMultiplier: 1,
		ThreatMultiplier: 1,

		ApplyEffects: func(sim *Simulation, target *Unit, spell *Spell) {
			maxHealth := character.MaxHealth()
			baseDamage := maxHealth * 0.05
			if !priestRacial {
				baseDamage = sim.Roll(maxHealth*0.025, maxHealth*0.05)
			}
			result := spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMagicHit)

			// Only the specs that track a health bar can be healed; for everyone else the
			// drain is still damage, it just has nothing to return the health to.
			if result.Landed() && character.HasHealthBar() {
				character.GainHealth(sim, result.Damage, healthMetrics)
			}
		},
	})

	MakePermanent(character.RegisterAura(Aura{
		Label:    "Touch of the Grave",
		ActionID: auraID,
		OnSpellHitDealt: func(_ *Aura, sim *Simulation, spell *Spell, result *SpellResult) {
			if !result.Landed() || spell == drain {
				return
			}
			if priestRacial && (!spell.ProcMask.Matches(ProcMaskDirect) || !procCD.IsReady(sim)) {
				return
			}
			if sim.RandomFloat("Touch of the Grave") < procChance {
				if priestRacial {
					procCD.Use(sim)
				}
				drain.Cast(sim, result.Target)
			}
		},
	}))
}

// Elune's Light, the night elf's Forever racial cooldown: 10% critical strike for 15
// seconds on a three minute cooldown.
func (character *Character) registerElunesLight() {
	actionID := ActionID{SpellID: 1259799}

	aura := character.RegisterAura(Aura{
		Label:    "Elune's Light",
		ActionID: actionID,
		Duration: time.Second * 15,
		OnGain: func(aura *Aura, sim *Simulation) {
			character.AddStatsDynamic(sim, stats.Stats{
				stats.MeleeCrit: 10 * CritRatingPerCritChance,
				stats.SpellCrit: 10 * SpellCritRatingPerCritChance,
			})
		},
		OnExpire: func(aura *Aura, sim *Simulation) {
			character.AddStatsDynamic(sim, stats.Stats{
				stats.MeleeCrit: -10 * CritRatingPerCritChance,
				stats.SpellCrit: -10 * SpellCritRatingPerCritChance,
			})
		},
	})

	spell := character.RegisterSpell(SpellConfig{
		ActionID: actionID,
		Flags:    SpellFlagNoOnCastComplete,
		Cast: CastConfig{
			CD: Cooldown{
				Timer:    character.NewTimer(),
				Duration: time.Minute * 3,
			},
		},
		ApplyEffects: func(sim *Simulation, _ *Unit, _ *Spell) {
			aura.Activate(sim)
		},
	})

	character.AddMajorCooldown(MajorCooldown{
		Spell: spell,
		Type:  CooldownTypeDPS,
	})
}

// Windshaper, the Horde half of the Skyborne. The Alliance half gets a regen cooldown
// in its place.
func (character *Character) registerWindshaper() {
	actionID := ActionID{SpellID: 460530}

	var attackPower, spellPower float64
	aura := character.RegisterAura(Aura{
		Label:    "Windshaper",
		ActionID: actionID,
		Duration: time.Second * 15,
		OnGain: func(aura *Aura, sim *Simulation) {
			attackPower = character.GetStat(stats.AttackPower) * 0.1
			spellPower = character.GetStat(stats.SpellPower) * 0.1
			character.AddStatDynamic(sim, stats.AttackPower, attackPower)
			character.AddStatDynamic(sim, stats.SpellPower, spellPower)
		},
		OnExpire: func(aura *Aura, sim *Simulation) {
			character.AddStatDynamic(sim, stats.AttackPower, -attackPower)
			character.AddStatDynamic(sim, stats.SpellPower, -spellPower)
		},
	})

	spell := character.RegisterSpell(SpellConfig{
		ActionID: actionID,
		Flags:    SpellFlagNoOnCastComplete,
		Cast: CastConfig{
			CD: Cooldown{
				Timer:    character.NewTimer(),
				Duration: time.Minute * 3,
			},
		},
		ApplyEffects: func(sim *Simulation, _ *Unit, _ *Spell) {
			aura.Activate(sim)
		},
	})

	character.AddMajorCooldown(MajorCooldown{
		Spell: spell,
		Type:  CooldownTypeDPS,
	})
}

// Troll Beast Slaying, and Dwarf Big Game Hunter under Forever.
func (character *Character) beastSlayingAura(multiplier float64) {
	character.mobTypeDamageAura(proto.MobType_MobTypeBeast, multiplier)
}

func (character *Character) mobTypeDamageAura(mobType proto.MobType, multiplier float64) {
	character.Env.RegisterPostFinalizeEffect(func() {
		for _, t := range character.Env.Encounter.Targets {
			if t.MobType == mobType {
				for _, at := range character.AttackTables[t.UnitIndex] {
					at.DamageDealtMultiplier *= multiplier
					at.CritMultiplier *= multiplier
				}
			}
		}
	})
}

// If customPercentage is 0, use the baseline Berserking calculations from health missing
// otherwise create a cooldown hard-coded to the custom percentage.
func makeBerserkingCooldown(character *Character, customPercentage float64, timer *Timer) {
	actionID := ActionID{SpellID: 26297, Tag: int32(customPercentage * 20)}
	priestForever := character.Env.IsForever() && character.Class == proto.Class_ClassPriest
	if priestForever {
		actionID = ActionID{SpellID: 20554}
	}

	label := "Berserking"
	if customPercentage != 0 {
		label = fmt.Sprintf("%s (%d)", label, int(customPercentage*100))
	}

	calcBerserkingPct := func() float64 {
		if customPercentage != 0 {
			return customPercentage
		}
		// from 10% at full health to 30% at 40% or less health
		switch hp := character.CurrentHealthPercent(); {
		case hp >= 1:
			return 0.1
		case hp <= 0.4:
			return 0.3
		default:
			return 0.1 + (1-hp)/3
		}
	}

	var berserkingAura *Aura
	var berserkingHaste float64
	if character.HasManaBar() {
		// Mana-using classes gain a flat % reduction in attack and cast speed
		berserkingAura = character.RegisterAura(Aura{
			Label:    label,
			ActionID: actionID,
			Duration: time.Second * 10,
			OnGain: func(aura *Aura, sim *Simulation) {
				berserkingHaste = 1 / (1 - calcBerserkingPct())
				if priestForever {
					// Client 20554 has +10 casting/melee/ranged haste, rather than the
					// inherited 10% time reduction. A 1.10 speed multiplier models that
					// wording; exact server stacking still needs a timing measurement.
					berserkingHaste = 1.10
				}

				character.MultiplyCastSpeed(berserkingHaste)
				character.MultiplyAttackSpeed(sim, berserkingHaste)

				if sim.Log != nil {
					character.Log(sim, "Berserking increased attack and casting speed by %.2f%% (%.2f%% hp)", berserkingHaste*100-100, character.CurrentHealthPercent()*100)
				}
			},
			OnExpire: func(aura *Aura, sim *Simulation) {
				character.MultiplyCastSpeed(1 / berserkingHaste)
				character.MultiplyAttackSpeed(sim, 1/berserkingHaste)
			},
		})
	} else {
		// Non-mana bar classes gain a flat % reduction in attack and cast speed
		berserkingAura = character.RegisterAura(Aura{
			Label:    label,
			ActionID: actionID,
			Duration: time.Second * 10,
			OnGain: func(aura *Aura, sim *Simulation) {
				berserkingHaste = 1 + calcBerserkingPct()

				character.MultiplyAttackSpeed(sim, berserkingHaste)

				if sim.Log != nil {
					character.Log(sim, "Berserking increased attack speed by %.2f%% (%.2f%% hp)", berserkingHaste*100-100, character.CurrentHealthPercent()*100)
				}
			},
			OnExpire: func(aura *Aura, sim *Simulation) {
				character.MultiplyAttackSpeed(sim, 1/berserkingHaste)
			},
		})
	}

	config := SpellConfig{
		ActionID: actionID,

		Cast: CastConfig{
			CD: Cooldown{
				Timer:    timer,
				Duration: time.Minute * 3,
			},
		},

		ApplyEffects: func(sim *Simulation, _ *Unit, _ *Spell) {
			berserkingAura.Activate(sim)
		},
	}

	switch {
	case priestForever:
		// The exact-build SpellPower query for 20554 is empty: no resource cost.
	case character.HasManaBar():
		config.ManaCost = ManaCostOptions{BaseCost: 0.07}
	case character.HasRageBar():
		config.RageCost = RageCostOptions{Cost: 5}
	case character.HasEnergyBar():
		config.EnergyCost = EnergyCostOptions{Cost: 10}
	}

	berserkingSpell := character.RegisterSpell(config)

	character.AddMajorCooldown(MajorCooldown{
		Spell: berserkingSpell,
		Type:  CooldownTypeDPS,
	})
}

func (character *Character) GetFaction() proto.Faction {
	if slices.Contains([]proto.Race{proto.Race_RaceHuman, proto.Race_RaceDwarf, proto.Race_RaceGnome, proto.Race_RaceNightElf, proto.Race_RaceSkyborneHighOrder}, character.Race) {
		return proto.Faction_Alliance
	} else if slices.Contains([]proto.Race{proto.Race_RaceOrc, proto.Race_RaceTroll, proto.Race_RaceTauren, proto.Race_RaceUndead, proto.Race_RaceSkyborneWindshaper}, character.Race) {
		return proto.Faction_Horde
	} else {
		return proto.Faction_Unknown
	}
}
