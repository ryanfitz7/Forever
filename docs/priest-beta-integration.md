# Priest beta integration — 20 September 2026

This fork builds on [ElliotWood/Forever](https://github.com/ElliotWood/Forever) at
`b2e6a49ef`, preserving its equipment editor, talent trees, encounter settings,
simulation results, and editable APL interface. Priest is the first class reviewed
against the local research archive. The other class modules remain available, but
have not received the same review in this fork.

## Choosing the base

The [official Forever project](https://github.com/wowsims/forever) is actively being
developed, but its Priest spell implementations currently contain `To be implemented`
panics and its Shadow Priest test is skipped. Its generated spell data are useful
for comparison; its running Warrior simulator does not establish Priest coverage.

The inspected ElliotWood derivatives did not provide a more complete Priest engine:
ProfetGit/forever-sim was identical, Alexwaukee/ForeverCapped was behind, and
sage3648/mythicsim-forever-engine uses the same underlying engine. Hosted alternatives
include [wowforeversim.com](https://wowforeversim.com/) and
[MythicSim](https://www.mythicsim.com/wow-forever/sim). This is a dated review, not a claim that no future
fork can improve on this base.

## Evidence and behavior

The original source site is [Forever Changes](https://foreverchanges.pro/spellbook/priest).
The September 17 spellbook/talent capture is build **1.60.1.69893**. Targeted
September 19 racial records are **1.60.1.69913**; they are not a complete newer-build
recapture. [The archived inputs](evidence/priest-2026-09/) retain source URLs,
timestamps, and available hashes. The archive contains 239 Priest spell variants and
148 talent-rank descriptions; archiving a spell does not imply the engine models it.

| Ability | Reviewed inputs and changes |
|---|---|
| Mind Blast | Nine client rank records; .429 coefficient; damage growth evaluated at the selected character level rather than precomputed at the rank cap. |
| Mind Flay | Three ticks, .167 coefficient per tick; existing Forever rank bases retained. Mental Agility no longer treats a channel as an instant spell. |
| Shadow Word: Pain | Six baseline ticks, .20 per tick; Improved Pain adds duration/ticks. Existing periodic crit behavior retained. |
| Devouring Plague | Every Priest race; eight ticks, .10 per tick, 60-second cooldown. Damage returns health. |
| Shadow Word: Death | Four Forever spell ranks; .429 coefficient, 15-second shared cooldown, Early Demise's 15/30 percentage-point execute crit bonus, and 10%-maximum-health backlash after a non-killing landed hit. |
| Starshards | Six Arcane ticks, .167 per tick; 30-second cooldown shared across ranks and channel variants. Shadow-only bonuses do not apply. |
| Eureka | Priest spell 1259823: 15% mana reduction, 10% damage, three charges. Damage changes while the buff is active, including on already-running DoTs, and stops when it expires. |
| Dark Sacrifice | Five Undead-only ranks, ten-minute shared cooldown. Rank five transfers 320 health to mana every three seconds for five ticks. Available for explicit APL use. |

Mental Agility and Twin Disciplines distinguish instant spells from channels.
Devouring Contagion and Mental Agility use multiplicative mana discounts under the
Forever model, avoiding the inherited additive combination that made Plague free
with Shadowform. Inner Focus still makes its eligible next spell free.

Fresh Wago SpellEffect and SpellLevels rows for every Mind Blast and Death rank live
in `sim/priest/testdata/forever_direct_spell_records.json`. Linear growth up to the
client MaxLevel and continuous damage variance are explicit model assumptions:
server rounding is not established. At level 60 the modeled means before spell power
are 490.2 for Mind Blast and 458 for Death; these are not their training-level tooltip
means of 485 and 448.

The supplied 14/3/34 allocation is mapped by talent name into the Forever field order.
Auto resolves available spell ranks at the UI's current level cap (60) and removes unavailable
talent/racial actions. Full-channel and clipped-Flay APL presets are comparison
starting points, not a proven optimal rotation. The rank resolver and engine support
lower-level inputs, but this inherited UI does not yet provide a character-level picker.
The existing APL editor remains
available for custom priorities and channel conditions.

## Spell power and damage formula audit

For the six spells in the Shadow rotation, a normal, unresisted hit or tick is:

`(rank base damage + coefficient × eligible spell power) × damage multipliers`

Eligible spell power includes generic spell damage and the spell's own school bonus.
Shadow power does not increase Arcane Starshards. Critical outcomes, target modifiers,
and resistance are separate from the calculation shown here.

| Spell | Spell-power coefficient | Total for a completed base-duration cast |
|---|---:|---:|
| Mind Blast | 0.429 per hit | 0.429 |
| Shadow Word: Death | 0.429 per hit | 0.429 |
| Mind Flay | 0.167 per tick | 0.501 across three ticks |
| Shadow Word: Pain | 0.200 per tick | 1.200 across six ticks |
| Devouring Plague | 0.100 per tick | 0.800 across eight ticks |
| Starshards | 0.167 per tick | 1.002 across six ticks |

These are explicit beta client `SpellEffect.EffectBonusCoefficient` values, rounded
only to their intended decimal precision, not estimates from cast time. In particular,
0.167 is not silently replaced with 1/6. Improved Pain adds ticks with the same 0.200
coefficient: eight ticks at two talent points contribute 1.600 spell power in total.
Clipping a channel removes the damage and spell-power contribution of its skipped ticks.
An additional targeted check against build **1.60.1.69913** found the same coefficients
for 18 representative low/high ranks; the selected record fields and URLs are preserved in
`sim/priest/testdata/forever_coefficient_audit_69913.json`. This supplements the original
archive rather than claiming a full newer-build recapture.

Darkness at five points, Shadow Weaving at five stacks, and Shadowform are separate
factors: `1.10 × 1.10 × 1.10 = 1.331`, or **33.1% more Shadow damage**. Weaving's own
five stacks form a single 10% factor; they are not five separate 2% multipliers.
Improved Mind Flay adds a separate 1.20 factor at two points. Twin Disciplines adds a
separate 1.05 factor to eligible instant spells at five points, excluding channels.

For example, with 500 eligible spell power, rank-six Mind Flay, two points in Improved
Mind Flay, and all three Shadow bonuses active at application, each ordinary tick is
`(130 + 0.167 × 500) × 1.331 × 1.20 = 341.0022` before target mitigation. This is not
a DPS estimate: critical hits, misses, fight timing, gear effects, and resource limits
still affect simulation results.

The regression tests in `sim/priest/forever_damage_formula_test.go` check the actual
damage pipeline and archived coefficient records. They establish that the engine
implements this formula; client records alone do not prove every beta server stacking
or snapshot rule. In particular, ordinary periodic spells retain the spell power and
Shadow Weaving multiplier from application; only the reviewed Eureka damage bonus is
dynamic. A full no-snapshot model still requires controlled combat-log evidence.

The Mind Flay and Starshards APL expected-tick helpers now include Forever's periodic
critical hits and honor the request to estimate an existing snapshot. Current crit
chance is still used, matching actual Forever ticks. These helpers estimate damage
conditional on the channel landing; they do not apply the initial hit chance again.

The inherited default equipment includes an Engineering-only Green Lens. The default
profession selection now satisfies that requirement; this is not a profession ranking.

## Validation

The complete `go test --tags=with_db ./sim/...` suite passes. Priest integration tests
exercise 13 rotation/race combinations (20 iterations each), require Death and Plague
casts, reject APL warnings, and verify actual two-tick Flay clipping. Separate tests
cover rank records, mana stacking, Eureka expiry, execute crits, backlash, healing,
resource ticks, and shared cooldowns. Source-manifest/talent consistency checks,
TypeScript, and `node tools/check_shadow_presets.cjs` also pass.

The Windows production build is tested in the browser through the normal Simulate
button. These checks validate the implementation against the stated model, not
against a complete set of controlled beta combat logs.

## Limits that still affect conclusions

- Eureka's no-snapshot damage behavior comes from user-reported beta testing. Its
  channel charge timing remains an assumption (consumption at channel start).
  Death's client mask does not overlap the displayed Eureka modifiers; generic
  damaging-spell eligibility is retained as an explicit uncertainty pending a live test.
- Mana discounts use multiplicative stacking for the reviewed Priest interactions.
  The exact beta server's stacking and rounding still require combat-log verification.
- Touch of the Grave excludes periodic tick callbacks, consistent with the reported
  application-only behavior. Its inherited 5% proc chance, damage range, mitigation,
  and lack of an internal cooldown remain assumptions; racial rankings are provisional.
- Ordinary DoT damage snapshots remain inherited except for Eureka. Live periodic crit
  is already part of the Forever ruleset. No universal no-snapshot rule is claimed.
- Mind Blast's cooldown starts on cast completion in this engine. Client cooldown
  fields alone do not prove that timing on the beta server.
- The damage simulator does not halt a Priest's rotation at zero health. Death backlash
  and optional health-for-mana abilities are resource accounting, not a survival model.
- The gear database includes the upstream Forever data work, but availability, hotfix
  discrepancies, presets, and inherited EP weights still need separate validation.
  A gear preset is not a certified best-in-slot list.
- The Classic switch changes engine rules, but the talent schema is still Forever's.
  It is not a full Classic character simulator.

For the next class, follow the same pattern: versioned source records, rank metadata,
class-local formulas, explicit uncertain mechanics, meaningful engine tests, and an
Auto/APL smoke test. Do not copy Priest coefficients into another class or silently
mix builds. The upstream class registry and protobuf interfaces already support the
nine-class expansion; no replacement frontend is needed.

## Run on Windows

Use PowerShell 7, Node 20 or newer, Go 1.23.4 or newer, Python, and
`protoc-gen-go` v1.36.6. The npm-pinned protobuf tool supplies `protoc` and its
TypeScript plugin. Install the Go plugin with
`go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.6` and add its directory
to PATH.

```powershell
npm ci
./tools/build_windows.ps1
python -m http.server 8766 --bind 127.0.0.1 --directory dist
```

Open `http://127.0.0.1:8766/classic/shadow_priest/`. The build script also recognizes
a portable Go installation at `../.toolchain/go/bin`; use `-GoBin` to supply another.
Linux/macOS can continue using the inherited Makefile. `SITE_REPO` routes repository
links to this fork. Uploads require an explicitly configured `VITE_UPLOAD_URL`; the
fork does not send contributed files to the original maintainer's collector.
