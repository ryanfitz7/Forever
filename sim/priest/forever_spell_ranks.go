package priest

// These are SpellEffect and SpellLevels fields from build 1.60.1.69893.
// The source rows for every rank are retained in testdata/forever_direct_spell_records.json.
// Linear growth up to MaxLevel, followed by the client variance without integer
// rounding, is a model assumption; the beta server's rounding is not verified.
type foreverDirectSpellRank struct {
	mean, variance, pointsPerLevel float64
	spellLevel, maxLevel           int
}

func (rank foreverDirectSpellRank) damageRange(level int) (float64, float64) {
	mean := rank.mean + float64(max(0, min(level, rank.maxLevel)-rank.spellLevel))*rank.pointsPerLevel
	return mean * (1 - rank.variance/2), mean * (1 + rank.variance/2)
}

var mindBlastForeverRanks = [MindBlastRanks + 1]foreverDirectSpellRank{
	{},
	{39, .09756097198, .6, 10, 15},
	{68, .07999999821, .9, 16, 21},
	{101, .06896551698, 1.1, 22, 27},
	{151, .05813953653, 1.4, 28, 33},
	{196, .0625, 1.6, 34, 39},
	{258, .0625, 1.9, 40, 45},
	{324, .05617977679, 2.1, 46, 51},
	{405, .05491990969, 2.4, 52, 57},
	{485, .05415860564, 2.6, 58, 63},
}

var shadowWordDeathForeverRanks = [ShadowWordDeathRanks + 1]foreverDirectSpellRank{
	{},
	{295, .06049999967, 1.5, 32, 37},
	{370, .06049999967, 1.9, 40, 45},
	{403, .06049999967, 2.2, 48, 53},
	{448, .06049999967, 2.5, 56, 61},
}
