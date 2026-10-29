package api

import (
	"log"
	mathrand "math/rand"
	"sort"
	"strings"
)

// ai_battle.go — боевой мозг NPC-бота (ai_level 1/2).
//
// L1: 1-ply, 20–30% случайных легальных ходов.
// L2: 2-ply, учитывает вероятный ответ игрока по уже открытым ходам.
//
// Честность: бот не читает выбранный ход игрока и не знает его способность/
// предмет до проявления. Для оценки урона способность/предмет неизвестной
// стороны обнуляются, а ходы игрока берутся только из UsedMoveIDs (открытые).

type battleAgent interface {
	ChooseMove(bs *BattleState, me, opp *BattlePokemon) *BattleMove
	ChooseSwitch(bs *BattleState, me, opp *BattlePokemon) int
}

type aiAgent struct {
	level int
	spec  aiSpec
}

func (a aiAgent) ChooseMove(bs *BattleState, me, opp *BattlePokemon) *BattleMove {
	return chooseAIMoveWithSpec(bs, me, opp, a.spec)
}

func (a aiAgent) ChooseSwitch(bs *BattleState, me, opp *BattlePokemon) int {
	return chooseAISwitchWithSpec(bs, me, opp, a.spec)
}

func agentForLevel(aiLevel int) battleAgent {
	return aiAgent{level: aiLevel, spec: aiSpecForLevel(aiLevel)}
}

func chooseAIMove(bs *BattleState, me, opp *BattlePokemon) *BattleMove {
	return agentForLevel(bs.AILevel).ChooseMove(bs, me, opp)
}

func chooseAISwitch(bs *BattleState, me, opp *BattlePokemon) int {
	return agentForLevel(bs.AILevel).ChooseSwitch(bs, me, opp)
}

func chooseAIMoveWithSpec(bs *BattleState, me, opp *BattlePokemon, spec aiSpec) *BattleMove {
	if me == nil {
		return nil
	}
	if me.ChargingMoveID != 0 || me.LockedMoveID != 0 || me.BideTurns > 0 ||
		(me.EncoreTurns != 0 && me.EncoreMoveID != 0) || me.MustRecharge || opp == nil {
		return chooseWildMoveOrCharge(me)
	}
	legal := make([]*BattleMove, 0, len(me.Moves))
	for i := range me.Moves {
		m := &me.Moves[i]
		if moveBlockedFor(me, m.ID) {
			continue
		}
		if m.PP == 0 {
			continue
		}
		legal = append(legal, m)
	}
	if len(legal) == 0 {
		return chooseWildMove(me)
	}
	type scoredMove struct {
		m *BattleMove
		s float64
	}
	scored := make([]scoredMove, 0, len(legal))
	for _, m := range legal {
		scored = append(scored, scoredMove{m: m, s: aiScoreMove(bs, me, opp, m, spec)})
	}
	sort.Slice(scored, func(i, j int) bool { return scored[i].s > scored[j].s })
	// L1: иногда ошибается — берёт 2-й/3-й по оценке ход, а не лучший.
	if spec.RandomMoveChance > 0 && mathrand.Intn(100) < spec.RandomMoveChance {
		n := len(scored)
		if n > 3 {
			n = 3
		}
		if n > 1 {
			// Исключаем лучший: это и есть «ошибка».
			return scored[1+mathrand.Intn(n-1)].m
		}
	}
	return scored[0].m
}

func aiScoreMove(bs *BattleState, me, opp *BattlePokemon, m *BattleMove, spec aiSpec) float64 {
	if m.CategoryID == 3 || m.Power <= 0 {
		v := aiStatusMoveValue(bs, me, opp, m, spec)
		if spec.SearchDepth >= 2 {
			pen := aiIncomingPenalty(bs, me, opp, m.Priority)
			// Сон/паралич/ожог/заморозка срезают следующий урон игрока.
			if m.EffectID >= 2 && m.EffectID <= 5 && opp.StatusID == 0 {
				pen *= 0.5
			}
			v -= pen
		}
		return v
	}
	dmg, eff := aiEstimateDamage(bs, me, opp, m, true, false, &bs.PlayerSide)
	if eff == 0 {
		return -1
	}
	hp := float64(max(1, opp.CurrentHP))
	frac := float64(dmg) / hp
	acc := float64(m.Accuracy)
	if m.NeverMisses || acc <= 0 {
		acc = 100
	}
	score := frac * 3.0 * (acc / 100.0)
	if dmg >= opp.CurrentHP {
		score += 3.0
	}
	if m.Priority > 0 {
		score += 0.2
	}
	if m.Drain > 0 && me.CurrentHP < me.MaxHP/2 {
		score += 0.5
	}
	if m.Drain < 0 && me.CurrentHP < me.MaxHP/3 {
		score -= 0.5
	}
	if eff > 1 {
		score += 0.2
	}
	if rechargeMoves[m.ID] && dmg < opp.CurrentHP {
		score -= 1.0
	}
	// Двухходовая атака: первый ход — зарядка, урон только на втором. Оценка
	// урона этого не видит, поэтому штрафуем отдельно.
	if _, isCharge := chargeMoves[m.ID]; isCharge && me.ChargingMoveID != m.ID {
		score *= 0.45
		if dmg >= opp.CurrentHP {
			score -= 1.0
		}
	}
	// Само-снижение (Перегрев/Лиственный шторм): не спамить, если не добиваем.
	if drop := aiSelfDropStages(m); drop > 0 && dmg < opp.CurrentHP {
		score -= float64(drop) * 0.2
	}
	if aiMoveNameHasTag(m.EngName, "self_destruct") {
		// Взрыв имеет смысл только как добивание; иначе — самоубийство.
		if dmg < opp.CurrentHP {
			return -10
		}
		score = 5.0
	}
	if spec.SearchDepth >= 2 {
		score -= aiIncomingPenalty(bs, me, opp, m.Priority)
		// Добивание снимает ответ.
		if dmg >= opp.CurrentHP {
			score += 2.0
		}
	}
	return score
}

// aiIncomingPenalty — ожидаемый ответ игрока (2-ply). Использует только
// открытые ходы; если игрок ещё не бил — синтетическую атаку primary-типа.
func aiIncomingPenalty(bs *BattleState, me, opp *BattlePokemon, priority int) float64 {
	oppBest, _ := aiBestDamageFrac(bs, opp, me, false, true, &bs.WildSide, true)
	if aiMovesFirst(bs, opp, me, 0, priority) {
		return 0.7 * min(oppBest, 1.5)
	}
	return 0.3 * min(oppBest, 1.5)
}

func aiSelfDropStages(m *BattleMove) int {
	total := 0
	for _, sc := range m.StatChanges {
		if sc.Target == 1 && sc.Stages < 0 {
			total += -sc.Stages
		}
	}
	return total
}

func aiStatusMoveValue(bs *BattleState, me, opp *BattlePokemon, m *BattleMove, spec aiSpec) float64 {
	_ = spec
	v := 0.0
	tags := aiMoveTags[strings.ToLower(strings.TrimSpace(m.EngName))]
	for _, tag := range tags {
		switch tag {
		case "hazard_set":
			if aiSideHasHazard(&bs.PlayerSide) {
				v -= 1.5
			} else if aiAliveBench(bs.PlayerTeam, bs.ActiveIdx) > 0 {
				v += 1.5
			} else {
				v += 0.4
			}
		case "hazard_clear":
			if aiSideHasHazard(&bs.WildSide) {
				v += 1.0
			} else {
				v -= 0.5
			}
		case "screen_set":
			if bs.WildSide.ReflectTurns > 0 || bs.WildSide.LightScreenTurns > 0 {
				v -= 1.0
			} else {
				v += 1.0
			}
		case "tailwind":
			if bs.WildSide.TailwindTurns > 0 {
				v -= 1.0
			} else if tailwindSpeed(bs, me) <= tailwindSpeed(bs, opp) {
				v += 1.0
			}
		case "trick_room":
			if bs.TrickRoomTurns > 0 {
				v -= 1.0
			} else if tailwindSpeed(bs, me) < tailwindSpeed(bs, opp) {
				v += 1.5
			}
		case "recovery":
			switch {
			case me.CurrentHP*2 < me.MaxHP:
				v += 2.0
			case me.CurrentHP*4 < me.MaxHP*3:
				v += 1.0
			default:
				v -= 0.5
			}
		case "setup":
			v += aiSetupValue(me, opp, m)
		case "status":
			if opp.StatusID == 0 {
				v += 1.5
			} else {
				v -= 2.0
			}
		case "protect":
			if me.ProtectPrimed {
				v -= 1.0
			} else {
				v += 0.3
			}
		case "rest":
			// Отдых без Сновидения/способности — ловушка: сон на 2–3 хода.
			v -= 1.5
		case "self_destruct":
			v -= 10.0
		case "recharge":
			v -= 2.0
		case "pivot":
			if aiBenchBetter(bs, me, opp) {
				v += 1.0
			}
		case "phaze":
			if aiEnemyHasBoosts(opp) || aiSideHasHazard(&bs.PlayerSide) {
				v += 1.5
			} else {
				v += 0.2
			}
		case "weather_set:rain", "weather_set:sun", "weather_set:hail", "weather_set:sand",
			"weather_benefit:rain", "weather_benefit:sun", "weather_benefit:hail", "weather_benefit:sand":
			// Погодные механики — отдельная задача, в v1 не поощряем.
		}
	}
	// Эффект-статус без тега (например, русское имя без eng_name).
	if m.EffectID >= 2 && m.EffectID <= 7 && opp.StatusID == 0 {
		v += 1.2
	} else if m.EffectID == 13 && !opp.Confused {
		v += 0.8
	}
	if m.HealPercent > 0 && me.CurrentHP*2 < me.MaxHP {
		v += float64(m.HealPercent) / 15.0
	}
	return v
}

func aiSetupValue(me, opp *BattlePokemon, m *BattleMove) float64 {
	best := 0.0
	for _, sc := range m.StatChanges {
		if sc.Target != 1 || sc.Stages <= 0 {
			continue
		}
		if sc.StatID < 0 || sc.StatID >= len(me.StatStages) {
			continue
		}
		if me.StatStages[sc.StatID] >= 3 {
			continue
		}
		// Ценность зависит от стата: атака важнее скорости/защиты.
		weight := 0.1
		switch sc.StatID {
		case 1, 4: // Атака / СпецАтака
			weight = 0.6
		case 2, 5: // Защита / СпецЗащита
			weight = 0.15
		case 3: // Скорость
			weight = 0.1
			if opp != nil && me.Speed >= opp.Speed {
				weight = 0.05
			}
		}
		val := float64(sc.Stages) * weight
		if me.CurrentHP*4 < me.MaxHP*3 {
			val *= 0.6
		}
		if val > best {
			best = val
		}
	}
	return best
}

func aiSideHasHazard(side *SideState) bool {
	if side == nil {
		return false
	}
	return side.StealthRock || side.SpikesLayers > 0 || side.ToxicSpikesLayers > 0 || side.StickyWeb
}

func aiAliveBench(team []BattlePokemon, activeIdx int) int {
	n := 0
	for i := range team {
		if i == activeIdx {
			continue
		}
		if !team[i].IsFainted && team[i].CurrentHP > 0 {
			n++
		}
	}
	return n
}

func aiEnemyHasBoosts(p *BattlePokemon) bool {
	for i := 1; i <= 5; i++ {
		if p.StatStages[i] > 0 {
			return true
		}
	}
	return false
}

func aiBenchBetter(bs *BattleState, me, opp *BattlePokemon) bool {
	cur := aiMatchupScore(bs, me, opp)
	for i := range bs.EnemyTeam {
		if i == bs.EnemyActiveIdx {
			continue
		}
		c := &bs.EnemyTeam[i]
		if c.IsFainted || c.CurrentHP <= 0 {
			continue
		}
		if aiMatchupScore(bs, c, opp) > cur+0.2 {
			return true
		}
	}
	return false
}

func chooseAISwitchWithSpec(bs *BattleState, me, opp *BattlePokemon, spec aiSpec) int {
	if me == nil || opp == nil {
		return -1
	}
	if me.ChargingMoveID != 0 || me.LockedMoveID != 0 || me.BideTurns > 0 || me.EncoreTurns > 0 {
		return -1
	}
	if me.Trapped || me.NoEscape {
		return -1
	}
	if me.Rooted && !pokemonHasType(me, typeGhost) {
		return -1
	}
	if len(bs.EnemyTeam) <= 1 {
		return -1
	}
	// Первый ход: лид выбран заранее, не меняем его «на реакцию».
	if bs.Turn <= 1 {
		return -1
	}

	cur := aiMatchupScore(bs, me, opp)
	curThreat, _ := aiBestDamageFrac(bs, opp, me, false, true, &bs.WildSide, true)
	// Если мы можем добить — бьём, а не меняемся.
	if ourKO, _ := aiBestDamageFrac(bs, me, opp, true, false, &bs.PlayerSide, false); ourKO >= 1.0 {
		return -1
	}
	// Если текущего всё равно снесут — выгоднее ударить и получить БЕСПЛАТНУЮ
	// замену после обморока, чем тратить ход на добровольный свитч.
	if curThreat >= 0.8 {
		return -1
	}
	// Не сбрасываем уже набранные бусты.
	for i := 1; i <= 5; i++ {
		if me.StatStages[i] > 0 {
			return -1
		}
	}
	// Анти-петля: не меняемся два хода подряд и не возвращаемся на только что
	// ушедшего, если только текущий не в смертельной опасности.
	recentSwitch := bs.AILastSwitchTurn > 0 && bs.Turn-bs.AILastSwitchTurn <= 1
	if recentSwitch && curThreat < 0.9 {
		return -1
	}
	// Текущий и так выигрывает расклад — незачем тратить ход на свитч.
	if cur >= 0.25 && curThreat < 0.9 {
		return -1
	}

	best := -1
	bestImprovement := 0.0
	bestScore := cur
	for i := range bs.EnemyTeam {
		if i == bs.EnemyActiveIdx {
			continue
		}
		if i == bs.AILastSwitchFrom && bs.AILastSwitchTurn > 0 && bs.Turn-bs.AILastSwitchTurn <= 2 {
			continue
		}
		c := &bs.EnemyTeam[i]
		if c.IsFainted || c.CurrentHP <= 0 {
			continue
		}
		our, _ := aiBestDamageFrac(bs, c, opp, true, false, &bs.PlayerSide, false)
		their, _ := aiBestDamageFrac(bs, opp, c, false, true, &bs.WildSide, true)
		// Входящий не должен умирать быстрее, чем успеет что-то сделать.
		if their >= 0.6 && our < 0.8 {
			continue
		}
		// Скамейка должна либо угрожать, либо держать удар.
		if our < 0.25 && their >= 0.4 {
			continue
		}
		hazard := aiEntryHazardFrac(bs, &bs.WildSide, c)
		if hazard >= 1.0 {
			continue
		}
		s := our - 1.25*their - hazard
		if curThreat >= 1.0 && their < 0.8 {
			s += 0.5
		}
		// Не меняемся на проигрышный расклад, если текущий не умирает.
		if s <= 0 && curThreat < 0.9 {
			continue
		}
		if imp := s - cur; imp > bestImprovement {
			bestImprovement = imp
			bestScore = s
			best = i
		}
	}
	if best < 0 {
		return -1
	}
	threshold := 0.45
	if spec.SearchDepth >= 2 {
		threshold = 0.35
	}
	if bestImprovement < threshold {
		return -1
	}
	// Текущий и так в плюсе, угроза не критична — не дёргаемся зря.
	if cur > 0.2 && curThreat < 0.8 && bestImprovement < threshold+0.2 {
		return -1
	}
	log.Printf("[ai-battle] L%d замена %s -> %s (cur=%.2f best=%.2f threat=%.2f)",
		bs.AILevel, me.Name, bs.EnemyTeam[best].Name, cur, bestScore, curThreat)
	return best
}

func aiMatchupScore(bs *BattleState, me, opp *BattlePokemon) float64 {
	our, _ := aiBestDamageFrac(bs, me, opp, true, false, &bs.PlayerSide, false)
	their, _ := aiBestDamageFrac(bs, opp, me, false, true, &bs.WildSide, true)
	return our - their
}

// aiBestDamageFrac — лучший оценочный урон как доля MaxHP цели.
// attackerKnown/defenderKnown управляют «честностью»: неизвестной стороне
// обнуляем способность/предмет. revealedOnly — брать только открытые ходы.
func aiBestDamageFrac(bs *BattleState, attacker, defender *BattlePokemon, attackerKnown, defenderKnown bool, defSide *SideState, revealedOnly bool) (float64, int) {
	if attacker == nil || defender == nil || defender.MaxHP <= 0 {
		return 0, 0
	}
	moves := attacker.Moves
	if revealedOnly {
		moves = aiRevealedMoves(attacker)
	}
	best := 0.0
	bestID := 0
	for i := range moves {
		m := &moves[i]
		if m.CategoryID == 3 || m.Power <= 0 {
			continue
		}
		dmg, eff := aiEstimateDamage(bs, attacker, defender, m, attackerKnown, defenderKnown, defSide)
		if eff == 0 {
			continue
		}
		frac := float64(dmg) / float64(defender.MaxHP)
		if frac > best {
			best, bestID = frac, m.ID
		}
	}
	return best, bestID
}

// aiEstimateDamage — детерминированная оценка урона через реальную формулу.
// Способности/предметы неизвестной стороны обнуляются (честная информация).
func aiEstimateDamage(bs *BattleState, attacker, defender *BattlePokemon, move *BattleMove, attackerKnown, defenderKnown bool, defSide *SideState) (int, float64) {
	if attacker == nil || defender == nil || move == nil || move.CategoryID == 3 {
		return 0, 1
	}
	a := *attacker
	d := *defender
	if !attackerKnown {
		a.AbilityID, a.AbilityName, a.AbilityKey = 0, "", ""
		a.HeldItemID, a.HeldItemTypeID, a.HeldItemEffect, a.HeldAtkMult = 0, 0, "", 1.0
		a.FlashFireBoosted = false
	}
	if !defenderKnown {
		d.AbilityID, d.AbilityName, d.AbilityKey = 0, "", ""
		d.HeldItemID, d.HeldItemTypeID, d.HeldItemEffect, d.HeldAtkMult = 0, 0, "", 1.0
		d.FlashFireBoosted = false
	}
	var discard []BattleLogEntry
	dmg, _, eff := calculateDamageCore(bs, &a, &d, move, defSide, bs.Turn, &discard, true)
	return dmg, eff
}

// aiRevealedMoves — только ходы, которые игрок уже применял. Если открытых
// урон-ходов нет, добавляем синтетическую «неизвестную атаку» primary-типа,
// иначе бот считал бы игрока безобидным.
func aiRevealedMoves(p *BattlePokemon) []BattleMove {
	if p == nil {
		return nil
	}
	synthetic := func() []BattleMove {
		cat := 1
		if p.SpecialAttack > p.Attack {
			cat = 2
		}
		t := p.PrimaryTypeID
		if t == 0 {
			t = 1
		}
		return []BattleMove{{ID: -1, Name: "неизвестная атака", TypeID: t, CategoryID: cat, Power: 80, Accuracy: 100, PP: -1}}
	}
	if len(p.UsedMoveIDs) == 0 {
		return synthetic()
	}
	used := map[int]bool{}
	for _, id := range p.UsedMoveIDs {
		used[id] = true
	}
	var out []BattleMove
	hasDamage := false
	for _, m := range p.Moves {
		if !used[m.ID] {
			continue
		}
		out = append(out, m)
		if m.CategoryID != 3 && m.Power > 0 {
			hasDamage = true
		}
	}
	if !hasDamage {
		out = append(out, synthetic()...)
	}
	return out
}

func aiMovesFirst(bs *BattleState, a, b *BattlePokemon, aPriority, bPriority int) bool {
	if aPriority != bPriority {
		return aPriority > bPriority
	}
	as := tailwindSpeed(bs, a)
	bsSpeed := tailwindSpeed(bs, b)
	if bs.TrickRoomTurns > 0 {
		return as < bsSpeed
	}
	if as == bsSpeed {
		return true
	}
	return as > bsSpeed
}

func aiEntryHazardFrac(bs *BattleState, side *SideState, p *BattlePokemon) float64 {
	if side == nil || p == nil {
		return 0
	}
	frac := 0.0
	if side.StealthRock {
		frac += 0.125 * getTypeEffectiveness(typeRock, getPokemonTypes(p))
	}
	grounded := !pokemonHasType(p, typeFlying) && !abilityHovering(p, bs)
	if grounded {
		switch side.SpikesLayers {
		case 1:
			frac += 1.0 / 8
		case 2:
			frac += 1.0 / 6
		case 3:
			frac += 1.0 / 4
		}
		if side.StickyWeb {
			frac += 0.05
		}
		if side.ToxicSpikesLayers > 0 && !pokemonHasType(p, typePoison) && !pokemonHasType(p, typeSteel) {
			frac += 0.05
		}
	}
	return frac
}
