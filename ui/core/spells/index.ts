// The spell manifest, loaded for the browser.
//
// These files were written so Wowhead's Classic database would stop describing abilities
// Forever changed, and sim/spell_sources_test.go keeps them honest against what the sim
// registers. Nothing read them at runtime, so an ability Wowhead has never heard of - Lava
// Burst's Forever ranks, Spearing Strike, the set procs in the 450000 range - arrived in the
// damage table with no name at all, as a blank row.

import commonJson from './common.json';
import coreJson from './core.json';
import druidJson from './druid.json';
import encountersJson from './encounters.json';
import hunterJson from './hunter.json';
import mageJson from './mage.json';
import paladinJson from './paladin.json';
import priestJson from './priest.json';
import rogueJson from './rogue.json';
import shamanJson from './shaman.json';
import warlockJson from './warlock.json';
import warriorJson from './warrior.json';

/**
 * A number checked against a running game rather than against a table.
 *
 * Sits alongside `source` rather than replacing it: where a number came from and whether
 * anyone has watched it happen are two different facts, and an ability read from the client
 * AND confirmed in game is worth more than either on its own.
 */
export type SpellMeasured = {
	date: string;
	how: string;
	/** What the client's tables say, for comparison. */
	client: string;
	/** The largest single hit the game's own meter recorded. */
	biggest: number;
};

export type SpellSource = {
	ability: string;
	file: string;
	source: 'classic' | 'forever' | 'assumed' | 'unreviewed';
	foreverId?: number;
	/** Client icon name and rank for abilities absent from the Classic database. */
	icon?: string;
	rank?: number;
	tooltip?: string;
	note?: string;
	assumptions?: Array<string>;
	measured?: SpellMeasured;
};

const files: Array<Record<string, unknown>> = [
	commonJson,
	coreJson,
	druidJson,
	encountersJson,
	hunterJson,
	mageJson,
	paladinJson,
	priestJson,
	rogueJson,
	shamanJson,
	warlockJson,
	warriorJson,
];

const bySpellId = new Map<number, SpellSource>();
for (const file of files) {
	for (const [id, entry] of Object.entries(file)) {
		bySpellId.set(parseInt(id), entry as SpellSource);
	}
}

export function spellSource(spellId: number): SpellSource | undefined {
	return bySpellId.get(spellId);
}

/** Every entry, for the evidence page. */
export function allSpellSources(): Array<[number, SpellSource]> {
	return [...bySpellId.entries()];
}
