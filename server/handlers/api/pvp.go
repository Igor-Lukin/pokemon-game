package api

// PvP-бой между двумя игроками (поток [17], часть B). Ключевая идея (см.
// HANDOFF_pvp_battle §2): PvP ≈ тренерский бой, где «враг» — человек. НЕ пишем
// второй движок: сторона A = PlayerTeam/ActiveIdx, сторона B = EnemyTeam/
// EnemyActiveIdx + WildPokemon (указатель на активного B, «указательный трюк»
// из потока [11]B). pvpResolveTurn — оркестратор поверх ТЕХ ЖЕ примитивов, что
// processTurn (executeDamagingMove/applyEffectByID/applyStatChanges/…).
// Транспорт — HTTP-поллинг; модель серверная (позже можно ws-пуш без смены форм).

import (
	"encoding/json"
	"fmt"
	"log"
	mathrand "math/rand"
	"net/http"
	"pokemon-game/server/db"
	"strconv"
	"sync"
	"time"
)

// Тайминги уборки эфемерных PvP-сессий.
const (
	pvpTurnTimeout = 3 * time.Minute // нет действий -> бой прерван (ушёл игрок)
	pvpKeepAfter   = 2 * time.Minute // держать finished-сессию, чтобы клиенты увидели итог
)

// pvpAction — сабмит действия игрока на текущий ход.
type pvpAction struct {
	Type          string // move | switch
	AttackID      int
	UserPokemonID int
}

// PvpBattle — обвязка вокруг BattleState для боя двух людей.
type PvpBattle struct {
	ID string
	BS *BattleState // A=PlayerTeam..., B=EnemyTeam...(указатель WildPokemon)

	AUserID, BUserID int
	AName, BName     string

	Phase string // lead_select | battling | await_replace | finished
	ALead bool
	BLead bool

	AAction, BAction *pvpAction

	AwaitReplaceA, AwaitReplaceB bool
	WinnerUserID                 int // 0 = ничья/прервано

	Log          []BattleLogEntry
	LastActivity time.Time
}

var (
	pvpSessions = map[string]*PvpBattle{}
	pvpMu       sync.Mutex
)

func init() {
	go cleanupPvpSessions()
}

func cleanupPvpSessions() {
	for {
		time.Sleep(time.Minute)
		pvpMu.Lock()
		now := time.Now()
		for id, p := range pvpSessions {
			if p.Phase != "finished" && now.Sub(p.LastActivity) > pvpTurnTimeout {
				// Долго нет действий -> считаем бой прерванным (кто-то ушёл).
				p.Phase = "finished"
				p.WinnerUserID = 0
				savePvpState(p.BS)
				p.LastActivity = now
			} else if p.Phase == "finished" && now.Sub(p.LastActivity) > pvpKeepAfter {
				delete(pvpSessions, id)
			}
		}
		pvpMu.Unlock()
	}
}

// ======================== Старт (наполнение заглушки из части A) ========================

// startPvpBattle создаёт PvP-сессию на «принять вызов» (online.go
// ChallengeRespondHandler). Заменяет заглушку из части A.
func startPvpBattle(challengerID, targetID int) {
	teamA, _, errA := buildPlayerTeam(challengerID)
	teamB, _, errB := buildPlayerTeam(targetID)
	if errA != nil || errB != nil {
		log.Printf("[PvP] старт отменён: A(%d)=%v B(%d)=%v", challengerID, errA, targetID, errB)
		return
	}

	var aName, bName string
	db.DB.QueryRow("SELECT username FROM users WHERE id = ?", challengerID).Scan(&aName)
	db.DB.QueryRow("SELECT username FROM users WHERE id = ?", targetID).Scan(&bName)

	bs := &BattleState{
		ID:             newBattleID(),
		PlayerTeam:     teamA,
		ActiveIdx:      -1, // до выбора лида
		EnemyTeam:      teamB,
		EnemyActiveIdx: -1,
		WildPokemon:    nil, // ставим при выборе лида B
		Turn:           0,
		StartedAt:      time.Now(),
		BattleType:     "pvp",
		PlayerName:     aName, // имя стороны A (для логов смены A)
		TrainerName:    bName, // имя стороны B (для логов смены B)
	}

	pvp := &PvpBattle{
		ID:           bs.ID,
		BS:           bs,
		AUserID:      challengerID,
		BUserID:      targetID,
		AName:        aName,
		BName:        bName,
		Phase:        "lead_select",
		LastActivity: time.Now(),
	}

	pvpMu.Lock()
	pvpSessions[bs.ID] = pvp
	pvpMu.Unlock()
	log.Printf("[PvP] бой %s начат: A=%d(%s) vs B=%d(%s)", bs.ID, challengerID, aName, targetID, bName)
}

// ======================== Хелперы ========================

func pvpSideOf(p *PvpBattle, userID int) string {
	if userID == p.AUserID {
		return "A"
	}
	if userID == p.BUserID {
		return "B"
	}
	return ""
}

func teamHasAlive(team []BattlePokemon) bool {
	for i := range team {
		if !team[i].IsFainted && team[i].CurrentHP > 0 {
			return true
		}
	}
	return false
}

// pvpMyActive/pvpOppActive — активные покемоны с точки зрения стороны (nil до
// выбора лида).
func pvpMyActive(bs *BattleState, side string) *BattlePokemon {
	if side == "A" {
		if bs.ActiveIdx < 0 {
			return nil
		}
		return &bs.PlayerTeam[bs.ActiveIdx]
	}
	if bs.EnemyActiveIdx < 0 {
		return nil
	}
	return bs.WildPokemon
}

func pvpOppActive(bs *BattleState, side string) *BattlePokemon {
	if side == "A" {
		if bs.EnemyActiveIdx < 0 {
			return nil
		}
		return bs.WildPokemon
	}
	if bs.ActiveIdx < 0 {
		return nil
	}
	return &bs.PlayerTeam[bs.ActiveIdx]
}

func pvpMyTeam(bs *BattleState, side string) []BattlePokemon {
	if side == "A" {
		return bs.PlayerTeam
	}
	return bs.EnemyTeam
}

// savePvpState пишет HP/статус/PP ОБЕИХ команд в user_pokemons (урон сохраняется,
// решение владельца). Зеркало saveBattleState на две команды.
func savePvpState(bs *BattleState) {
	savePvpTeam(bs.PlayerTeam)
	savePvpTeam(bs.EnemyTeam)
}

func savePvpTeam(team []BattlePokemon) {
	for _, p := range team {
		statusID := p.StatusID
		if statusID < 1 || statusID > 7 {
			statusID = 1
		}
		db.DB.Exec("UPDATE user_pokemons SET current_hp = ?, status_id = ? WHERE id = ?",
			p.CurrentHP, statusID, p.ID)
		for _, m := range p.Moves {
			if m.MaxPP <= 0 {
				continue
			}
			db.DB.Exec(`
				UPDATE user_pokemon_learnable_attacks
				SET current_pp = ?
				WHERE user_pokemon_id = ? AND attack_id = ? AND is_learned = 1
			`, m.PP, p.ID, m.ID)
		}
	}
}

// pvpSwitchB — смена активного стороны B (EnemyTeam). Зеркало
// performInBattleSwitch (сторона A), но по EnemyTeam/EnemyActiveIdx/WildSide +
// поддержание указателя WildPokemon. ОСТАТКИ: список сбрасываемых при уходе
// волатилей дублирует performInBattleSwitch — держать синхронно.
func pvpSwitchB(bs *BattleState, userPokemonID int, log *[]BattleLogEntry) bool {
	newIdx := -1
	for i := range bs.EnemyTeam {
		if bs.EnemyTeam[i].ID == userPokemonID && !bs.EnemyTeam[i].IsFainted {
			newIdx = i
			break
		}
	}
	if newIdx < 0 || newIdx == bs.EnemyActiveIdx {
		return false
	}

	old := &bs.EnemyTeam[bs.EnemyActiveIdx]
	old.ChargingMoveID = 0
	old.Invulnerable = false
	old.LockedMoveID = 0
	old.LockTurns = 0
	old.Trapped = false
	old.TrapTurns = 0
	old.NoEscape = false
	old.Infatuated = false
	old.Rooted = false
	old.AquaRing = false
	old.WishPending = false
	old.BattleTypes = nil
	old.HealBlockTurns = 0
	old.SureHit = false
	old.SureCrit = false
	old.ChargedElectric = false
	old.Foresighted = false
	old.MiracleEyed = false
	old.DestinyBond = false
	old.Grudge = false
	old.Electrified = false
	old.MagnetRiseTurns = 0
	old.LevitatedTurns = 0
	old.ActiveTurns = 0
	old.BideTurns, old.BideDamage = 0, 0
	old.YawnTurns = 0
	restoreTransform(old)
	restoreBaseStats(old)
	resetBlockState(old)

	bs.EnemyActiveIdx = newIdx
	bs.WildPokemon = &bs.EnemyTeam[newIdx]
	active := bs.WildPokemon
	*log = append(*log, BattleLogEntry{
		Round:        bs.Turn,
		PokemonName:  active.Name,
		PokemonImage: active.ImageURL,
		Message:      bs.TrainerName + ": " + active.Name + " вступает в бой!",
	})

	applyHealingWish(&bs.WildSide, active, bs.Turn, log)
	if applyEntryHazards(bs, active, &bs.WildSide, bs.Turn, log) {
		*log = append(*log, BattleLogEntry{Round: bs.Turn, PokemonName: active.Name, Fainted: true})
	}
	return true
}

// pvpForceMove — форс-состояния мувера (зарядка/лок/выжидание/овации), зеркало
// player-логики processTurn, применённое к ЛЮБОЙ стороне. Возвращает
// (реальный ход, wasCharging — не списывать PP, обновлённый canAct).
func pvpForceMove(p *BattlePokemon, chosen *BattleMove, canAct bool, turn int, log *[]BattleLogEntry) (*BattleMove, bool, bool) {
	move := chosen
	wasCharging := false

	if !canAct && p.StatusID == 5 && move != nil && sleepActingMove(move.ID) {
		canAct = true
		*log = append(*log, BattleLogEntry{
			Round: turn, PokemonName: p.Name, PokemonImage: p.ImageURL,
			Message: p.Name + " действует во сне!",
		})
	}

	if p.ChargingMoveID != 0 {
		if canAct {
			if m := findMoveByID(p, p.ChargingMoveID); m != nil {
				move = m
				wasCharging = true
			}
		} else {
			p.ChargingMoveID = 0
			p.Invulnerable = false
		}
	}
	if p.LockedMoveID != 0 {
		if canAct {
			if m := findMoveByID(p, p.LockedMoveID); m != nil {
				move = m
				wasCharging = true
			}
		} else {
			p.LockedMoveID = 0
			p.LockTurns = 0
		}
	}
	if p.BideTurns > 0 {
		if canAct {
			if m := findMoveByID(p, moveBide); m != nil {
				move = m
				wasCharging = true
			}
		} else {
			p.BideTurns, p.BideDamage = 0, 0
		}
	}
	if p.EncoreTurns > 0 && p.EncoreMoveID != 0 && canAct && !wasCharging {
		if m := findMoveByID(p, p.EncoreMoveID); m != nil {
			move = m
		}
	}
	return move, wasCharging, canAct
}

// pvpExecuteMove — исполнить один ход через ТЕ ЖЕ примитивы, что processTurn.
func pvpExecuteMove(bs *BattleState, atk, def *BattlePokemon, move *BattleMove, log *[]BattleLogEntry) {
	if move == nil {
		return
	}
	markMoveUsed(atk, move.ID)
	if move.CategoryID != 3 && move.Power > 0 {
		fainted, wc := executeDamagingMove(bs, atk, def, move, bs.BattleWeatherID, bs.Turn, log, sideFor(bs, def), sideFor(bs, atk), bs.WaterSportTurns > 0)
		if wc > 0 {
			setBattleWeather(bs, wc, log)
		}
		if fainted {
			*log = append(*log, BattleLogEntry{Round: bs.Turn, PokemonName: def.Name, Fainted: true})
		}
	} else {
		*log = append(*log, makeEntry(bs.Turn, atk, move, 0, false, 1, false))
		if move.CategoryID == 3 {
			if wc := applyEffectByID(bs, atk, def, move, bs.Turn, log, sideFor(bs, def)); wc > 0 {
				setBattleWeather(bs, wc, log)
			}
			applyStatChanges(atk, def, move, bs.BattleWeatherID, bs.Turn, log, sideFor(bs, def))
			applyHeal(atk, def, move, bs.BattleWeatherID, bs.Turn, log)
			handleCustomStatusMove(bs, atk, def, move, bs.Turn, log)
			applySideMove(bs, atk, def, move, log)
			handlePhazeMove(bs, atk, def, move, log)
			handleCallMove(bs, atk, def, move, bs.Turn, log)
		}
	}
}

// pvpResolveTurn — разрешение хода, зеркало processTurn для ДВУХ людей. Вызывать
// РОВНО когда сдали ОБА действия, под pvpMu.
func pvpResolveTurn(pvp *PvpBattle) {
	bs := pvp.BS
	aAct, bAct := pvp.AAction, pvp.BAction
	if aAct == nil || bAct == nil {
		return
	}
	bs.Turn++
	logv := []BattleLogEntry{}

	aSwitched := aAct.Type == "switch"
	bSwitched := bAct.Type == "switch"

	// (6.2) Смены — первыми (внутри применяются ловушки входа).
	if aSwitched {
		performInBattleSwitch(bs, aAct.UserPokemonID, &logv)
	}
	if bSwitched {
		pvpSwitchB(bs, bAct.UserPokemonID, &logv)
	}

	a := &bs.PlayerTeam[bs.ActiveIdx]
	b := bs.WildPokemon

	// Счётчик ходов на поле (у сменившихся сброшен в 0 switch-хелпером).
	if !aSwitched {
		a.ActiveTurns++
	}
	if !bSwitched {
		b.ActiveTurns++
	}
	a.DamageTakenPhys, a.DamageTakenSpec = 0, 0
	b.DamageTakenPhys, b.DamageTakenSpec = 0, 0

	releaseStaleSkyHold(a, b)

	// Кто бьёт атакой (не сменился и жив после входа).
	aActs := aAct.Type == "move" && !a.IsFainted
	bActs := bAct.Type == "move" && !b.IsFainted

	// (6.1) Pre-turn статус только для атакующих.
	aCanAct := false
	bCanAct := false
	if aActs {
		aCanAct = processPreTurnStatus(a, &logv)
	}
	if bActs {
		bCanAct = processPreTurnStatus(b, &logv)
	}

	// Выбранные ходы + форс-состояния (зарядка/лок/выжидание/овации) для обеих сторон.
	var aMove, bMove *BattleMove
	aWasCharging, bWasCharging := false, false
	if aActs {
		aMove = findMoveByID(a, aAct.AttackID)
		aMove, aWasCharging, aCanAct = pvpForceMove(a, aMove, aCanAct, bs.Turn, &logv)
	}
	if bActs {
		bMove = findMoveByID(b, bAct.AttackID)
		bMove, bWasCharging, bCanAct = pvpForceMove(b, bMove, bCanAct, bs.Turn, &logv)
	}

	// По-ходовые volatile (семена-пиявки/ловушка) — как в processTurn, для обоих.
	runEndTurnVolatiles(a, b, &logv)
	if a.IsFainted {
		aCanAct = false
		logv = append(logv, BattleLogEntry{Round: bs.Turn, PokemonName: a.Name, Fainted: true})
	}
	if b.IsFainted {
		bCanAct = false
		logv = append(logv, BattleLogEntry{Round: bs.Turn, PokemonName: b.Name, Fainted: true})
	}

	aCan := aCanAct && aMove != nil
	bCan := bCanAct && bMove != nil

	// MoveFailed из-за статуса (парализация/сон/заморозка) — как в processTurn.
	if aActs && !aCanAct && !a.IsFainted && aMove != nil {
		if reason := statusSkipReason(a.StatusID); reason != "" {
			logv = append(logv, BattleLogEntry{Round: bs.Turn, PokemonName: a.Name, PokemonImage: a.ImageURL, MoveName: aMove.Name, MoveFailed: reason})
		}
	}
	if bActs && !bCanAct && !b.IsFainted && bMove != nil {
		if reason := statusSkipReason(b.StatusID); reason != "" {
			logv = append(logv, BattleLogEntry{Round: bs.Turn, PokemonName: b.Name, PokemonImage: b.ImageURL, MoveName: bMove.Name, MoveFailed: reason})
		}
	}

	// (6.3) Порядок: приоритет, при равенстве — скорость (учёт Trick Room).
	aFirst := true
	if aCan && bCan {
		if aMove.Priority != bMove.Priority {
			aFirst = aMove.Priority > bMove.Priority
		} else {
			aSpd := tailwindSpeed(bs, a)
			bSpd := tailwindSpeed(bs, b)
			aFirst = aSpd > bSpd
			if bs.TrickRoomTurns > 0 {
				aFirst = aSpd < bSpd
			}
			if aSpd == bSpd {
				aFirst = mathrand.Intn(2) == 0
			}
		}
	}

	var first, second *BattlePokemon
	var firstMove, secondMove *BattleMove
	if aCan && bCan {
		if aFirst {
			first, firstMove, second, secondMove = a, aMove, b, bMove
		} else {
			first, firstMove, second, secondMove = b, bMove, a, aMove
		}
	} else if aCan {
		first, firstMove = a, aMove
	} else if bCan {
		first, firstMove = b, bMove
	}

	// (6.4) Исполнение. Защищающийся = другой активный.
	defOf := func(atk *BattlePokemon) *BattlePokemon {
		if atk == a {
			return b
		}
		return a
	}
	if first != nil && firstMove != nil && !first.IsFainted && !defOf(first).IsFainted {
		pvpExecuteMove(bs, first, defOf(first), firstMove, &logv)
	}
	if second != nil && secondMove != nil && !second.IsFainted && !first.IsFainted {
		pvpExecuteMove(bs, second, defOf(second), secondMove, &logv)
	}

	// Списание PP (ход-удар двухходовки уже списан при зарядке — wasCharging).
	if aCan && aMove != nil && !aWasCharging && aMove.PP > 0 {
		aMove.PP--
	}
	if bCan && bMove != nil && !bWasCharging && bMove.PP > 0 {
		bMove.PP--
	}

	// (6.5) Конец хода — те же хелперы, что MoveHandler после processTurn.
	applyWeatherChip(bs, a, b, &logv)
	applyFutureAttacks(bs, a, b, &logv)
	postProcessWeather(bs, &logv)
	tickSideTimers(bs, &logv)
	abilityOnEndTurn(bs, a, bs.Turn, &logv)
	abilityOnEndTurn(bs, b, bs.Turn, &logv)

	// (6.6) Обмороки/победа/замена.
	pvp.AAction, pvp.BAction = nil, nil
	aDown := a.IsFainted
	bDown := b.IsFainted
	aAlive := teamHasAlive(bs.PlayerTeam)
	bAlive := teamHasAlive(bs.EnemyTeam)

	aLost := aDown && !aAlive
	bLost := bDown && !bAlive

	if aLost || bLost {
		pvp.Phase = "finished"
		switch {
		case aLost && bLost:
			pvp.WinnerUserID = 0 // ничья
		case aLost:
			pvp.WinnerUserID = pvp.BUserID
		default:
			pvp.WinnerUserID = pvp.AUserID
		}
	} else {
		pvp.AwaitReplaceA = aDown && aAlive
		pvp.AwaitReplaceB = bDown && bAlive
		if pvp.AwaitReplaceA || pvp.AwaitReplaceB {
			pvp.Phase = "await_replace"
		} else {
			pvp.Phase = "battling"
		}
	}

	pvp.Log = append(pvp.Log, logv...)
	pvp.LastActivity = time.Now()
	savePvpState(bs) // (7) персист по ходу — защита от потери прогресса при обрыве
}

// ======================== Эндпоинты ========================

func pvpFind(w http.ResponseWriter, battleID string) (*PvpBattle, bool) {
	pvp, ok := pvpSessions[battleID]
	if !ok {
		http.Error(w, "Бой не найден", http.StatusNotFound)
		return nil, false
	}
	return pvp, true
}

// GET /api/pvp/current — активный PvP-бой для меня.
func PvpCurrentHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := onlineUserID(w, r)
	if !ok {
		return
	}
	pvpMu.Lock()
	var found *PvpBattle
	for _, p := range pvpSessions {
		if (p.AUserID == uid || p.BUserID == uid) && p.Phase != "finished" {
			found = p
			break
		}
	}
	var resp map[string]interface{}
	if found == nil {
		resp = map[string]interface{}{"none": true}
	} else {
		resp = map[string]interface{}{"battle_id": found.ID, "phase": found.Phase, "my_side": pvpSideOf(found, uid)}
	}
	pvpMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

type pvpStateResp struct {
	Phase              string           `json:"phase"`
	MySide             string           `json:"my_side"`
	MyTeam             []BattlePokemon  `json:"my_team,omitempty"`
	MyActive           *BattlePokemon   `json:"my_active,omitempty"`
	OppActive          *BattlePokemon   `json:"opp_active,omitempty"`
	ISubmitted         bool             `json:"i_submitted"`
	WaitingForOpponent bool             `json:"waiting_for_opponent"`
	AwaitMyReplace     bool             `json:"await_my_replace"`
	NewLog             []BattleLogEntry `json:"new_log"`
	LogSeq             int              `json:"log_seq"`
	Finished           bool             `json:"finished"`
	IWon               bool             `json:"i_won"`
	Aborted            bool             `json:"aborted,omitempty"`
	WeatherID          int              `json:"weather_id,omitempty"`
	WeatherName        string           `json:"weather_name,omitempty"`
	WeatherTurns       int              `json:"weather_turns,omitempty"`
	MyName             string           `json:"my_name,omitempty"`
	OppName            string           `json:"opp_name,omitempty"`
}

// GET /api/pvp/state?battle_id=..&since=.. — perspective-состояние для меня.
func PvpStateHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := onlineUserID(w, r)
	if !ok {
		return
	}
	battleID := r.URL.Query().Get("battle_id")
	since, _ := strconv.Atoi(r.URL.Query().Get("since"))

	pvpMu.Lock()
	pvp, ok := pvpFind(w, battleID)
	if !ok {
		pvpMu.Unlock()
		return
	}
	side := pvpSideOf(pvp, uid)
	if side == "" {
		pvpMu.Unlock()
		http.Error(w, "Вы не участник боя", http.StatusForbidden)
		return
	}
	bs := pvp.BS

	resp := pvpStateResp{
		Phase:        pvp.Phase,
		MySide:       side,
		WeatherID:    bs.BattleWeatherID,
		WeatherName:  bs.BattleWeatherName,
		WeatherTurns: bs.WeatherTurns,
	}
	if side == "A" {
		resp.MyName, resp.OppName = pvp.AName, pvp.BName
	} else {
		resp.MyName, resp.OppName = pvp.BName, pvp.AName
	}

	// Моя команда (для выбора лида/замены) — копии.
	for i := range pvpMyTeam(bs, side) {
		resp.MyTeam = append(resp.MyTeam, *copyBattlePokemon(&pvpMyTeam(bs, side)[i]))
	}
	if mine := pvpMyActive(bs, side); mine != nil {
		resp.MyActive = copyBattlePokemon(mine)
	}
	if opp := pvpOppActive(bs, side); opp != nil {
		oc := copyBattlePokemon(opp)
		oc.Moves = nil // не палим ходы соперника
		resp.OppActive = oc
	}

	var myAction *pvpAction
	if side == "A" {
		myAction = pvp.AAction
		resp.AwaitMyReplace = pvp.AwaitReplaceA
	} else {
		myAction = pvp.BAction
		resp.AwaitMyReplace = pvp.AwaitReplaceB
	}
	resp.ISubmitted = myAction != nil
	resp.WaitingForOpponent = myAction != nil // сдал я, ход ещё не разрешён => ждём

	resp.Finished = pvp.Phase == "finished"
	if resp.Finished {
		resp.IWon = pvp.WinnerUserID == uid
		resp.Aborted = pvp.WinnerUserID == 0
	}

	// Новые строки лога по курсору клиента.
	if since < 0 {
		since = 0
	}
	if since > len(pvp.Log) {
		since = len(pvp.Log)
	}
	resp.NewLog = append([]BattleLogEntry{}, pvp.Log[since:]...)
	resp.LogSeq = len(pvp.Log)
	pvpMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

type pvpLeadRequest struct {
	BattleID      string `json:"battle_id"`
	UserPokemonID int    `json:"user_pokemon_id"`
}

// POST /api/pvp/lead {battle_id, user_pokemon_id} — выбрать лида.
func PvpLeadHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := onlineUserID(w, r)
	if !ok {
		return
	}
	var req pvpLeadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Некорректный запрос", http.StatusBadRequest)
		return
	}

	pvpMu.Lock()
	defer pvpMu.Unlock()
	pvp, ok := pvpFind(w, req.BattleID)
	if !ok {
		return
	}
	side := pvpSideOf(pvp, uid)
	if side == "" {
		http.Error(w, "Вы не участник боя", http.StatusForbidden)
		return
	}
	if pvp.Phase != "lead_select" {
		http.Error(w, "Лид уже выбран", http.StatusBadRequest)
		return
	}
	bs := pvp.BS

	if side == "A" {
		if pvp.ALead {
			http.Error(w, "Вы уже выбрали", http.StatusBadRequest)
			return
		}
		idx := pvpTeamIndexAlive(bs.PlayerTeam, req.UserPokemonID)
		if idx < 0 {
			http.Error(w, "Покемон недоступен", http.StatusBadRequest)
			return
		}
		bs.ActiveIdx = idx
		pvp.ALead = true
	} else {
		if pvp.BLead {
			http.Error(w, "Вы уже выбрали", http.StatusBadRequest)
			return
		}
		idx := pvpTeamIndexAlive(bs.EnemyTeam, req.UserPokemonID)
		if idx < 0 {
			http.Error(w, "Покемон недоступен", http.StatusBadRequest)
			return
		}
		bs.EnemyActiveIdx = idx
		bs.WildPokemon = &bs.EnemyTeam[idx]
		pvp.BLead = true
	}

	if pvp.ALead && pvp.BLead {
		pvp.Phase = "battling"
		// [ability hook] способности лидов при старте боя (лиды мимо ловушек).
		abilityOnSwitchIn(bs, &bs.PlayerTeam[bs.ActiveIdx], &pvp.Log)
		abilityOnSwitchIn(bs, bs.WildPokemon, &pvp.Log)
	}
	pvp.LastActivity = time.Now()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func pvpTeamIndexAlive(team []BattlePokemon, userPokemonID int) int {
	for i := range team {
		if team[i].ID == userPokemonID && !team[i].IsFainted && team[i].CurrentHP > 0 {
			return i
		}
	}
	return -1
}

type pvpActionRequest struct {
	BattleID      string `json:"battle_id"`
	Type          string `json:"type"` // move | switch
	AttackID      int    `json:"attack_id"`
	UserPokemonID int    `json:"user_pokemon_id"`
}

// POST /api/pvp/action — сабмит действия на текущий ход.
func PvpActionHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := onlineUserID(w, r)
	if !ok {
		return
	}
	var req pvpActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Некорректный запрос", http.StatusBadRequest)
		return
	}

	pvpMu.Lock()
	defer pvpMu.Unlock()
	pvp, ok := pvpFind(w, req.BattleID)
	if !ok {
		return
	}
	side := pvpSideOf(pvp, uid)
	if side == "" {
		http.Error(w, "Вы не участник боя", http.StatusForbidden)
		return
	}
	if pvp.Phase != "battling" {
		http.Error(w, "Сейчас нельзя ходить", http.StatusBadRequest)
		return
	}
	bs := pvp.BS

	// Уже сдал действие?
	if (side == "A" && pvp.AAction != nil) || (side == "B" && pvp.BAction != nil) {
		http.Error(w, "Ход уже выбран", http.StatusBadRequest)
		return
	}

	mine := pvpMyActive(bs, side)
	if mine == nil {
		http.Error(w, "Нет активного покемона", http.StatusBadRequest)
		return
	}

	var act *pvpAction
	switch req.Type {
	case "move":
		var mv *BattleMove
		for i := range mine.Moves {
			if mine.Moves[i].ID == req.AttackID {
				mv = &mine.Moves[i]
				break
			}
		}
		if mv == nil {
			http.Error(w, "Атака не найдена", http.StatusBadRequest)
			return
		}
		if mv.PP == 0 {
			http.Error(w, "Нет PP", http.StatusBadRequest)
			return
		}
		if moveBlockedFor(mine, mv.ID) {
			http.Error(w, "Этот ход сейчас недоступен", http.StatusBadRequest)
			return
		}
		act = &pvpAction{Type: "move", AttackID: req.AttackID}
	case "switch":
		if pvpTeamIndexAlive(pvpMyTeam(bs, side), req.UserPokemonID) < 0 {
			http.Error(w, "Покемон недоступен", http.StatusBadRequest)
			return
		}
		if mine.ID == req.UserPokemonID {
			http.Error(w, "Этот покемон уже в бою", http.StatusBadRequest)
			return
		}
		act = &pvpAction{Type: "switch", UserPokemonID: req.UserPokemonID}
	default:
		http.Error(w, "Неизвестное действие", http.StatusBadRequest)
		return
	}

	if side == "A" {
		pvp.AAction = act
	} else {
		pvp.BAction = act
	}
	pvp.LastActivity = time.Now()

	// Оба сдали -> разрешаем ход атомарно (под pvpMu).
	if pvp.AAction != nil && pvp.BAction != nil {
		pvpResolveTurn(pvp)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

type pvpReplaceRequest struct {
	BattleID      string `json:"battle_id"`
	UserPokemonID int    `json:"user_pokemon_id"`
}

// POST /api/pvp/replace — выбрать замену павшему активному.
func PvpReplaceHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := onlineUserID(w, r)
	if !ok {
		return
	}
	var req pvpReplaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Некорректный запрос", http.StatusBadRequest)
		return
	}

	pvpMu.Lock()
	defer pvpMu.Unlock()
	pvp, ok := pvpFind(w, req.BattleID)
	if !ok {
		return
	}
	side := pvpSideOf(pvp, uid)
	if side == "" {
		http.Error(w, "Вы не участник боя", http.StatusForbidden)
		return
	}
	if pvp.Phase != "await_replace" {
		http.Error(w, "Сейчас не нужна замена", http.StatusBadRequest)
		return
	}
	awaitMine := (side == "A" && pvp.AwaitReplaceA) || (side == "B" && pvp.AwaitReplaceB)
	if !awaitMine {
		http.Error(w, "Вы не выбираете замену", http.StatusBadRequest)
		return
	}
	bs := pvp.BS
	if pvpTeamIndexAlive(pvpMyTeam(bs, side), req.UserPokemonID) < 0 {
		http.Error(w, "Покемон недоступен", http.StatusBadRequest)
		return
	}

	logv := []BattleLogEntry{}
	if side == "A" {
		performInBattleSwitch(bs, req.UserPokemonID, &logv)
	} else {
		pvpSwitchB(bs, req.UserPokemonID, &logv)
	}

	// Вошедший мог пасть от ловушек.
	mine := pvpMyActive(bs, side)
	if mine != nil && mine.IsFainted {
		if teamHasAlive(pvpMyTeam(bs, side)) {
			// остаёмся в await_replace (нужна ещё замена)
		} else {
			pvp.Phase = "finished"
			if side == "A" {
				pvp.WinnerUserID = pvp.BUserID
			} else {
				pvp.WinnerUserID = pvp.AUserID
			}
		}
	} else {
		if side == "A" {
			pvp.AwaitReplaceA = false
		} else {
			pvp.AwaitReplaceB = false
		}
		if pvp.Phase != "finished" && !pvp.AwaitReplaceA && !pvp.AwaitReplaceB {
			pvp.Phase = "battling"
		}
	}

	pvp.Log = append(pvp.Log, logv...)
	pvp.LastActivity = time.Now()
	savePvpState(bs)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

type pvpForfeitRequest struct {
	BattleID string `json:"battle_id"`
}

// POST /api/pvp/forfeit — сдаться.
func PvpForfeitHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := onlineUserID(w, r)
	if !ok {
		return
	}
	var req pvpForfeitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Некорректный запрос", http.StatusBadRequest)
		return
	}

	pvpMu.Lock()
	defer pvpMu.Unlock()
	pvp, ok := pvpFind(w, req.BattleID)
	if !ok {
		return
	}
	side := pvpSideOf(pvp, uid)
	if side == "" {
		http.Error(w, "Вы не участник боя", http.StatusForbidden)
		return
	}
	if pvp.Phase == "finished" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		return
	}

	loserName := pvp.AName
	if side == "A" {
		pvp.WinnerUserID = pvp.BUserID
	} else {
		pvp.WinnerUserID = pvp.AUserID
		loserName = pvp.BName
	}
	pvp.Phase = "finished"
	pvp.Log = append(pvp.Log, BattleLogEntry{Round: pvp.BS.Turn, Message: fmt.Sprintf("%s сдался — бой окончен.", loserName)})
	pvp.LastActivity = time.Now()
	savePvpState(pvp.BS)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}
