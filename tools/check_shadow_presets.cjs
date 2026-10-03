const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const Module = require('node:module');
const ts = require('typescript');

const root = path.resolve(__dirname, '..');
const read = name => JSON.parse(fs.readFileSync(path.join(root, name), 'utf8'));
const build = read('ui/shadow_priest/talents/forever-14-3-34.json');
const trees = read('ui/core/talents/trees/priest.json');
const rankData = read('ui/shadow_priest/spell-ranks.json').spells;

// Exercise the actual Auto builder without importing the browser-only UI.
const filename = path.join(root, 'ui/shadow_priest/rotation.ts');
const compiled = ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
	compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020, esModuleInterop: true },
}).outputText;
const rotationModule = new Module(filename, module);
rotationModule.filename = filename;
rotationModule.paths = Module._nodeModulePaths(path.dirname(filename));
rotationModule._compile(compiled, filename);
const { foreverAutoRotation } = rotationModule.exports;

const knownFields = new Set(trees.flatMap(tree => tree.talents.map(talent => talent.fieldName)));
for (const field of Object.keys(build)) assert.ok(knownFields.has(field), `Unknown talent ${field}`);
const totals = trees.map(tree => {
	for (const talent of tree.talents) {
		const points = Number(build[talent.fieldName] || 0);
		assert.ok(Number.isInteger(points) && points <= talent.maxPoints, `Invalid rank for ${talent.fieldName}`);
		if (!points) continue;
		const earlier = tree.talents
			.filter(other => other.location.rowIdx < talent.location.rowIdx)
			.reduce((sum, other) => sum + Number(build[other.fieldName] || 0), 0);
		assert.ok(earlier >= talent.location.rowIdx * 5, `Tier requirement for ${talent.fieldName}`);
		if (talent.prereqLocation) {
			const prerequisite = tree.talents.find(
				other => other.location.rowIdx === talent.prereqLocation.rowIdx && other.location.colIdx === talent.prereqLocation.colIdx,
			);
			assert.equal(Number(build[prerequisite.fieldName]), prerequisite.maxPoints, `Prerequisite for ${talent.fieldName}`);
		}
	}
	return tree.talents.reduce((sum, talent) => sum + Number(build[talent.fieldName] || 0), 0);
});
assert.deepEqual(totals, [14, 3, 34]);

function ids(value, result = []) {
	if (!value || typeof value !== 'object') return result;
	if (typeof value.spellId === 'number') result.push(value.spellId);
	for (const child of Object.values(value)) ids(child, result);
	return result;
}

const allRanks = Object.values(rankData).flat();
for (const level of [25, 38, 40, 50, 60]) {
	for (const nightElf of [false, true]) {
		const apl = foreverAutoRotation(level, nightElf, build);
		for (const id of ids(apl)) {
			const rank = allRanks.find(row => row.spellId === id);
			if (rank) assert.ok(rank.level <= level, `${id} unavailable at ${level}`);
		}
		for (const [maxId, ranks] of Object.entries(rankData)) {
			if (Number(maxId) === 19305 && !nightElf) {
				assert.ok(!ids(apl).some(id => ranks.some(rank => rank.spellId === id)), 'Starshards on non-Night Elf');
				continue;
			}
			const highest = ranks.filter(rank => rank.level <= level).at(-1);
			if (highest) assert.ok(ids(apl).includes(highest.spellId), `Missing highest rank ${highest.spellId} at ${level}`);
		}
	}
}

const untalented = foreverAutoRotation(60, false, { shadowform: false, innerFocus: false, mindFlay: false });
for (const id of [15473, 14751, 18807]) assert.ok(!ids(untalented).includes(id), `Unlearned talent spell ${id}`);
assert.ok(ids(untalented).includes(19280), 'Plague must remain usable without Inner Focus');

const base = foreverAutoRotation(60, false, build);
const actions = base.priorityList.map(row => row.action);
const deathExecute = actions.findIndex(action => action.castSpell?.spellId.spellId === 1309636 && action.condition?.isExecutePhase);
const blast = actions.findIndex(action => action.castSpell?.spellId.spellId === 10947);
const deathOutside = actions.findIndex(action => action.castSpell?.spellId.spellId === 1309636 && !action.condition);
assert.equal(actions[deathExecute].condition.isExecutePhase.threshold, 'E20');
assert.ok(deathExecute < blast && blast < deathOutside, 'Execute/non-execute Death priority');
assert.equal(actions.at(-1).castSpell.spellId.spellId, 18807, 'Default full Flay');
const focus = actions.find(action => action.castSpell?.spellId.spellId === 14751);
assert.ok(
	focus.condition.and.vals.some(value => value.not?.val.auraIsActive?.auraId.spellId === 14751),
	'Inner Focus must not repeatedly activate while its charge is active',
);
assert.ok(
	focus.condition.and.vals.some(value => value.spellIsReady?.spellId.spellId === 19280),
	'Free Plague must remain possible when mana is low',
);

const nightElf = foreverAutoRotation(60, true, build);
const star = nightElf.priorityList.find(row => row.action.castSpell?.spellId.spellId === 19305);
assert.ok(star.action.condition.and.vals.some(value => value.spellCanCast?.spellId.spellId === 19305));
const clipped = read('ui/shadow_priest/apls/forever-clipped.apl.json').priorityList.at(-1).action.channelSpell;
assert.equal(clipped.allowRecast, false);
assert.equal(clipped.interruptIf.and.vals[0].cmp.rhs.const.val, '2');
assert.ok(clipped.interruptIf.and.vals[1].or.vals.every(value => value.spellCanCast || value.and));

// A future second class or Auto run must not inherit rank rewrites from the first.
assert.deepEqual(foreverAutoRotation(60, false, build), base);
console.log(`Shadow presets passed: legal ${totals.join('/')} build; 10 level/race Auto combinations; optional talents; execute priority; channel rules.`);
