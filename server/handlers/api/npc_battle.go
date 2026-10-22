package api

import (
	"encoding/json"
	"fmt"
	"log"
	mathrand "math/rand"
	"net/http"
	"pokemon-game/server/db"
	"pokemon-game/server/session"
	"time"
)

// Тренерский бой vs команда NPC (поток B). Точка входа из диалога: опция с
// action='battle' -> клиент зовёт /api/battle/trainer/start. Дальше бой идёт по
// общим хендлерам (move/switch/run) — команда врага реализована «указательным
// трюком»: bs.WildPokemon всегда указывает на активного врага в bs.EnemyTeam.

type trainerBattleRequest struct {
	NPCID int `json:"npc_id"`
}

// TrainerBattleStartHandler — старт тренерского боя из диалога с NPC.
func TrainerBattleStartHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	userID, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req trainerBattleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// NPC + настройки команды (поля от потока A).
	var npcName, teamMode string
	var npcLoc, teamSize, minLevel, maxLevel int
	err := db.DB.QueryRow(`
		SELECT name, location_id, COALESCE(team_mode,'random'), COALESCE(team_size,6),
		       COALESCE(min_level,5), COALESCE(max_level,10)
		FROM npc WHERE id = ?`, req.NPCID).
		Scan(&npcName, &npcLoc, &teamMode, &teamSize, &minLevel, &maxLevel)
	if err != nil {
		http.Error(w, "NPC не найден", http.StatusNotFound)
		return
	}

	// Команда игрока — общий хелпер (одна точка правды с диким стартом).
	playerTeam, firstAlive, err := buildPlayerTeam(userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Команда NPC (>=1 боеспособного).
	enemyTeam, err := buildNPCTeam(req.NPCID, teamMode, teamSize, minLevel, maxLevel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	battleID := newBattleID()
	var playerName string
	db.DB.QueryRow("SELECT username FROM users WHERE id = ?", userID).Scan(&playerName)

	bs := &BattleState{
		ID:             battleID,
		PlayerTeam:     playerTeam,
		ActiveIdx:      firstAlive,
		EnemyTeam:      enemyTeam,
		EnemyActiveIdx: 0,
		EnemySmart:     true, // все тренеры «умные» (random/fixed убраны) -> ИИ-замены всегда

		WildPokemon:    &enemyTeam[0], // указатель в срез: EnemyTeam строим один раз, не append-им
		TrainerName:    npcName,
		Turn:           0,
		StartedAt:      time.Now(),
		BattleType:     "trainer",
		PlayerName:     playerName,
	}
	// Первым в бой должен идти живой враг (страховка, если [0] почему-то пал).
	if bs.WildPokemon.IsFainted || bs.WildPokemon.CurrentHP <= 0 {
		for i := range bs.EnemyTeam {
			if !bs.EnemyTeam[i].IsFainted && bs.EnemyTeam[i].CurrentHP > 0 {
				bs.EnemyActiveIdx = i
				bs.WildPokemon = &bs.EnemyTeam[i]
				break
			}
		}
	}

	battleMu.Lock()
	battleSessions[battleID] = bs
	battleMu.Unlock()

	// Погода по локации NPC (тот же SELECT, что в StartBattleHandler).
	var weatherID int
	var weatherName string
	db.DB.QueryRow(`
		SELECT w.id, w.name FROM weather w
		JOIN regions r ON r.current_weather_id = w.id
		JOIN locations l ON l.region_id = r.id
		WHERE l.id = ?
	`, npcLoc).Scan(&weatherID, &weatherName)

	weatherTurns := 0
	if weatherID != 1 && weatherID != 0 {
		weatherTurns = 5
	}
	bs.WeatherTurns = weatherTurns
	bs.BattleWeatherID = weatherID
	bs.BattleWeatherName = weatherName

	// [ability hook] Способности ЛИДОВ при старте боя (лиды не идут через
	// applyEntryHazards). Пусто => no-op; демо-ветка logtest пишет в лог старта.
	var startLog []BattleLogEntry
	abilityOnSwitchIn(bs, &bs.PlayerTeam[bs.ActiveIdx], &startLog)
	abilityOnSwitchIn(bs, bs.WildPokemon, &startLog)

	resp := BattleResponse{
		Type:         "start",
		Log:          startLog,
		BattleID:     battleID,
		Player:       &bs.PlayerTeam[bs.ActiveIdx],
		Wild:         bs.WildPokemon,
		WeatherID:    weatherID,
		WeatherName:  weatherName,
		WeatherTurns: weatherTurns,
	}
	fillEnemyIndicator(bs, &resp)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// buildNPCTeam строит боевую команду NPC (живёт только на бой, не persistent).
// Режимы random/fixed убраны по решению владельца — ВСЕ тренеры «умные» (поток
// [12]): всегда buildSmartNPCTeam с фиксированным уровнем minL. Параметры mode/maxL
// сохранены в сигнатуре для совместимости, но не влияют на состав. random-набор
// остался ТОЛЬКО как защитный фолбэк на случай пустого умного пула (бой не падает).
func buildNPCTeam(npcID int, mode string, size, minL, maxL int) ([]BattlePokemon, error) {
	_ = mode
	_ = npcID
	if size < 1 {
		size = 1
	} else if size > 6 {
		size = 6
	}
	if minL < 1 {
		minL = 1
	}
	if maxL < minL {
		maxL = minL
	}

	// Умная генерация — единственный штатный путь.
	if smart, err := buildSmartNPCTeam(size, minL); err == nil && len(smart) > 0 {
		return smart, nil
	} else if err != nil {
		log.Printf("[smart-team] умный драфт не удался (%v) -> защитный random-фолбэк", err)
	}

	// Защитный фолбэк (не режим): пустой/сбойный умный пул -> случайные виды,
	// чтобы тренерский бой всё равно состоялся.
	var team []BattlePokemon
	rows, err := db.DB.Query("SELECT id FROM pokemons ORDER BY RANDOM() LIMIT ?", size)
	if err != nil {
		return nil, fmt.Errorf("не удалось сформировать команду тренера")
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		lvl := minL
		if maxL > minL {
			lvl = minL + mathrand.Intn(maxL-minL+1)
		}
		if gen := buildGenerateFor(id, lvl); gen != nil {
			team = append(team, *wildToBattlePokemon(gen))
		}
	}

	alive := 0
	for i := range team {
		if !team[i].IsFainted && team[i].CurrentHP > 0 {
			alive++
		}
	}
	if len(team) == 0 || alive == 0 {
		return nil, fmt.Errorf("у тренера нет команды")
	}
	return team, nil
}

// handleEnemyFaint вызывается там, где раньше «враг пал = победа».
// Возвращает:
//   allEnemyDown — вся команда врага повержена (для "wild" — единственный враг);
//   switched     — активный враг сменился на следующего (только "trainer").
// Инвариант указателя: bs.EnemyTeam НЕ реаллоцируем; смена врага — только
// переустановка индекса и bs.WildPokemon на элемент среза.
func handleEnemyFaint(bs *BattleState, log *[]BattleLogEntry) (allEnemyDown, switched bool) {
	if bs.WildPokemon != nil && !bs.WildPokemon.IsFainted {
		return false, false
	}
	if bs.BattleType != "trainer" {
		return true, false // одиночка пал = конец (старое поведение дикого боя)
	}
	// Ищем следующего живого врага. Если входящий гибнет от ловушек — берём дальше.
	for {
		next := -1
		for i := range bs.EnemyTeam {
			if !bs.EnemyTeam[i].IsFainted && bs.EnemyTeam[i].CurrentHP > 0 {
				next = i
				break
			}
		}
		if next < 0 {
			return true, false // вся команда NPC пала = победа игрока
		}
		bs.EnemyActiveIdx = next
		bs.WildPokemon = &bs.EnemyTeam[next]
		*log = append(*log, BattleLogEntry{
			Round:        bs.Turn,
			PokemonName:  bs.WildPokemon.Name,
			PokemonImage: bs.WildPokemon.ImageURL,
			Message:      fmt.Sprintf("%s отправляет %s в бой!", bs.TrainerName, bs.WildPokemon.Name),
		})
		// Вход врага: ловушки его стороны (та же функция, что применяет их к игроку).
		if applyEntryHazards(bs, bs.WildPokemon, &bs.WildSide, bs.Turn, log) {
			*log = append(*log, BattleLogEntry{
				Round:       bs.Turn,
				PokemonName: bs.WildPokemon.Name,
				Fainted:     true,
			})
			continue // входящий пал от ловушек — следующий
		}
		return false, true
	}
}

// fillEnemyIndicator проставляет индикатор отряда врага в ответ (только trainer).
func fillEnemyIndicator(bs *BattleState, resp *BattleResponse) {
	if bs.BattleType != "trainer" {
		return
	}
	remaining := 0
	for i := range bs.EnemyTeam {
		if !bs.EnemyTeam[i].IsFainted && bs.EnemyTeam[i].CurrentHP > 0 {
			remaining++
		}
	}
	resp.EnemyCount = len(bs.EnemyTeam)
	resp.EnemyRemaining = remaining
	resp.TrainerName = bs.TrainerName
}
