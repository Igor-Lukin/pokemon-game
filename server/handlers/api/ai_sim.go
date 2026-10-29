package api

import (
	"fmt"
	mathrand "math/rand"
	"time"
)

// ai_sim.go — headless-симулятор для бенчмарка AI-команд и автоподбора.
// Использует ТОТ ЖЕ processTurn и боевые примитивы, что и живой бой; второго
// движка нет. Игрок в симуляторе управляется legacy smart-AI (без добровольных
// свитчей) — этого достаточно для сравнения силы команд.

type aiSimResult struct {
	Winner     string // player | enemy | timeout
	Turns      int
	PlayerHP   float64 // доля оставшегося HP команды игрока
	EnemyHP    float64
	PlayerLeft int
	EnemyLeft  int
	Log        []BattleLogEntry // для отладки/бенчмарка
}

type BenchmarkReport struct {
	NPCID     int
	AILevel   int
	Level     int
	Size      int
	Seeds     int
	BotWins   int
	BotLosses int
	Timeouts  int
	WinRate   float64 // BotWins / Seeds
	Tactics   bool
}

func cloneBattlePokemon(p BattlePokemon) BattlePokemon {
	c := p
	if p.SecondaryTypeID != nil {
		v := *p.SecondaryTypeID
		c.SecondaryTypeID = &v
	}
	c.Moves = cloneBattleMoves(p.Moves)
	c.SavedMoves = cloneBattleMoves(p.SavedMoves)
	c.UsedMoveIDs = append([]int(nil), p.UsedMoveIDs...)
	c.BattleTypes = append([]int(nil), p.BattleTypes...)
	c.ImprisonedMoveIDs = append([]int(nil), p.ImprisonedMoveIDs...)
	if p.abilityLogSeen != nil {
		c.abilityLogSeen = make(map[string]bool, len(p.abilityLogSeen))
		for k, v := range p.abilityLogSeen {
			c.abilityLogSeen[k] = v
		}
	}
	return c
}

func cloneBattleMoves(in []BattleMove) []BattleMove {
	out := make([]BattleMove, len(in))
	for i := range in {
		out[i] = in[i]
		out[i].StatChanges = append([]StatChange(nil), in[i].StatChanges...)
	}
	return out
}

func cloneTeam(team []BattlePokemon) []BattlePokemon {
	out := make([]BattlePokemon, len(team))
	for i := range team {
		out[i] = cloneBattlePokemon(team[i])
	}
	return out
}

func firstAliveIndex(team []BattlePokemon) int {
	for i := range team {
		if !team[i].IsFainted && team[i].CurrentHP > 0 {
			return i
		}
	}
	return -1
}

func teamHPRemaining(team []BattlePokemon) float64 {
	total, cur := 0, 0
	for i := range team {
		total += team[i].MaxHP
		if !team[i].IsFainted && team[i].CurrentHP > 0 {
			cur += team[i].CurrentHP
		}
	}
	if total <= 0 {
		return 0
	}
	return float64(cur) / float64(total)
}

func aliveCount(team []BattlePokemon) int {
	n := 0
	for i := range team {
		if !team[i].IsFainted && team[i].CurrentHP > 0 {
			n++
		}
	}
	return n
}

// simulateAIBattle — один бой. enemyTeam играет за бота уровня aiLevel,
// playerTeam — за legacy-AI.
func simulateAIBattle(playerTeam, enemyTeam []BattlePokemon, aiLevel int, seed int64) aiSimResult {
	mathrand.Seed(seed)
	pTeam := cloneTeam(playerTeam)
	eTeam := cloneTeam(enemyTeam)
	pIdx := firstAliveIndex(pTeam)
	eIdx := firstAliveIndex(eTeam)
	if pIdx < 0 && eIdx < 0 {
		return aiSimResult{Winner: "timeout", PlayerLeft: 0, EnemyLeft: 0}
	}
	if pIdx < 0 {
		return aiSimResult{Winner: "enemy", PlayerLeft: 0, EnemyLeft: aliveCount(eTeam)}
	}
	if eIdx < 0 {
		return aiSimResult{Winner: "player", PlayerLeft: aliveCount(pTeam), EnemyLeft: 0}
	}

	bs := &BattleState{
		ID:                fmt.Sprintf("sim-%d", seed),
		PlayerTeam:        pTeam,
		ActiveIdx:         pIdx,
		EnemyTeam:         eTeam,
		EnemyActiveIdx:    eIdx,
		WildPokemon:       &eTeam[eIdx],
		EnemySmart:        true,
		AILevel:           aiLevel,
		NoSwitch:          true, // бенчмарк: игрок не свитчится, значит и ИИ не должен
		TrainerName:       "bot",
		PlayerName:        "player",
		Turn:              0,
		StartedAt:         time.Now(),
		BattleType:        "trainer",
		BattleWeatherID:   1,
		BattleWeatherName: "Обычная",
	}

	var logs []BattleLogEntry
	abilityOnSwitchIn(bs, &bs.PlayerTeam[bs.ActiveIdx], &logs)
	abilityOnSwitchIn(bs, bs.WildPokemon, &logs)

	const maxTurns = 100
	for turn := 0; turn < maxTurns; turn++ {
		// Довести принудительные замены до боеспособного активного.
		for bs.PlayerTeam[bs.ActiveIdx].IsFainted {
			next := firstAliveIndex(bs.PlayerTeam)
			if next < 0 {
				return aiSimResult{Winner: "enemy", Turns: turn, PlayerHP: 0,
					EnemyHP: teamHPRemaining(bs.EnemyTeam), PlayerLeft: 0,
					EnemyLeft: aliveCount(bs.EnemyTeam), Log: logs}
			}
			bs.ActiveIdx = next
			applyHealingWish(&bs.PlayerSide, &bs.PlayerTeam[bs.ActiveIdx], bs.Turn, &logs)
			if applyEntryHazards(bs, &bs.PlayerTeam[bs.ActiveIdx], &bs.PlayerSide, bs.Turn, &logs) {
				logs = append(logs, BattleLogEntry{Round: bs.Turn,
					PokemonName: bs.PlayerTeam[bs.ActiveIdx].Name, Fainted: true})
			}
		}
		if bs.WildPokemon == nil || bs.WildPokemon.IsFainted {
			allDown, _ := handleEnemyFaint(bs, &logs)
			if allDown {
				return aiSimResult{Winner: "player", Turns: turn,
					PlayerHP: teamHPRemaining(bs.PlayerTeam), EnemyHP: 0,
					PlayerLeft: aliveCount(bs.PlayerTeam), EnemyLeft: 0, Log: logs}
			}
		}
		if bs.WildPokemon == nil {
			break
		}

		player := &bs.PlayerTeam[bs.ActiveIdx]
		var playerMove *BattleMove
		if player.ChargingMoveID != 0 || player.LockedMoveID != 0 || player.BideTurns > 0 ||
			(player.EncoreTurns != 0 && player.EncoreMoveID != 0) {
			playerMove = chooseWildMoveOrCharge(player)
		} else {
			playerMove = chooseSmartEnemyMove(player, bs.WildPokemon)
		}
		if playerMove == nil {
			playerMove = chooseWildMove(player)
		}
		if playerMove == nil {
			break
		}

		logs = append(logs, processTurn(bs, playerMove, 0)...)

		p := &bs.PlayerTeam[bs.ActiveIdx]
		applyWeatherChip(bs, p, bs.WildPokemon, &logs)
		applyFutureAttacks(bs, p, bs.WildPokemon, &logs)
		postProcessWeather(bs, &logs)
		tickSideTimers(bs, &logs)
		abilityOnEndTurn(bs, p, bs.Turn, &logs)
		if bs.WildPokemon != nil {
			abilityOnEndTurn(bs, bs.WildPokemon, bs.Turn, &logs)
		}
		heldItemOnEndTurn(bs, p, bs.Turn, &logs)
		if bs.WildPokemon != nil {
			heldItemOnEndTurn(bs, bs.WildPokemon, bs.Turn, &logs)
		}

		allDown, _ := handleEnemyFaint(bs, &logs)
		if allDown {
			return aiSimResult{Winner: "player", Turns: bs.Turn,
				PlayerHP: teamHPRemaining(bs.PlayerTeam), EnemyHP: 0,
				PlayerLeft: aliveCount(bs.PlayerTeam), EnemyLeft: 0, Log: logs}
		}
	}
	return aiSimResult{
		Winner: "timeout", Turns: maxTurns,
		PlayerHP: teamHPRemaining(bs.PlayerTeam), EnemyHP: teamHPRemaining(bs.EnemyTeam),
		PlayerLeft: aliveCount(bs.PlayerTeam), EnemyLeft: aliveCount(bs.EnemyTeam),
		Log: logs,
	}
}

// benchmarkTeams — прогнать seeds боёв на уже собранных командах.
func benchmarkTeams(playerTeam, enemyTeam []BattlePokemon, aiLevel, seeds int) BenchmarkReport {
	rep := BenchmarkReport{AILevel: aiLevel, Seeds: seeds}
	if seeds < 1 {
		seeds = 1
	}
	for seed := int64(1); seed <= int64(seeds); seed++ {
		res := simulateAIBattle(playerTeam, enemyTeam, aiLevel, seed)
		// playerTeam всегда управляется legacy-AI, enemyTeam — ботом.
		switch res.Winner {
		case "enemy":
			rep.BotWins++
		case "player":
			rep.BotLosses++
		default:
			rep.Timeouts++
		}
	}
	rep.WinRate = float64(rep.BotWins) / float64(seeds)
	return rep
}

// RunAIBenchmarkForNPC — публичная точка для cmd/ai_bench.
// tactics=true: обе стороны получают одну и ту же команду (проверяем тактику).
// tactics=false: бот-драфт против legacy-драфта (полная сила).
func RunAIBenchmarkForNPC(npcID, aiLevel, size, level, seeds int, tactics bool) (BenchmarkReport, error) {
	if size < 1 {
		size = 1
	} else if size > 6 {
		size = 6
	}
	botTeam, err := buildAITeam(npcID, aiLevel, size, level)
	if err != nil {
		return BenchmarkReport{}, err
	}
	var playerTeam []BattlePokemon
	if tactics {
		playerTeam = cloneTeam(botTeam)
	} else {
		// Честное сравнение: у legacy-команды тот же лимит легендарок, что и у
		// бота на этом уровне сложности.
		spec := aiSpecForLevel(aiLevel)
		playerTeam, err = buildSmartNPCTeamWithLimit(size, level, spec.MaxLegendaries)
		if err != nil {
			return BenchmarkReport{}, err
		}
	}
	rep := benchmarkTeams(playerTeam, botTeam, aiLevel, seeds)
	rep.NPCID = npcID
	rep.Level = level
	rep.Size = size
	rep.Tactics = tactics
	return rep, nil
}

// RunAIAutoPick — сгенерировать variants команд, прогнать seeds боёв каждую,
// сохранить лучшие в npc_ai_teams (source=auto, пул top-5).
func RunAIAutoPick(npcID, aiLevel, size, level, variants, seeds int) ([]BenchmarkReport, error) {
	if variants < 1 {
		variants = 1
	}
	var reports []BenchmarkReport
	for i := 0; i < variants; i++ {
		botTeam, err := buildAITeam(npcID, aiLevel, size, level)
		if err != nil {
			return reports, err
		}
		spec := aiSpecForLevel(aiLevel)
		legacy, err := buildSmartNPCTeamWithLimit(size, level, spec.MaxLegendaries)
		if err != nil {
			return reports, err
		}
		rep := benchmarkTeams(legacy, botTeam, aiLevel, seeds)
		rep.NPCID = npcID
		rep.Level = level
		rep.Size = size
		reports = append(reports, rep)
		if err := saveAITeam(npcID, aiLevel, level, botTeam, rep.WinRate, "auto"); err != nil {
			return reports, err
		}
	}
	return reports, nil
}
