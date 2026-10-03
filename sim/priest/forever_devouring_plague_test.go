package priest

import (
	"fmt"
	"testing"
	"time"

	"github.com/wowsims/classic/sim/core/proto"
)

func TestForeverDevouringPlagueOneMinuteCooldown(t *testing.T) {
	for rank := 1; rank <= 6; rank++ {
		t.Run(fmt.Sprintf("Rank%d", rank), func(t *testing.T) {
			sim, priest := newPriestTestSim(t, proto.Race_RaceNightElf, 60, &proto.PriestTalents{}, proto.Ruleset_RulesetForever)
			spell := priest.DevouringPlague[rank]
			if spell.CD.Duration != 60*time.Second {
				t.Fatalf("cooldown = %s, want 60s", spell.CD.Duration)
			}
			if !spell.Cast(sim, priest.CurrentTarget) {
				t.Fatal("initial Devouring Plague cast failed")
			}
			// All ranks share the cooldown; switching rank must not bypass it.
			sim.CurrentTime = 60*time.Second - time.Nanosecond
			for otherRank := 1; otherRank <= 6; otherRank++ {
				if priest.DevouringPlague[otherRank].IsReady(sim) {
					t.Fatalf("rank %d ready before 60s", otherRank)
				}
			}
			sim.CurrentTime = 60 * time.Second
			for otherRank := 1; otherRank <= 6; otherRank++ {
				if !priest.DevouringPlague[otherRank].IsReady(sim) {
					t.Fatalf("rank %d not ready at 60s", otherRank)
				}
			}
		})
	}
}
