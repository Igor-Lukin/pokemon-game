//go:build aitest

package api

import (
	"io"
	"log"
	mathrand "math/rand"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"pokemon-game/server/db"
)

func aiTestChdir(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
}

func aiTeamNames(team []BattlePokemon) string {
	out := ""
	for i := range team {
		p := &team[i]
		var legendary int
		db.DB.QueryRow("SELECT COALESCE(is_legendary,0) FROM pokemons WHERE id = ?", p.PokemonID).Scan(&legendary)
		mark := ""
		if legendary != 0 {
			mark = "[L]"
		}
		if out != "" {
			out += ", "
		}
		out += p.Name + mark
	}
	return out
}

func TestAITeamSmoke(t *testing.T) {
	aiTestChdir(t)
	db.InitDB()
	mathrand.Seed(20261023)

	for _, aiLevel := range []int{1, 2} {
		team, err := buildAITeam(1, aiLevel, 6, 50)
		if err != nil {
			t.Fatalf("L%d buildAITeam: %v", aiLevel, err)
		}
		if len(team) == 0 {
			t.Fatalf("L%d пустая команда", aiLevel)
		}
		legends := 0
		for i := range team {
			p := &team[i]
			if p.MaxHP <= 0 || p.Attack <= 0 || p.Speed <= 0 {
				t.Errorf("L%d %s: пустые статы %+v", aiLevel, p.Name, p)
			}
			if p.NatureID == 0 {
				t.Errorf("L%d %s: нет характера", aiLevel, p.Name)
			}
			if len(p.Moves) == 0 {
				t.Errorf("L%d %s: нет ходов", aiLevel, p.Name)
			}
			var legendary int
			db.DB.QueryRow("SELECT COALESCE(is_legendary,0) FROM pokemons WHERE id = ?", p.PokemonID).Scan(&legendary)
			if legendary != 0 {
				legends++
			}
			t.Logf("L%d #%d %-14s ур.%d HP=%d Atk=%d Def=%d Spd=%d SpA=%d SpD=%d ability=%s nature=%d item=%s IV=%v EV=%v moves=%s",
				aiLevel, p.PokemonID, p.Name, p.Level, p.MaxHP, p.Attack, p.Defense,
				p.Speed, p.SpecialAttack, p.SpecialDefense, p.AbilityName, p.NatureID,
				p.HeldItemName, p.IV, p.EV, aiMoveNames(p.Moves))
		}
		switch aiLevel {
		case 1:
			if legends != 0 {
				t.Errorf("L1 легенд %d, ожидалось 0", legends)
			}
		case 2:
			if legends > 1 {
				t.Errorf("L2 легенд %d, ожидалось <=1", legends)
			}
		}
	}
}

func TestAIStrengthDebug(t *testing.T) {
	aiTestChdir(t)
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	db.InitDB()
	mathrand.Seed(1000)
	legacy, err := buildSmartNPCTeamWithLimit(6, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	bot, err := buildAITeamFromLegacy(legacy, 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	res := simulateAIBattle(legacy, bot, 0, 1)
	t.Logf("RESULT winner=%s turns=%d playerHP=%.2f enemyHP=%.2f", res.Winner, res.Turns, res.PlayerHP, res.EnemyHP)
	for _, e := range res.Log {
		t.Logf("r%-3d %-14s mv=%-22s dmg=%-3d faint=%v eff=%.1f msg=%s",
			e.Round, e.PokemonName, e.MoveName, e.Damage, e.Fainted, e.Effectiveness, e.Message)
	}
}

func TestAICompare(t *testing.T) {
	aiTestChdir(t)
	db.InitDB()
	mathrand.Seed(1000)
	legacy, err := buildSmartNPCTeamWithLimit(6, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	bot, err := buildAITeamFromLegacy(legacy, 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	for i := range legacy {
		l, b := &legacy[i], &bot[i]
		t.Logf("%s legacy: HP=%d Atk=%d Def=%d Spd=%d SpA=%d SpD=%d ab=%s item=%s moves=%s",
			l.Name, l.MaxHP, l.Attack, l.Defense, l.Speed, l.SpecialAttack, l.SpecialDefense,
			l.AbilityName, l.HeldItemName, aiMoveNames(l.Moves))
		t.Logf("%s bot   : HP=%d Atk=%d Def=%d Spd=%d SpA=%d SpD=%d ab=%s item=%s moves=%s",
			b.Name, b.MaxHP, b.Attack, b.Defense, b.Speed, b.SpecialAttack, b.SpecialDefense,
			b.AbilityName, b.HeldItemName, aiMoveNames(b.Moves))
	}
}

func TestAISimSmoke(t *testing.T) {
	aiTestChdir(t)
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	db.InitDB()

	const drafts = 10
	const perDraft = 20
	for _, aiLevel := range []int{1, 2} {
		spec := aiSpecForLevel(aiLevel)
		var fullW, fullL, fullT int
		var tacW, tacL, tacT int
		var strW, strL, strT int
		var firstTeam string

		for d := 0; d < drafts; d++ {
			mathrand.Seed(int64(1000 + d))
			legacyFair, err := buildSmartNPCTeamWithLimit(6, 50, spec.MaxLegendaries)
			if err != nil {
				t.Fatalf("L%d draft %d legacy: %v", aiLevel, d, err)
			}
			bot, err := buildAITeamFromLegacy(legacyFair, aiLevel, 50)
			if err != nil {
				t.Fatalf("L%d draft %d bot: %v", aiLevel, d, err)
			}
			if d == 0 {
				firstTeam = aiTeamNames(bot)
			}

			full := benchmarkTeams(legacyFair, bot, aiLevel, perDraft)
			fullW += full.BotWins
			fullL += full.BotLosses
			fullT += full.Timeouts

			tac := benchmarkTeams(bot, bot, aiLevel, perDraft)
			tacW += tac.BotWins
			tacL += tac.BotLosses
			tacT += tac.Timeouts

			str := benchmarkTeams(legacyFair, bot, 0, perDraft)
			strW += str.BotWins
			strL += str.BotLosses
			strT += str.Timeouts
			t.Logf("L%d draft %d: full=%d/%d/%d tactics=%d/%d/%d strength=%d/%d/%d team=%s",
				aiLevel, d, full.BotWins, full.BotLosses, full.Timeouts,
				tac.BotWins, tac.BotLosses, tac.Timeouts, str.BotWins, str.BotLosses, str.Timeouts,
				aiTeamNames(bot))
		}

		total := drafts * perDraft
		t.Logf("L%d drafts=%d battles=%d first team: %s", aiLevel, drafts, total, firstTeam)
		t.Logf("L%d bot vs legacy fair (same species): wins=%d losses=%d timeouts=%d winrate=%.2f",
			aiLevel, fullW, fullL, fullT, float64(fullW)/float64(total))
		t.Logf("L%d bot tactics (same team): wins=%d losses=%d timeouts=%d winrate=%.2f",
			aiLevel, tacW, tacL, tacT, float64(tacW)/float64(total))
		t.Logf("L%d bot team strength (both legacy AI): wins=%d losses=%d timeouts=%d winrate=%.2f",
			aiLevel, strW, strL, strT, float64(strW)/float64(total))
	}
}
