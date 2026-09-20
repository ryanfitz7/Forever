package priest

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/simsignals"
	"github.com/wowsims/classic/sim/core/stats"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type priestTestAgent struct{ *Priest }

func (agent *priestTestAgent) Reset(*core.Simulation) {}

func init() {
	core.RegisterAgentFactory(proto.Player_ShadowPriest{}, proto.Spec_SpecShadowPriest,
		func(character *core.Character, player *proto.Player) core.Agent {
			level, _ := strconv.Atoi(player.Name)
			character.Level = int32(level)
			return &priestTestAgent{New(character, player.TalentsString)}
		}, func(player *proto.Player, spec interface{}) { player.Spec = spec.(*proto.Player_ShadowPriest) })
}

func testTalentString(talents *proto.PriestTalents) string {
	message := talents.ProtoReflect()
	var trees []string
	field := 1
	for _, size := range TalentTreeSizes {
		var tree strings.Builder
		for i := 0; i < size; i++ {
			descriptor := message.Descriptor().Fields().ByNumber(protoreflect.FieldNumber(field))
			value := message.Get(descriptor)
			points := int64(0)
			if descriptor.Kind() == protoreflect.BoolKind {
				if value.Bool() {
					points = 1
				}
			} else {
				points = value.Int()
			}
			tree.WriteString(strconv.FormatInt(points, 10))
			field++
		}
		trees = append(trees, tree.String())
	}
	return strings.Join(trees, "-")
}

func newPriestTestSim(t *testing.T, race proto.Race, level int32, talents *proto.PriestTalents, ruleset proto.Ruleset) (*core.Simulation, *Priest) {
	t.Helper()
	sim := core.NewSim(&proto.RaidSimRequest{
		SimOptions: &proto.SimOptions{RandomSeed: 101, IsTest: true, Ruleset: ruleset},
		Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{{
			Name: strconv.Itoa(int(level)), Race: race, Class: proto.Class_ClassPriest,
			Equipment: &proto.EquipmentSpec{}, TalentsString: testTalentString(talents),
			Rotation: &proto.APLRotation{Type: proto.APLRotation_TypeAPL},
			Spec:     &proto.Player_ShadowPriest{ShadowPriest: &proto.ShadowPriest{Options: &proto.ShadowPriest_Options{}}},
		}}}}},
		Encounter: &proto.Encounter{Duration: 100, ExecuteProportion_20: .2, Targets: []*proto.Target{{Level: level}}},
	}, simsignals.CreateSignals())
	sim.Reset()
	p := sim.Raid.Parties[0].Players[0].(*priestTestAgent).Priest
	p.AddStatDynamic(sim, stats.SpellCrit, -p.GetStat(stats.SpellCrit))
	return sim, p
}

func closeEnough(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-5 {
		t.Fatalf("%s: got %.9f, want %.9f", label, got, want)
	}
}

func TestForeverRankGrowthMatchesClientRecords(t *testing.T) {
	raw, err := os.ReadFile("testdata/forever_direct_spell_records.json")
	if err != nil {
		t.Fatal(err)
	}
	var source struct {
		Tables []struct {
			Table   string
			SpellID int32 `json:"spell_id"`
			Records []map[string]string
		}
	}
	if err := json.Unmarshal(raw, &source); err != nil {
		t.Fatal(err)
	}
	for _, group := range []struct {
		ids   []int32
		ranks []foreverDirectSpellRank
	}{
		{MindBlastSpellId[:], mindBlastForeverRanks[:]},
		{ShadowWordDeathSpellId[:], shadowWordDeathForeverRanks[:]},
		{DarkSacrificeSpellId[:], darkSacrificeTickRanks[:]},
	} {
		for rank := 1; rank < len(group.ids); rank++ {
			data := group.ranks[rank]
			for _, table := range source.Tables {
				if table.SpellID != group.ids[rank] {
					continue
				}
				row := table.Records[0]
				get := func(key string) float64 {
					n, e := strconv.ParseFloat(row[key], 64)
					if e != nil {
						t.Fatal(e)
					}
					return n
				}
				if table.Table == "SpellEffect" {
					closeEnough(t, "client base", data.mean, get("EffectBasePointsF"))
					closeEnough(t, "client growth", data.pointsPerLevel, get("EffectRealPointsPerLevel"))
					closeEnough(t, "client variance", data.variance, get("Variance"))
				} else {
					closeEnough(t, "training level", float64(data.spellLevel), get("SpellLevel"))
					closeEnough(t, "growth cap", float64(data.maxLevel), get("MaxLevel"))
				}
			}
			low, high := data.damageRange(data.spellLevel)
			closeEnough(t, "training mean", (low+high)/2, data.mean)
			low, high = data.damageRange(data.maxLevel + 10)
			closeEnough(t, "capped mean", (low+high)/2, data.mean+float64(data.maxLevel-data.spellLevel)*data.pointsPerLevel)
		}
	}
	low, high := mindBlastForeverRanks[9].damageRange(60)
	closeEnough(t, "level 60 Mind Blast mean", (low+high)/2, 490.2)
	low, high = shadowWordDeathForeverRanks[4].damageRange(60)
	closeEnough(t, "level 60 Death mean", (low+high)/2, 458)
}

func TestForeverPriestManaDiscounts(t *testing.T) {
	sim, p := newPriestTestSim(t, proto.Race_RaceNightElf, 60, &proto.PriestTalents{
		MindFlay: true, Penance: true, MentalAgility: 3, DevouringContagion: 2, Shadowform: true, InnerFocus: true,
	}, proto.Ruleset_RulesetForever)
	p.ShadowformAura.Activate(sim)
	for _, check := range []struct {
		spell *core.Spell
		cost  float64
	}{
		{p.ShadowWordPain[8], 470 * .9 * .5}, {p.DevouringPlague[6], 985 * .5 * .9 * .5},
		{p.ShadowWordDeath[4], 340 * .9 * .5}, {p.MindBlast[9], 350 * .5},
		{p.MindFlay[6][0], 205 * .5}, {p.Starshards[7][0], 350},
	} {
		closeEnough(t, check.spell.ActionID.String(), check.spell.Cost.GetCurrentCost(), check.cost)
	}
	for _, spell := range []*core.Spell{p.Smite[8], p.HolyFire[8]} {
		if spell == nil {
			t.Fatal("missing Holy spell")
		}
	}
	p.InnerFocusAura.Activate(sim)
	closeEnough(t, "free Plague with Inner Focus", p.DevouringPlague[6].Cost.GetCurrentCost(), 0)
	p.InnerFocusAura.Deactivate(sim)
	closeEnough(t, "Plague cost after Inner Focus", p.DevouringPlague[6].Cost.GetCurrentCost(), 221.625)
}

func TestForeverStarshardsSharedCooldownAndSchool(t *testing.T) {
	sim, p := newPriestTestSim(t, proto.Race_RaceNightElf, 60, &proto.PriestTalents{Shadowform: true, Darkness: 5}, proto.Ruleset_RulesetForever)
	p.ShadowformAura.Activate(sim)
	p.AddStatDynamic(sim, stats.ShadowPower, 1000)
	p.AddStatDynamic(sim, stats.ArcanePower, 300)
	spell := p.Starshards[7][0]
	spell.Flags |= core.SpellFlagIgnoreResists
	dot := spell.Dot(p.CurrentTarget)
	dot.Apply(sim)
	dot.TickOnce(sim)
	closeEnough(t, "Starshards Arcane-only tick", spell.SpellMetrics[0].TotalDamage, 350.1)
	if dot.NumberOfTicks != 6 || dot.TickLength != time.Second {
		t.Fatal("wrong Starshards cadence")
	}
	spell.CD.Use(sim)
	for rank := 1; rank <= StarshardsRanks; rank++ {
		for tick := 0; tick < StarshardsTicks; tick++ {
			if p.Starshards[rank][tick].ReadyAt() != 30*time.Second {
				t.Fatal("Starshards rank/clip bypassed cooldown")
			}
		}
	}
	_, classic := newPriestTestSim(t, proto.Race_RaceNightElf, 60, &proto.PriestTalents{}, proto.Ruleset_RulesetClassic)
	if classic.Starshards[7][0].CD.Duration != 0 {
		t.Fatal("changed Classic cooldown")
	}
}

func TestForeverTwinDisciplinesExcludesChannels(t *testing.T) {
	_, p := newPriestTestSim(t, proto.Race_RaceNightElf, 60, &proto.PriestTalents{
		MindFlay: true, Penance: true, TwinDisciplines: 5,
	}, proto.Ruleset_RulesetForever)
	for _, spell := range []*core.Spell{p.MindFlay[6][0], p.Starshards[7][0], p.Penance} {
		closeEnough(t, "channel excluded from Twin Disciplines", spell.DamageMultiplierAdditive, 1)
	}
	for _, spell := range []*core.Spell{p.ShadowWordDeath[4], p.ShadowWordPain[8], p.DevouringPlague[6]} {
		closeEnough(t, "instant receives Twin Disciplines", spell.DamageMultiplierAdditive, 1.05)
	}
}

func TestForeverDeathRanksExecuteAndBacklash(t *testing.T) {
	for _, level := range []int32{31, 32, 39, 40, 48, 56, 60} {
		_, p := newPriestTestSim(t, proto.Race_RaceHuman, level, &proto.PriestTalents{}, proto.Ruleset_RulesetForever)
		for rank := 1; rank <= ShadowWordDeathRanks; rank++ {
			if (p.ShadowWordDeath[rank] != nil) != (ShadowWordDeathLevel[rank] <= int(level)) {
				t.Fatalf("wrong Death rank %d at level %d", rank, level)
			}
		}
	}
	_, classic := newPriestTestSim(t, proto.Race_RaceHuman, 60, &proto.PriestTalents{}, proto.Ruleset_RulesetClassic)
	if len(classic.ShadowWordDeath) != 0 {
		t.Fatal("Death registered in Classic")
	}
	for points := int32(0); points <= 2; points++ {
		t.Run(fmt.Sprint(points), func(t *testing.T) {
			sim, p := newPriestTestSim(t, proto.Race_RaceHuman, 60, &proto.PriestTalents{EarlyDemise: points, Shadowform: true}, proto.Ruleset_RulesetForever)
			p.ShadowformAura.Activate(sim)
			spell := p.ShadowWordDeath[4]
			before := spell.ExpectedInitialDamage(sim, p.CurrentTarget)
			for !sim.IsExecutePhase20() {
				if sim.Step() {
					t.Fatal("execute phase never began")
				}
			}
			after := spell.ExpectedInitialDamage(sim, p.CurrentTarget)
			closeEnough(t, "Early Demise expected crit contribution", after/before, 1+.15*float64(points))
			closeEnough(t, "execute crit must not leak", spell.BonusCritRating, 0)
			health := p.CurrentHealth()
			spell.BonusHitRating += 1000
			spell.ApplyEffects(sim, p.CurrentTarget, spell)
			closeEnough(t, "Death backlash", health-p.CurrentHealth(), .1*p.MaxHealth())
			spell.CD.Use(sim)
			for rank := 1; rank <= ShadowWordDeathRanks; rank++ {
				if p.ShadowWordDeath[rank].TimeToReady(sim) != 15*time.Second {
					t.Fatal("Death ranks do not share cooldown")
				}
			}
			sim.Encounter.EndFightAtHealth = sim.Encounter.DamageTaken + 1
			health = p.CurrentHealth()
			spell.ApplyEffects(sim, p.CurrentTarget, spell)
			closeEnough(t, "killing blow has no backlash", p.CurrentHealth(), health)
		})
	}
}

func TestForeverDarkSacrificeAndPlagueHealing(t *testing.T) {
	sim, p := newPriestTestSim(t, proto.Race_RaceUndead, 60, &proto.PriestTalents{}, proto.Ruleset_RulesetForever)
	p.SpendMana(sim, 1700, p.NewManaMetrics(core.ActionID{SpellID: 1277328}))
	spell := p.DarkSacrifice[5]
	health, mana := p.CurrentHealth(), p.CurrentMana()
	spell.ApplyEffects(sim, &p.Unit, spell)
	closeEnough(t, "Dark Sacrifice no upfront mana", p.CurrentMana(), mana)
	if spell.SelfHot().NextTickAt() != 3*time.Second {
		t.Fatal("wrong first Dark Sacrifice tick")
	}
	for i := 1; i <= 5; i++ {
		spell.SelfHot().TickOnce(sim)
		closeEnough(t, "Dark Sacrifice mana", p.CurrentMana()-mana, 320*float64(i))
	}
	closeEnough(t, "Dark Sacrifice health", health-p.CurrentHealth(), 1600)
	if spell.CD.Duration != 10*time.Minute {
		t.Fatal("wrong Dark Sacrifice cooldown")
	}
	plague := p.DevouringPlague[6]
	plague.Flags |= core.SpellFlagIgnoreResists
	plague.Dot(p.CurrentTarget).Apply(sim)
	health = p.CurrentHealth()
	plague.Dot(p.CurrentTarget).TickOnce(sim)
	closeEnough(t, "Plague heals its damage", p.CurrentHealth()-health, plague.SpellMetrics[0].TotalDamage)
	_, human := newPriestTestSim(t, proto.Race_RaceHuman, 60, &proto.PriestTalents{}, proto.Ruleset_RulesetForever)
	if len(human.DarkSacrifice) != 0 {
		t.Fatal("Dark Sacrifice available outside Undead")
	}
}
