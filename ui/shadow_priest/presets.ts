import { CURRENT_LEVEL_CAP } from '../core/constants/mechanics.js';
import { ClassicPhase } from '../core/constants/other.js';
import * as PresetUtils from '../core/preset_utils.js';
import { Ruleset } from '../core/proto/api.js';
import {
	Conjured,
	Consumes,
	Debuffs,
	Flask,
	Food,
	IndividualBuffs,
	ManaRegenElixir,
	Potions,
	Profession,
	Race,
	RaidBuffs,
	ShadowPowerBuff,
	SpellPowerBuff,
	TristateEffect,
	WeaponImbue,
	ZanzaBuff,
} from '../core/proto/common.js';
import { PriestTalents, ShadowPriest_Options as Options } from '../core/proto/priest.js';
import { SavedTalents } from '../core/proto/ui.js';
import { protoToTalentString } from '../core/talents/factory.js';
import { priestTalentsConfig } from '../core/talents/priest.js';
import ForeverAPL from './apls/forever.apl.json';
import ForeverClippedAPL from './apls/forever-clipped.apl.json';
import ForeverDarkSacrificeAPL from './apls/forever-dark-sacrifice.apl.json';
import ForeverDarkSacrificeClippedAPL from './apls/forever-dark-sacrifice-clipped.apl.json';
import ForeverStarshardsAPL from './apls/forever-starshards.apl.json';
import P1APL from './apls/p1.apl.json';
import LaunchGearJSON from './gear_sets/launch.gear.json';
import P0BISGear from './gear_sets/p0.bis.gear.json';
import P1BISGear from './gear_sets/p1.bis.gear.json';
import ForeverBuild from './talents/forever-14-3-34.json';

// Preset options for this spec.
// Eventually we will import these values for the raid sim too, so its good to
// keep them in a separate file.

///////////////////////////////////////////////////////////////////////////
//                                 Gear Presets
///////////////////////////////////////////////////////////////////////////

const inheritedGearTooltip =
	'Inherited equipment example. Forever availability, item stats and effects have not all been verified; this is not a confirmed Forever best-in-slot set.';
export const GearLaunch = PresetUtils.makePresetGear('Launch (inherited)', LaunchGearJSON, { tooltip: inheritedGearTooltip });
export const GearP0BIS = PresetUtils.makePresetGear('Pre-BiS (inherited)', P0BISGear, { tooltip: inheritedGearTooltip });
export const GearP1BIS = PresetUtils.makePresetGear('P1 BiS (inherited)', P1BISGear, { tooltip: inheritedGearTooltip });

export const GearPresets = {
	[ClassicPhase.Phase1]: [GearLaunch, GearP0BIS, GearP1BIS],
};

export const DefaultGear = GearP0BIS;

///////////////////////////////////////////////////////////////////////////
//                                 APL Presets
///////////////////////////////////////////////////////////////////////////

export const APLForever = PresetUtils.makePresetAPLRotation('Forever · Full channels', ForeverAPL);
export const APLForeverClipped = PresetUtils.makePresetAPLRotation('Forever · Clip Flay after 2 ticks', ForeverClippedAPL);
export const APLForeverDarkSacrifice = PresetUtils.makePresetAPLRotation('Forever · Undead Dark Sacrifice', ForeverDarkSacrificeAPL, {
	customCondition: player => player.getRace() === Race.RaceUndead && CURRENT_LEVEL_CAP === 60 && player.sim.getRuleset() === Ruleset.RulesetForever,
});
export const APLForeverDarkSacrificeClipped = PresetUtils.makePresetAPLRotation('Forever · Undead Dark Sacrifice (clipped)', ForeverDarkSacrificeClippedAPL, {
	customCondition: player => player.getRace() === Race.RaceUndead && CURRENT_LEVEL_CAP === 60 && player.sim.getRuleset() === Ruleset.RulesetForever,
});
export const APLForeverStarshards = PresetUtils.makePresetAPLRotation('Forever · Night Elf Starshards', ForeverStarshardsAPL, {
	customCondition: player => player.getRace() === Race.RaceNightElf,
});
export const APLP1Shadow = PresetUtils.makePresetAPLRotation('Shadow (inherited)', P1APL);

APLForever.tooltip =
	'Level-60 starting priority: maintain Pain and Plague, prioritize Death during execute, then Mind Blast, Death and full Mind Flay. Auto includes Starshards for Night Elves. Compare priorities for your own gear and encounter.';
APLForeverClipped.tooltip =
	'Level-60 comparison: interrupt Flay after at least two ticks when a higher-priority damage spell is castable. Channel mana is paid in full and channel delay applies. This can trade mana efficiency for cooldown timing.';
APLForeverDarkSacrifice.tooltip =
	'Level-60 Undead full-channel rotation with optional Dark Sacrifice: at most 50% mana, at least 1600 missing mana, over 1600 health and at least 60% health, with over 15 seconds remaining. Transfers 1600 health to mana over 15 seconds; 10-minute cooldown. Edit the APL thresholds for your encounter.';
APLForeverDarkSacrificeClipped.tooltip =
	'Uses the same Dark Sacrifice mana and health conditions as the full-channel Undead preset, while retaining the two-tick Mind Flay interruption rules. Edit the APL thresholds for your encounter.';
APLForeverStarshards.tooltip =
	'Level-60 Night Elf option: cast six-tick Arcane Starshards after the direct spells. Its channel displaces other casts; compare with the full-channel preset without Starshards.';
APLP1Shadow.tooltip = 'Earlier fork rotation, retained for comparison. It omits Death and requires Inner Focus for its Plague sequence.';

export const APLPresets = {
	[ClassicPhase.Phase1]: [APLForever, APLForeverClipped, APLForeverDarkSacrifice, APLForeverDarkSacrificeClipped, APLForeverStarshards, APLP1Shadow],
};

export const DefaultAPL = APLPresets[ClassicPhase.Phase1][0];

///////////////////////////////////////////////////////////////////////////
//                                 Talent Presets
///////////////////////////////////////////////////////////////////////////

// Map the supplied allocation by Forever field name before encoding it in this
// tree's order. Classic talent-calculator strings use a different tree shape.
export const TalentsForever = PresetUtils.makePresetTalents(
	'Forever 14/3/34',
	SavedTalents.create({
		talentsString: protoToTalentString(PriestTalents.fromJson(ForeverBuild), priestTalentsConfig),
	}),
);

export const TalentsP1Shadow = PresetUtils.makePresetTalents('Shadow (inherited)', SavedTalents.create({ talentsString: '005300231303--505120501201300051' }));

export const TalentsShadow = PresetUtils.makePresetTalents(
	'Shadow 15/0/36',
	SavedTalents.create({ talentsString: '0253000311--550022501201302251' }),
);

// Keep the inherited community build's name so existing build links continue to work.

export const TalentPresets = {
	[ClassicPhase.Phase1]: [TalentsForever, TalentsP1Shadow, TalentsShadow],
};

export const DefaultTalents = TalentPresets[ClassicPhase.Phase1][0];

///////////////////////////////////////////////////////////////////////////
//                                 Options
///////////////////////////////////////////////////////////////////////////

export const DefaultOptions = Options.create({});

export const DefaultConsumes = Consumes.create({
	defaultConjured: Conjured.ConjuredDemonicRune,
	defaultPotion: Potions.MajorManaPotion,
	flask: Flask.FlaskOfSupremePower,
	food: Food.FoodRunnTumTuberSurprise,
	mainHandImbue: WeaponImbue.BrilliantWizardOil,
	manaRegenElixir: ManaRegenElixir.MagebloodPotion,

	shadowPowerBuff: ShadowPowerBuff.ElixirOfShadowPower,
	spellPowerBuff: SpellPowerBuff.GreaterArcaneElixir,
	zanzaBuff: ZanzaBuff.CerebralCortexCompound,
});

export const DefaultRaidBuffs = RaidBuffs.create({
	arcaneBrilliance: true,
	divineSpirit: true,
	fireResistanceAura: true,
	fireResistanceTotem: true,
	giftOfTheWild: TristateEffect.TristateEffectImproved,
	manaSpringTotem: TristateEffect.TristateEffectImproved,
	moonkinAura: true,
});

export const DefaultIndividualBuffs = IndividualBuffs.create({
	blessingOfWisdom: TristateEffect.TristateEffectImproved,
});

export const DefaultDebuffs = Debuffs.create({
	judgementOfWisdom: true,
});

export const OtherDefaults = {
	channelClipDelay: 100,
	distanceFromTarget: 30,
	// The inherited default equipment includes Green Lens, which requires Engineering.
	profession1: Profession.Engineering,
	profession2: Profession.Enchanting,
};
