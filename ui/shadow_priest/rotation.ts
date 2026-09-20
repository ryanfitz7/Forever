import ForeverAPL from './apls/forever.apl.json';
import StarshardsAPL from './apls/forever-starshards.apl.json';
import SpellRanks from './spell-ranks.json';

interface AutoTalents {
	shadowform: boolean;
	innerFocus: boolean;
	mindFlay: boolean;
}

// Auto resolves its template against the character. A missing spell inside an
// APL condition still produces an engine warning, so omit unavailable actions
// before handing the rotation to the simulator.
export function foreverAutoRotation(level: number, useStarshards: boolean, talents: AutoTalents) {
	const replacements = new Map<number, { spellId: number; rank?: number } | null>();
	for (const [highestId, ranks] of Object.entries(SpellRanks.spells)) {
		const trained = ranks.filter(rank => rank.level <= level).at(-1);
		replacements.set(Number(highestId), trained ? { spellId: trained.spellId, rank: trained.rank } : null);
	}
	if (!talents.mindFlay) replacements.set(18807, null);
	replacements.set(15473, talents.shadowform ? { spellId: 15473 } : null);
	replacements.set(14751, talents.innerFocus && level >= 20 ? { spellId: 14751 } : null);

	function hasUnavailableSpell(value: JsonValue | undefined): boolean {
		if (!value || typeof value !== 'object') return false;
		if (Array.isArray(value)) return value.some(hasUnavailableSpell);
		if (typeof value.spellId === 'number' && replacements.has(value.spellId) && replacements.get(value.spellId) === null) return true;
		return Object.values(value).some(hasUnavailableSpell);
	}

	function resolveRanks(value: JsonValue): JsonValue {
		if (!value || typeof value !== 'object') return value;
		if (Array.isArray(value)) return value.map(resolveRanks);
		if (typeof value.spellId === 'number' && replacements.has(value.spellId)) return { ...value, ...replacements.get(value.spellId)! };
		return Object.fromEntries(
			Object.entries(value)
				.filter(([, child]) => child !== undefined)
				.map(([key, child]) => [key, resolveRanks(child!)]),
		);
	}

	const template = (useStarshards ? StarshardsAPL : ForeverAPL) as unknown as {
		type: string;
		prepullActions: JsonValue[];
		priorityList: JsonValue[];
	};
	return resolveRanks({
		...template,
		prepullActions: template.prepullActions.filter(action => !hasUnavailableSpell(action)),
		priorityList: template.priorityList.filter(action => !hasUnavailableSpell(action)),
	});
}
import type { JsonValue } from '@protobuf-ts/runtime';
