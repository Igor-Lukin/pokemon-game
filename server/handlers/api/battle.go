package api

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	mathrand "math/rand"
	"net/http"
	"pokemon-game/server/db"
	"pokemon-game/server/session"
	"strconv"
	"sync"
	"time"
)

func newBattleID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ======================== Data Structures ========================

type StatChange struct {
	StatID     int `json:"stat_id"`
	Stages     int `json:"stages"`
	Target     int `json:"target"`
	Chance     int `json:"chance"`
	ReqWeather int `json:"req_weather"` // 0 = любая погода; иначе срабатывает только при этой погоде
}

type BattleMove struct {
	ID             int          `json:"id"`
	Name           string       `json:"name"`
	EngName        string       `json:"eng_name"`
	TypeID         int          `json:"type_id"`
	CategoryID     int          `json:"category_id"`
	Power          int          `json:"power"`
	Accuracy       int          `json:"accuracy"`
	PP             int          `json:"pp"`
	MaxPP          int          `json:"max_pp"`
	Priority       int          `json:"priority"`
	TargetID       int          `json:"target_id"`
	Description    string       `json:"description"`
	EffectID       int          `json:"effect_id"`
	EffectChance   int          `json:"effect_chance"`
	NeverMisses    bool         `json:"never_misses"`
	Drain          int          `json:"drain"`
	CritRate       int          `json:"crit_rate"`
	HitsMin        int          `json:"hits_min"`
	HitsMax        int          `json:"hits_max"`
	Disabled       bool         `json:"disabled"`
	StatChanges    []StatChange `json:"stat_changes"`
}

type StatStages [8]int // 0=HP,1=Atk,2=Def,3=Spd,4=SpAtk,5=SpDef,6=Acc,7=Eva

type BattlePokemon struct {
	ID              int           `json:"id"`
	PokemonID       int           `json:"pokemon_id"`
	Name            string        `json:"name"`
	Level           int           `json:"level"`
	CurrentHP       int           `json:"current_hp"`
	MaxHP           int           `json:"max_hp"`
	Attack          int           `json:"attack"`
	Defense         int           `json:"defense"`
	Speed           int           `json:"speed"`
	SpecialAttack   int           `json:"special_attack"`
	SpecialDefense  int           `json:"special_defense"`
	PrimaryTypeID   int           `json:"primary_type_id"`
	SecondaryTypeID *int          `json:"secondary_type_id,omitempty"`
	ImageURL        string        `json:"image_url"`
	StatusID        int           `json:"status_id"`
	Happiness       int           `json:"-"`
	Moves           []BattleMove  `json:"moves"`
	IsFainted       bool          `json:"is_fainted"`
	// In-battle volatile state
	SleepTurns   int        `json:"-"`
	Confused     bool       `json:"-"`
	ConfusedTurns int       `json:"-"`
	Flinched     bool       `json:"-"`
	PoisonTurns  int        `json:"-"`
	LeechSeeded  bool       `json:"leech_seeded,omitempty"`
	Trapped      bool       `json:"trapped,omitempty"`
	StatStages   StatStages `json:"stat_stages"`
}

type BattleLogEntry struct {
	Round             int     `json:"round"`
	PokemonName       string  `json:"pokemon_name,omitempty"`
	PokemonImage      string  `json:"pokemon_image,omitempty"`
	MoveName          string  `json:"move_name,omitempty"`
	Damage            int     `json:"damage,omitempty"`
	IsCrit            bool    `json:"is_crit,omitempty"`
	Effectiveness     float64 `json:"effectiveness,omitempty"`
	EffectivenessText string  `json:"effectiveness_text,omitempty"`
	Fainted           bool    `json:"fainted,omitempty"`
	StatusEvent       string  `json:"status_event,omitempty"`
	MoveFailed        string  `json:"move_failed,omitempty"`
	Missed            bool    `json:"missed,omitempty"`
	Message           string  `json:"message,omitempty"`
	StatChange        string  `json:"stat_change,omitempty"`
	HitCount          int     `json:"hit_count,omitempty"`
	TotalHits         int     `json:"total_hits,omitempty"`
}

type BattleState struct {
	ID               string
	PlayerTeam       []BattlePokemon
	ActiveIdx        int
	WildPokemon      *BattlePokemon
	Turn             int
	StartedAt        time.Time
	BattleType       string
	Finished         bool
	PlayerName       string
	WeatherTurns     int
	BattleWeatherID  int
	BattleWeatherName string
}

type BattleStartRequest struct {
	LocationID int `json:"location_id"`
}

type BattleMoveRequest struct {
	BattleID string `json:"battle_id"`
	AttackID int    `json:"attack_id"`
}

type BattleResponse struct {
	Type        string           `json:"type"`
	Log         []BattleLogEntry `json:"log"`
	Player      *BattlePokemon   `json:"player,omitempty"`
	Wild        *BattlePokemon   `json:"wild,omitempty"`
	Team        []BattlePokemon  `json:"team,omitempty"`
	BattleID    string           `json:"battle_id,omitempty"`
	Evolution   *EvolutionInfo   `json:"evolution,omitempty"`
	WeatherID   int              `json:"weather_id,omitempty"`
	WeatherName string           `json:"weather_name,omitempty"`
}

// ======================== Session Management ========================

var (
	battleSessions = make(map[string]*BattleState)
	battleMu       sync.Mutex
)

func init() {
	go cleanupBattleSessions()
}

func cleanupBattleSessions() {
	for {
		time.Sleep(5 * time.Minute)
		battleMu.Lock()
		now := time.Now()
		for id, bs := range battleSessions {
			if now.Sub(bs.StartedAt) > 20*time.Minute {
				delete(battleSessions, id)
			}
		}
		battleMu.Unlock()
	}
}

// ======================== Type Effectiveness ========================

var typeEffectCache map[[2]int]float64
var typeEffectOnce sync.Once

func loadTypeEffectiveness() {
	typeEffectCache = make(map[[2]int]float64)
	rows, err := db.DB.Query("SELECT attacking_type_id, defending_type_id, multiplier FROM type_effectiveness")
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var atk, def int
		var mult float64
		if rows.Scan(&atk, &def, &mult) == nil {
			typeEffectCache[[2]int{atk, def}] = mult
		}
	}
}

func getTypeEffectiveness(attackType int, defenderTypes []int) float64 {
	typeEffectOnce.Do(loadTypeEffectiveness)
	mult := 1.0
	for _, dt := range defenderTypes {
		if v, ok := typeEffectCache[[2]int{attackType, dt}]; ok {
			mult *= v
		}
	}
	return mult
}

func getPokemonTypes(p *BattlePokemon) []int {
	types := []int{p.PrimaryTypeID}
	if p.SecondaryTypeID != nil {
		types = append(types, *p.SecondaryTypeID)
	}
	return types
}

// ======================== Damage Calculation ========================

func effectiveStat(base int, stage int) int {
	var mult float64
	if stage >= 0 {
		mult = float64(2+stage) / 2.0
	} else {
		mult = 2.0 / float64(2-stage)
	}
	val := int(math.Floor(float64(base) * mult))
	if val < 1 {
		val = 1
	}
	return val
}

// critChanceForLevel возвращает базовый шанс критического удара по уровню крита.
// Уровень: 1→7%, 2→13%, 3→25%, 4→33%, 5→50%. Значения вне диапазона клампятся.
func critChanceForLevel(level int) float64 {
	switch {
	case level <= 1:
		return 0.07
	case level == 2:
		return 0.13
	case level == 3:
		return 0.25
	case level == 4:
		return 0.33
	default: // 5 и выше
		return 0.50
	}
}

func calculateDamage(attacker, defender *BattlePokemon, move *BattleMove) (damage int, isCrit bool, effectiveness float64) {
	if move.CategoryID == 3 || move.Power == 0 {
		return 0, false, 1
	}

	// Random factor
	r := 0.85 + mathrand.Float64()*0.15

	// Critical hit: шанс зависит от уровня крита (базовый 1 + crit_rate атаки),
	// при максимальном счастье атакующего (255) итоговый шанс +50%.
	critChance := critChanceForLevel(1 + move.CritRate)
	if attacker.Happiness >= 255 {
		critChance *= 1.5
	}
	isCrit = mathrand.Float64() < critChance
	critMult := 1.0
	if isCrit {
		critMult = 1.5
	}

	// Attack / Defense selection based on category
	var a, d int
	if move.CategoryID == 1 { // physical
		a = effectiveStat(attacker.Attack, attacker.StatStages[1])
		d = effectiveStat(defender.Defense, defender.StatStages[2])
		// Множитель атаки от статуса (ожог ×0.5) — из карточки статуса.
		a = int(float64(a) * statMultiplier(attacker.StatusID, 1))
	} else { // special
		a = effectiveStat(attacker.SpecialAttack, attacker.StatStages[4])
		d = effectiveStat(defender.SpecialDefense, defender.StatStages[5])
	}
	if a < 1 {
		a = 1
	}
	if d < 1 {
		d = 1
	}

	// STAB
	atkTypes := getPokemonTypes(attacker)
	stab := 1.0
	for _, t := range atkTypes {
		if t == move.TypeID {
			stab = 1.5
			break
		}
	}

	// Type effectiveness
	defTypes := getPokemonTypes(defender)
	effectiveness = getTypeEffectiveness(move.TypeID, defTypes)

	baseDamage := (float64(2*attacker.Level+10)/250.0)*(float64(a)/float64(d))*float64(move.Power) + 2
	damage = int(math.Floor(baseDamage * stab * effectiveness * critMult * 1.0 * r))
	if damage < 1 {
		damage = 1
	}
	return
}

// ======================== Turn Processing ========================

func effText(eff float64) string {
	if eff == 0 {
		return "Атака не действует!"
	} else if eff < 0.5 {
		return "Атака почти не эффективна..."
	} else if eff < 1 {
		return "Атака не очень эффективна..."
	} else if eff > 2 {
		return "Атака суперэффективна!"
	} else if eff > 1 {
		return "Атака эффективна!"
	}
	return ""
}

func checkHit(move *BattleMove, attacker, defender *BattlePokemon) bool {
	if move.NeverMisses {
		return true
	}
	effAcc := float64(move.Accuracy)
	if attacker.StatStages[6] != 0 {
		effAcc *= accuracyEvasionMultiplier(attacker.StatStages[6])
	}
	if defender.StatStages[7] != 0 {
		effAcc /= accuracyEvasionMultiplier(defender.StatStages[7])
	}
	if effAcc >= 100 {
		return true
	}
	return mathrand.Intn(100) < int(effAcc)
}

func makeEntry(round int, pokemon *BattlePokemon, move *BattleMove, damage int, crit bool, eff float64, fainted bool, missed ...bool) BattleLogEntry {
	e := BattleLogEntry{
		Round:             round,
		PokemonName:       pokemon.Name,
		PokemonImage:      pokemon.ImageURL,
		MoveName:          move.Name,
		Damage:            damage,
		IsCrit:            crit,
		Effectiveness:     eff,
		EffectivenessText: effText(eff),
		Fainted:           fainted,
	}
	if len(missed) > 0 && missed[0] {
		e.Missed = true
	}
	return e
}

func executeDamagingMove(attacker, defender *BattlePokemon, move *BattleMove, weatherID, round int, log *[]BattleLogEntry) (fainted bool, weatherChange int) {
	hit := false

	if move.HitsMin > 1 {
		hitChances := []int{1000, 1000, 333, 166, 166}
		totalDmg := 0
		hitCount := 0
		anyCrit := false
		bestEff := 1.0

		for i := 0; i < move.HitsMax && !defender.IsFainted; i++ {
			chance := hitChances[i]
			if mathrand.Intn(1000) < chance {
				dmg, crit, eff := calculateDamage(attacker, defender, move)
				totalDmg += dmg
				hitCount++
				hit = true
				if crit {
					anyCrit = true
				}
				if eff > bestEff {
					bestEff = eff
				}
				defender.CurrentHP -= dmg
				if defender.CurrentHP < 0 {
					defender.CurrentHP = 0
				}
				if defender.CurrentHP <= 0 {
					defender.IsFainted = true
					fainted = true
				}
			} else {
				break
			}
		}

		applyDrain(attacker, move, totalDmg, round, log)

		entry := BattleLogEntry{
			Round:             round,
			PokemonName:       attacker.Name,
			PokemonImage:      attacker.ImageURL,
			MoveName:          move.Name,
			Damage:            totalDmg,
			IsCrit:            anyCrit,
			Effectiveness:     bestEff,
			EffectivenessText: effText(bestEff),
			Fainted:           false,
			HitCount:          hitCount,
			TotalHits:         move.HitsMax,
		}
		if hitCount == 0 {
			entry.Missed = true
		}
		*log = append(*log, entry)
	} else {
		if checkHit(move, attacker, defender) {
			dmg, crit, eff := calculateDamage(attacker, defender, move)
			defender.CurrentHP -= dmg
			if defender.CurrentHP < 0 {
				defender.CurrentHP = 0
			}
			applyDrain(attacker, move, dmg, round, log)
			fainted = defender.CurrentHP <= 0
			if fainted {
				defender.IsFainted = true
			}
			hit = true
			*log = append(*log, makeEntry(round, attacker, move, dmg, crit, eff, false))
		} else {
			*log = append(*log, makeEntry(round, attacker, move, 0, false, 1, false, true))
		}
	}

	if hit && !fainted {
		weatherChange = applyEffectByID(attacker, defender, move, round, log)
		applyStatChanges(attacker, defender, move, weatherID, round, log)
	} else {
		applyStatChanges(attacker, nil, move, weatherID, round, log)
	}

	return
}

func processPreTurnStatus(p *BattlePokemon, log *[]BattleLogEntry) bool {
	if p.IsFainted {
		return true
	}

	// Испуг (volatile) — может пропустить ход. Логика в volatiles.go.
	if !volatileFlinch(p, log) {
		return false
	}

	// Может ли покемон действовать в этом ходу — из карточки статуса
	// (паралич 25% пропуск, заморозка/оттаивание, сон/пробуждение).
	if def, ok := statusRegistry[p.StatusID]; ok && def.CanAct != nil {
		if !def.CanAct(p, log) {
			return false
		}
	}

	// Урон/лечение по ходам берётся из карточки статуса (реестр statusRegistry).
	// Заменяет прежние отдельные блоки ожога / отравления / сильного отравления.
	if def, ok := statusRegistry[p.StatusID]; ok && def.OnTurn != nil {
		delta := def.OnTurn(p) // отрицательное = урон, положительное = лечение
		if delta != 0 {
			p.CurrentHP += delta
			if delta < 0 {
				*log = append(*log, BattleLogEntry{
					Round:       0,
					PokemonName: p.Name,
					Message:     p.Name + " получает урон от " + def.TickWord + "! -" + strconv.Itoa(-delta) + " HP",
				})
			}
			if p.CurrentHP <= 0 {
				p.CurrentHP = 0
				p.IsFainted = true
				*log = append(*log, BattleLogEntry{
					Round:       0,
					PokemonName: p.Name,
					Fainted:     true,
				})
				return false
			}
		}
	}

	// Замешательство (volatile) — может ударить себя и пропустить ход. Логика в volatiles.go.
	if !volatileConfusion(p, log) {
		return false
	}

	return true
}

func applyEffectByID(attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) int {
	if move.EffectID == 0 || move.EffectChance == 0 {
		return 0
	}
	if mathrand.Intn(100) >= move.EffectChance {
		return 0
	}

	id := move.EffectID
	// Погода (14-17): вернуть id погоды — её ставит setBattleWeather выше по стеку.
	if w, ok := weatherEffects[id]; ok {
		return w
	}
	// Основной статус (2-7): наложение через карточку статуса.
	if _, ok := statusRegistry[id]; ok {
		inflictMajorStatus(defender, id, round, log)
		return 0
	}
	// Volatile-статус (8,10,11,13): наложение через реестр volatiles.
	if fn, ok := inflictVolatile[id]; ok {
		fn(defender, round, log)
		return 0
	}
	return 0
}

func setStatus(p *BattlePokemon, statusID int, round int, log *[]BattleLogEntry) {
	statusNames := map[int]string{
		2: "паралич",
		3: "ожог",
		4: "заморозка",
		5: "сон",
		6: "отравление",
		7: "сильное отравление",
	}
	p.StatusID = statusID
	*log = append(*log, BattleLogEntry{
		Round:        round,
		PokemonName:  p.Name,
		PokemonImage: p.ImageURL,
		StatusEvent:  statusNames[statusID],
	})
}

func setBattleWeather(bs *BattleState, weatherID int, log *[]BattleLogEntry) {
	weatherNames := map[int]string{
		2: "Солнечно",
		3: "Дождь",
		4: "Град",
		5: "Песчаная буря",
	}
	bs.BattleWeatherID = weatherID
	bs.WeatherTurns = 5
	bs.BattleWeatherName = weatherNames[weatherID]
	*log = append(*log, BattleLogEntry{
		Round:       bs.Turn,
		Message:     "Погода изменилась: " + weatherNames[weatherID],
	})
}

// processLeechSeed / processTrapDamage перенесены в volatiles.go
// (volatileLeechSeed / volatileTrap).

func applyDrain(attacker *BattlePokemon, move *BattleMove, dmg int, turn int, log *[]BattleLogEntry) {
	if move.Drain == 0 || dmg <= 0 {
		return
	}
	drainHP := dmg * move.Drain / 100
	if drainHP == 0 {
		if move.Drain > 0 {
			drainHP = 1
		} else {
			drainHP = -1
		}
	}
	attacker.CurrentHP += drainHP
	if move.Drain < 0 {
		if attacker.CurrentHP < 0 {
			attacker.CurrentHP = 0
		}
		*log = append(*log, BattleLogEntry{
			Round:       turn,
			PokemonName: attacker.Name,
			Message:     attacker.Name + " получает " + strconv.Itoa(-drainHP) + " урона от отдачи!",
		})
		if attacker.CurrentHP <= 0 {
			attacker.CurrentHP = 0
			attacker.IsFainted = true
			*log = append(*log, BattleLogEntry{
				Round:       turn,
				PokemonName: attacker.Name,
				Fainted:     true,
			})
		}
	} else {
		if attacker.CurrentHP > attacker.MaxHP {
			attacker.CurrentHP = attacker.MaxHP
		}
		*log = append(*log, BattleLogEntry{
			Round:       turn,
			PokemonName: attacker.Name,
			Message:     attacker.Name + " восстанавливает " + strconv.Itoa(drainHP) + " HP!",
		})
	}
}

var statChangeNames = map[int]string{
	1: "Атака",
	2: "Защита",
	3: "Скорость",
	4: "Спец. Атака",
	5: "Спец. Защита",
	6: "Точность",
	7: "Ловкость",
}

func accuracyEvasionMultiplier(stage int) float64 {
	if stage >= 0 {
		return float64(3+stage) / 3.0
	}
	return 3.0 / float64(3-stage)
}

func loadStatChanges(attackID int) []StatChange {
	rows, err := db.DB.Query(`
		SELECT stat_id, stages, target, chance, COALESCE(req_weather, 0)
		FROM attack_stat_changes
		WHERE attack_id = ?
	`, attackID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var changes []StatChange
	for rows.Next() {
		var sc StatChange
		if rows.Scan(&sc.StatID, &sc.Stages, &sc.Target, &sc.Chance, &sc.ReqWeather) == nil {
			changes = append(changes, sc)
		}
	}
	return changes
}

func applyStatChanges(attacker, defender *BattlePokemon, move *BattleMove, weatherID, round int, log *[]BattleLogEntry) {
	if len(move.StatChanges) == 0 {
		return
	}
	for _, sc := range move.StatChanges {
		// Условие погоды: 0 = любая; иначе строка срабатывает только при нужной погоде.
		if sc.ReqWeather != 0 && weatherID != sc.ReqWeather {
			continue
		}
		if mathrand.Intn(100) >= sc.Chance {
			continue
		}
		var target *BattlePokemon
		var targetName string
		if sc.Target == 1 {
			target = attacker
			targetName = attacker.Name
		} else {
			if defender == nil {
				continue
			}
			target = defender
			targetName = defender.Name
		}
		target.StatStages[sc.StatID] += sc.Stages
		if target.StatStages[sc.StatID] > 6 {
			target.StatStages[sc.StatID] = 6
		} else if target.StatStages[sc.StatID] < -6 {
			target.StatStages[sc.StatID] = -6
		}
		statName := statChangeNames[sc.StatID]
		if statName == "" {
			statName = "Стат"
		}
		verb := "повышена"
		if sc.Stages < 0 {
			verb = "снижена"
		}
		msg := statName + " " + targetName + " " + verb + "!"
		*log = append(*log, BattleLogEntry{
			Round:      round,
			StatChange: msg,
		})
	}
}

func effectiveSpeed(p *BattlePokemon) int {
	s := p.Speed
	// Множитель скорости от статуса (паралич ×0.5) — из карточки статуса.
	s = int(float64(s) * statMultiplier(p.StatusID, 3))
	stage := p.StatStages[3] // index 3 = speed
	if stage != 0 {
		mult := statStageMultiplier(stage)
		s = int(float64(s) * mult)
	}
	if s < 1 {
		s = 1
	}
	return s
}

func statusSkipReason(statusID int) string {
	switch statusID {
	case 2: return "Покемон парализован"
	case 4: return "Покемон заморожен"
	case 5: return "Покемон спит"
	}
	return ""
}

func statStageMultiplier(stage int) float64 {
	if stage >= 0 {
		return float64(2+stage) / 2.0
	}
	return 2.0 / float64(2-stage)
}

func processTurn(bs *BattleState, playerMove *BattleMove) []BattleLogEntry {
	bs.Turn++
	log := []BattleLogEntry{}

	player := &bs.PlayerTeam[bs.ActiveIdx]
	wild := bs.WildPokemon

	if player.IsFainted || wild.IsFainted {
		return log
	}

	// Pre-turn status processing
	playerCanAct := processPreTurnStatus(player, &log)
	wildCanAct := processPreTurnStatus(wild, &log)

	// По-ходовые volatile-эффекты (семена-пиявки, ловушка) — порядок сохранён.
	runEndTurnVolatiles(player, wild, &log)
	if player.IsFainted || wild.IsFainted {
		playerCanAct = !player.IsFainted
		wildCanAct = !wild.IsFainted
		if player.IsFainted {
			log = append(log, BattleLogEntry{
				Round:       bs.Turn,
				PokemonName: player.Name,
				Fainted:     true,
			})
		}
		if wild.IsFainted {
			log = append(log, BattleLogEntry{
				Round:       bs.Turn,
				PokemonName: wild.Name,
				Fainted:     true,
			})
		}
		if !playerCanAct && !wildCanAct {
			return log
		}
		// Only one can act — execute their move directly without wild move/turn order
		var attacker, defender *BattlePokemon
		var move *BattleMove
		if playerCanAct {
			attacker, defender = player, wild
			move = playerMove
		} else {
			attacker, defender = wild, player
			move = chooseWildMove(wild)
		}
		if move.CategoryID != 3 && move.Power > 0 {
			fainted, wc := executeDamagingMove(attacker, defender, move, bs.BattleWeatherID, bs.Turn, &log)
			if wc > 0 {
				setBattleWeather(bs, wc, &log)
			}
			if fainted {
				log = append(log, BattleLogEntry{
					Round:       bs.Turn,
					PokemonName: defender.Name,
					Fainted:     true,
				})
			}
		} else {
			log = append(log, makeEntry(bs.Turn, attacker, move, 0, false, 1, false))
			if move.CategoryID == 3 {
				if wc := applyEffectByID(attacker, defender, move, bs.Turn, &log); wc > 0 {
					setBattleWeather(bs, wc, &log)
				}
				applyStatChanges(attacker, defender, move, bs.BattleWeatherID, bs.Turn, &log)
			}
		}
		return log
	}

	wildMove := chooseWildMove(wild)

	// Log MoveFailed entries for pokemon blocked by status
	if !playerCanAct && !player.IsFainted {
		reason := statusSkipReason(player.StatusID)
		if reason != "" {
			log = append(log, BattleLogEntry{
				Round:       bs.Turn,
				PokemonName: player.Name,
				PokemonImage: player.ImageURL,
				MoveName:    playerMove.Name,
				MoveFailed:  reason,
			})
		}
	}
	if !wildCanAct && !wild.IsFainted {
		reason := statusSkipReason(wild.StatusID)
		if reason != "" {
			log = append(log, BattleLogEntry{
				Round:       bs.Turn,
				PokemonName: wild.Name,
				PokemonImage: wild.ImageURL,
				MoveName:    wildMove.Name,
				MoveFailed:  reason,
			})
		}
	}

	if !playerCanAct && !wildCanAct {
		return log
	}

	// Determine turn order (only among those who can act)
	playerFirst := true
	if playerCanAct && wildCanAct {
		if playerMove.Priority != wildMove.Priority {
			playerFirst = playerMove.Priority > wildMove.Priority
		} else {
			playerSpeed := effectiveSpeed(player)
			wildSpeed := effectiveSpeed(wild)
			playerFirst = playerSpeed > wildSpeed
			if playerSpeed == wildSpeed {
				playerFirst = mathrand.Intn(2) == 0
			}
		}
	}

	var first, second *BattlePokemon
	var firstMove, secondMove *BattleMove
	var firstCanAct, secondCanAct bool

	if playerCanAct && wildCanAct {
		if playerFirst {
			first, firstMove = player, playerMove
			second, secondMove = wild, wildMove
		} else {
			first, firstMove = wild, wildMove
			second, secondMove = player, playerMove
		}
		firstCanAct = true
		secondCanAct = true
	} else if playerCanAct {
		first, firstMove = player, playerMove
		firstCanAct = true
		secondCanAct = false
	} else {
		first, firstMove = wild, wildMove
		firstCanAct = true
		secondCanAct = false
	}

	// Execute first attack
	if firstCanAct && firstMove.CategoryID != 3 && firstMove.Power > 0 {
		fainted, wc := executeDamagingMove(first, second, firstMove, bs.BattleWeatherID, bs.Turn, &log)
		if wc > 0 {
			setBattleWeather(bs, wc, &log)
		}
		if fainted {
			log = append(log, BattleLogEntry{
				Round:       bs.Turn,
				PokemonName: second.Name,
				Fainted:     true,
			})
			return log
		}
	} else if firstCanAct {
		log = append(log, makeEntry(bs.Turn, first, firstMove, 0, false, 1, false))
		if firstMove.CategoryID == 3 {
			if wc := applyEffectByID(first, second, firstMove, bs.Turn, &log); wc > 0 {
				setBattleWeather(bs, wc, &log)
			}
			applyStatChanges(first, second, firstMove, bs.BattleWeatherID, bs.Turn, &log)
		}
	}

	// Execute second attack if applicable
	if secondCanAct && secondMove != nil && !second.IsFainted && !first.IsFainted {
		if secondMove.CategoryID != 3 && secondMove.Power > 0 {
			fainted, wc := executeDamagingMove(second, first, secondMove, bs.BattleWeatherID, bs.Turn, &log)
			if wc > 0 {
				setBattleWeather(bs, wc, &log)
			}
			if fainted {
				log = append(log, BattleLogEntry{
					Round:       bs.Turn,
					PokemonName: first.Name,
					Fainted:     true,
				})
			}
		} else {
			log = append(log, makeEntry(bs.Turn, second, secondMove, 0, false, 1, false))
			if secondMove.CategoryID == 3 {
				if wc := applyEffectByID(second, first, secondMove, bs.Turn, &log); wc > 0 {
					setBattleWeather(bs, wc, &log)
				}
				applyStatChanges(second, first, secondMove, bs.BattleWeatherID, bs.Turn, &log)
			}
		}
	}

	// Decrement PP for player's move if it was used
	if playerCanAct && playerMove.PP > 0 {
		playerMove.PP--
	}

	return log
}

func chooseWildMove(wild *BattlePokemon) *BattleMove {
	if len(wild.Moves) == 0 {
		return &BattleMove{Name: "Нет атаки", CategoryID: 3}
	}
	// Filter to non-status moves if possible
	var damaging []BattleMove
	for _, m := range wild.Moves {
		if m.CategoryID != 3 && m.Power > 0 {
			damaging = append(damaging, m)
		}
	}
	if len(damaging) > 0 {
		return &damaging[mathrand.Intn(len(damaging))]
	}
	return &wild.Moves[mathrand.Intn(len(wild.Moves))]
}

// ======================== Create Player Battle Pokemon ========================

func playerPokemonToBattle(userPokemonID int) *BattlePokemon {
	row := db.DB.QueryRow(`
		SELECT up.id, up.pokemon_id, p.name, up.level, up.current_hp, up.max_hp,
		       up.attack, up.defense, up.speed, up.special_attack, up.special_defense,
		       p.image_url, p.primary_type_id, p.secondary_type_id, up.status_id,
		       COALESCE(up.happiness, 0)
		FROM user_pokemons up
		JOIN pokemons p ON up.pokemon_id = p.id
		WHERE up.id = ?
	`, userPokemonID)

	var bp BattlePokemon
	var imageURL string
	var secType sql.NullInt64
	err := row.Scan(&bp.ID, &bp.PokemonID, &bp.Name, &bp.Level,
		&bp.CurrentHP, &bp.MaxHP, &bp.Attack, &bp.Defense, &bp.Speed,
		&bp.SpecialAttack, &bp.SpecialDefense,
		&imageURL, &bp.PrimaryTypeID, &secType, &bp.StatusID, &bp.Happiness)
	if err != nil {
		return nil
	}
	bp.ImageURL = imageURL
	if secType.Valid {
		st := int(secType.Int64)
		bp.SecondaryTypeID = &st
	}

	if bp.CurrentHP <= 0 {
		bp.IsFainted = true
	}

	// Load learned moves from user_pokemon_learnable_attacks
	moveRows, err := db.DB.Query(`
		SELECT a.id, a.name, COALESCE(a.eng_name, ''), a.type_id, a.category_id, a.power, a.accuracy,
		       ula.current_pp, a.pp,
		       a.priority,
		       COALESCE(a.target_id, 1), COALESCE(a.description, ''),
		       COALESCE(a.effect_status_id, 0), COALESCE(a.effect_chance, 0),
		       COALESCE(a.never_misses, 0), COALESCE(a.drain, 0),
		       COALESCE(a.crit_rate, 0), COALESCE(a.hits_min, 1), COALESCE(a.hits_max, 1),
		       COALESCE(a.disabled, 0)
		FROM user_pokemon_learnable_attacks ula
		JOIN attacks a ON ula.attack_id = a.id
		WHERE ula.user_pokemon_id = ? AND ula.is_learned = 1 AND ula.position IS NOT NULL
		ORDER BY ula.position
	`, bp.ID)
	if err == nil {
		defer moveRows.Close()
		for moveRows.Next() {
			var m BattleMove
			var neverMisses, disabled int
			if moveRows.Scan(&m.ID, &m.Name, &m.EngName, &m.TypeID, &m.CategoryID, &m.Power, &m.Accuracy, &m.PP, &m.MaxPP, &m.Priority, &m.TargetID, &m.Description, &m.EffectID, &m.EffectChance, &neverMisses, &m.Drain, &m.CritRate, &m.HitsMin, &m.HitsMax, &disabled) == nil {
			m.NeverMisses = neverMisses != 0
			m.Disabled = disabled != 0
			m.StatChanges = loadStatChanges(m.ID)
				bp.Moves = append(bp.Moves, m)
			}
		}
	}

	return &bp
}

func wildToBattlePokemon(w *WildPokemonGenerate) *BattlePokemon {
	moves := make([]BattleMove, len(w.Moves))
	for i, m := range w.Moves {
		moves[i] = BattleMove{
			ID:             m.ID,
			Name:           m.Name,
			EngName:        m.EngName,
			TypeID:         m.TypeID,
			CategoryID:     m.CategoryID,
			Power:          m.Power,
			Accuracy:       m.Accuracy,
			PP:             -1, // infinite PP
			MaxPP:          -1,
			Priority:       m.Priority,
			TargetID:       m.TargetID,
			Description:    m.Description,
			EffectID: m.EffectID,
			EffectChance:   m.EffectChance,
			NeverMisses:    m.NeverMisses,
			Disabled:       m.Disabled,
			Drain:          m.Drain,
			CritRate:       m.CritRate,
			HitsMin:        m.HitsMin,
			HitsMax:        m.HitsMax,
			StatChanges:    loadStatChanges(m.ID),
		}
	}

	return &BattlePokemon{
		ID:              0,
		PokemonID:       w.PokemonID,
		Name:            w.Name,
		Level:           w.Level,
		CurrentHP:       w.MaxHP,
		MaxHP:           w.MaxHP,
		Attack:          w.Attack,
		Defense:         w.Defense,
		Speed:           w.Speed,
		SpecialAttack:   w.SpecialAttack,
		SpecialDefense:  w.SpecialDefense,
		PrimaryTypeID:   w.PrimaryTypeID,
		SecondaryTypeID: w.SecondaryTypeID,
		ImageURL:        w.ImageURL,
		StatusID:        0,
		Moves:           moves,
		IsFainted:       false,
	}
}

// ======================== HTTP Handlers ========================

func StartBattleHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	userID, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req BattleStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Load all team pokemon
	rows, err := db.DB.Query(`
		SELECT id FROM user_pokemons
		WHERE user_id = ? AND position BETWEEN 1 AND 6
		ORDER BY position
	`, userID)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var teamIDs []int
	for rows.Next() {
		var id int
		rows.Scan(&id)
		teamIDs = append(teamIDs, id)
	}
	if len(teamIDs) == 0 {
		http.Error(w, "Нет покемонов в команде", http.StatusBadRequest)
		return
	}

	var playerTeam []BattlePokemon
	firstAlive := -1
	for i, id := range teamIDs {
		p := playerPokemonToBattle(id)
		if p == nil {
			continue
		}
		if firstAlive < 0 && !p.IsFainted {
			firstAlive = i
		}
		playerTeam = append(playerTeam, *p)
	}
	if len(playerTeam) == 0 {
		http.Error(w, "Нет покемонов в команде", http.StatusBadRequest)
		return
	}
	if firstAlive < 0 {
		http.Error(w, "Все покемоны в команде без сознания", http.StatusBadRequest)
		return
	}

	wildGen := generateWildPokemon(req.LocationID)
	if wildGen == nil {
		http.Error(w, "No wild pokemon in this location", http.StatusNotFound)
		return
	}
	wildPoke := wildToBattlePokemon(wildGen)

	battleID := newBattleID()
	var playerName string
	db.DB.QueryRow("SELECT username FROM users WHERE id = ?", userID).Scan(&playerName)

	bs := &BattleState{
		ID:         battleID,
		PlayerTeam: playerTeam,
		ActiveIdx:  firstAlive,
		WildPokemon: wildPoke,
		Turn:       0,
		StartedAt:  time.Now(),
		BattleType: "wild",
		PlayerName: playerName,
	}

	playerPoke := &bs.PlayerTeam[bs.ActiveIdx]

	battleMu.Lock()
	battleSessions[battleID] = bs
	battleMu.Unlock()

	// Получить погоду в регионе
	var weatherID int
	var weatherName string
	db.DB.QueryRow(`
		SELECT w.id, w.name FROM weather w
		JOIN regions r ON r.current_weather_id = w.id
		JOIN locations l ON l.region_id = r.id
		WHERE l.id = ?
	`, req.LocationID).Scan(&weatherID, &weatherName)

	weatherTurns := 0
	if weatherID != 1 {
		weatherTurns = 5
	}
	bs.WeatherTurns = weatherTurns
	bs.BattleWeatherID = weatherID
	bs.BattleWeatherName = weatherName

	resp := BattleResponse{
		Type:        "start",
		BattleID:    battleID,
		Player:      playerPoke,
		Wild:        wildPoke,
		WeatherID:   weatherID,
		WeatherName: weatherName,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func MoveHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	_, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req BattleMoveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	battleMu.Lock()
	bs, exists := battleSessions[req.BattleID]
	if !exists {
		battleMu.Unlock()
		http.Error(w, "Battle not found", http.StatusNotFound)
		return
	}
	battleMu.Unlock()

	if bs.Finished {
		http.Error(w, "Battle already finished", http.StatusBadRequest)
		return
	}

	player := &bs.PlayerTeam[bs.ActiveIdx]
	if player.IsFainted {
		hasAlive := false
		for _, p := range bs.PlayerTeam {
			if !p.IsFainted {
				hasAlive = true
				break
			}
		}
		respType := "defeat"
		if hasAlive {
			respType = "need_switch"
		}
		resp := BattleResponse{
			Type:        respType,
			Log:         []BattleLogEntry{{PokemonName: player.Name, Fainted: true, Message: player.Name + " без сознания!"}},
			Player:      copyBattlePokemon(player),
			Wild:        copyBattlePokemon(bs.WildPokemon),
			WeatherID:   bs.BattleWeatherID,
			WeatherName: bs.BattleWeatherName,
		}
		if hasAlive {
			team := make([]BattlePokemon, len(bs.PlayerTeam))
			for i := range bs.PlayerTeam {
				team[i] = *copyBattlePokemon(&bs.PlayerTeam[i])
			}
			resp.Team = team
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	// Find the move
	var playerMove *BattleMove
	for i := range player.Moves {
		if player.Moves[i].ID == req.AttackID {
			playerMove = &player.Moves[i]
			break
		}
	}
	if playerMove == nil {
		http.Error(w, "Attack not found", http.StatusNotFound)
		return
	}

	if playerMove.PP <= 0 {
		http.Error(w, "Атака не имеет PP", http.StatusBadRequest)
		return
	}

	log := processTurn(bs, playerMove)

	postProcessWeather(bs, &log)

	// Determine response type
	respType := "turn"
	var evoInfo *EvolutionInfo
	if bs.WildPokemon.IsFainted {
		respType = "victory"
		bs.Finished = true
		evoInfo = awardExp(player, bs.WildPokemon.PokemonID)
		saveBattleState(bs)
		battleMu.Lock()
		delete(battleSessions, req.BattleID)
		battleMu.Unlock()
	} else if player.IsFainted {
		// Check if there are more pokemon in the team
		hasMore := false
		for _, p := range bs.PlayerTeam {
			if !p.IsFainted {
				hasMore = true
				break
			}
		}
		if hasMore {
			respType = "need_switch"
		} else {
			respType = "defeat"
			saveBattleState(bs)
			bs.Finished = true
			battleMu.Lock()
			delete(battleSessions, req.BattleID)
			battleMu.Unlock()
		}
	}

	resp := BattleResponse{
		Type:        respType,
		Log:         log,
		Player:      copyBattlePokemon(player),
		Wild:        copyBattlePokemon(bs.WildPokemon),
		WeatherID:   bs.BattleWeatherID,
		WeatherName: bs.BattleWeatherName,
	}

	if evoInfo != nil && evoInfo.CanEvolve {
		resp.Evolution = evoInfo
	}

	// Include full team for switch selection
	if respType == "need_switch" {
		team := make([]BattlePokemon, len(bs.PlayerTeam))
		for i := range bs.PlayerTeam {
			team[i] = *copyBattlePokemon(&bs.PlayerTeam[i])
		}
		resp.Team = team
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func SwitchHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	_, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		BattleID     string `json:"battle_id"`
		UserPokemonID int   `json:"user_pokemon_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	battleMu.Lock()
	bs, exists := battleSessions[req.BattleID]
	battleMu.Unlock()
	if !exists {
		http.Error(w, "Battle not found", http.StatusNotFound)
		return
	}

	// Find the pokemon in the team
	oldIdx := bs.ActiveIdx
	found := false
	newIdx := -1
	for i, p := range bs.PlayerTeam {
		if p.ID == req.UserPokemonID && !p.IsFainted {
			newIdx = i
			found = true
			break
		}
	}
	if !found {
		http.Error(w, "Pokemon not available", http.StatusBadRequest)
		return
	}
	if newIdx == oldIdx {
		http.Error(w, "Already in battle", http.StatusBadRequest)
		return
	}

	// Check if active pokemon is trapped
	oldActive := &bs.PlayerTeam[bs.ActiveIdx]
	if oldActive.Trapped {
		http.Error(w, "Покемон пойман в ловушку и не может быть заменён!", http.StatusBadRequest)
		return
	}

	bs.ActiveIdx = newIdx
	bs.Turn++

	active := &bs.PlayerTeam[bs.ActiveIdx]
	wild := bs.WildPokemon
	logMsg := fmt.Sprintf("%s : %s в бой!", bs.PlayerName, active.Name)
	logEntries := []BattleLogEntry{
		{Round: bs.Turn, PokemonName: active.Name, PokemonImage: active.ImageURL, Message: logMsg},
	}

	// Pre-turn status damage for wild pokemon (burn/poison)
	processPreTurnStatus(wild, &logEntries)

	// Leech seed & trap damage for wild pokemon (volatiles.go)
	volatileLeechSeed(active, wild, &logEntries)
	volatileTrap(wild, &logEntries)

	if wild.IsFainted {
		logEntries = append(logEntries, BattleLogEntry{
			Round:       bs.Turn,
			PokemonName: wild.Name,
			Fainted:     true,
		})
		bs.Finished = true
		saveBattleState(bs)
		battleMu.Lock()
		delete(battleSessions, req.BattleID)
		battleMu.Unlock()
		resp := BattleResponse{
			Type:        "victory",
			Log:         logEntries,
			Player:      copyBattlePokemon(active),
			Wild:        copyBattlePokemon(wild),
			WeatherID:   bs.BattleWeatherID,
			WeatherName: bs.BattleWeatherName,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	postProcessWeather(bs, &logEntries)

	// Wild pokemon attacks after switch
	if !wild.IsFainted {
		wildMove := chooseWildMove(wild)
		if wildMove != nil {
			if wildMove.CategoryID != 3 && wildMove.Power > 0 {
				fainted, wc := executeDamagingMove(wild, active, wildMove, bs.BattleWeatherID, bs.Turn, &logEntries)
				if wc > 0 {
					setBattleWeather(bs, wc, &logEntries)
				}
				if fainted {
					logEntries = append(logEntries, BattleLogEntry{
						Round:       bs.Turn,
						PokemonName: active.Name,
						Fainted:     true,
					})
				}
			} else {
				logEntries = append(logEntries, makeEntry(bs.Turn, wild, wildMove, 0, false, 1, false))
				if wc := applyEffectByID(wild, active, wildMove, bs.Turn, &logEntries); wc > 0 {
					setBattleWeather(bs, wc, &logEntries)
				}
				applyStatChanges(wild, active, wildMove, bs.BattleWeatherID, bs.Turn, &logEntries)
			}
		}
	}

	resp := BattleResponse{
		Type:        "switch_done",
		Log:         logEntries,
		Player:      copyBattlePokemon(active),
		Wild:        copyBattlePokemon(wild),
		WeatherID:   bs.BattleWeatherID,
		WeatherName: bs.BattleWeatherName,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func RunHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	_, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		BattleID string `json:"battle_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	battleMu.Lock()
	bs, exists := battleSessions[req.BattleID]
	if exists {
		saveBattleState(bs)
		bs.Finished = true
		delete(battleSessions, req.BattleID)
	}
	battleMu.Unlock()

	if !exists {
		http.Error(w, "Battle not found", http.StatusNotFound)
		return
	}

	// Капитуляция всегда успешна — бой уже завершён на сервере выше.
	resp := BattleResponse{
		Type: "run_success",
		Log:  []BattleLogEntry{{Message: "Вы сдались!"}},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func postProcessWeather(bs *BattleState, log *[]BattleLogEntry) {
	if bs.WeatherTurns > 0 {
		bs.WeatherTurns--
		if bs.WeatherTurns == 0 && bs.BattleWeatherID != 1 {
			bs.BattleWeatherID = 1
			bs.BattleWeatherName = "Обычная"
			*log = append(*log, BattleLogEntry{
				Round:   bs.Turn,
				Message: "Погода вернулась в обычное состояние!",
			})
		}
	}
}

func copyBattlePokemon(p *BattlePokemon) *BattlePokemon {
	c := *p
	c.Moves = make([]BattleMove, len(p.Moves))
	copy(c.Moves, p.Moves)
	return &c
}

func expNeeded(experienceGroup, level int) int {
	switch experienceGroup {
	case 1, 2:
		return (4 * level * level * level) / 5
	case 3:
		return level * level * level
	case 4:
		n := float64(level)
		exp := (6.0/5.0)*n*n*n - 15*n*n + 100*n - 140
		return int(exp)
	case 5, 6:
		return (5 * level * level * level) / 4
	default:
		return level * level * level
	}
}

func awardExp(player *BattlePokemon, wildPokemonID int) *EvolutionInfo {
	var exp, level, pokemonID, natureID, experienceGroup, availableEV int
	var iv [6]int
	var ev [6]int
	err := db.DB.QueryRow(`
		SELECT up.experience, up.level, up.pokemon_id, up.nature_id,
		       up.iv_hp, up.iv_attack, up.iv_defense, up.iv_speed,
		       up.iv_special_attack, up.iv_special_defense,
		       up.ev_hp, up.ev_attack, up.ev_defense, up.ev_speed,
		       up.ev_special_attack, up.ev_special_defense,
		       COALESCE(up.available_ev, 0)
		FROM user_pokemons up WHERE up.id = ?
	`, player.ID).Scan(&exp, &level, &pokemonID, &natureID,
		&iv[0], &iv[1], &iv[2], &iv[3], &iv[4], &iv[5],
		&ev[0], &ev[1], &ev[2], &ev[3], &ev[4], &ev[5], &availableEV)
	if err != nil {
		log.Printf("awardExp: query error: %v", err)
		return nil
	}

	err = db.DB.QueryRow("SELECT experience_group FROM pokemons WHERE id = ?", pokemonID).Scan(&experienceGroup)
	if err != nil {
		log.Printf("awardExp: experience_group query error: %v", err)
		return nil
	}

	var categoryPower int
	err = db.DB.QueryRow("SELECT category_power FROM pokemons WHERE id = ?", wildPokemonID).Scan(&categoryPower)
	if err != nil {
		log.Printf("awardExp: category_power query error: %v", err)
		return nil
	}

	const other = 1.0
	gain := int(float64(categoryPower) * 100 * other)
	exp += gain
	leveledUp := false
	startLevel := level
	needed := expNeeded(experienceGroup, level)
	for exp >= needed {
		level++
		exp -= needed
		leveledUp = true
		if level >= 100 {
			break
		}
		needed = expNeeded(experienceGroup, level)
	}

	levelsGained := level - startLevel
	if levelsGained > 0 {
		availableEV += levelsGained * 2
	}

	var evoInfo *EvolutionInfo

	if leveledUp {
		var newPokemonID int
		var maxHP int
		newPokemonID, evoInfo, maxHP, err = applyLevelUp(player.ID, pokemonID, level, iv, ev, natureID, availableEV)
		if err != nil {
			log.Printf("awardExp: applyLevelUp error: %v", err)
			return nil
		}
		_, err = db.DB.Exec("UPDATE user_pokemons SET experience = ? WHERE id = ?", exp, player.ID)
		if err != nil {
			log.Printf("awardExp: update exp error: %v", err)
		}
		player.CurrentHP = maxHP
		player.MaxHP = maxHP
		log.Printf("awardExp: pokemon %d leveled up to %d (pokemon_id %d -> %d)", player.ID, level, pokemonID, newPokemonID)
	} else {
		_, err = db.DB.Exec("UPDATE user_pokemons SET experience = ?, available_ev = ? WHERE id = ?", exp, availableEV, player.ID)
		if err != nil {
			log.Printf("awardExp: update exp error: %v", err)
		}
	}

	log.Printf("awardExp: pokemon %d earned %d EXP (category_power=%d, total=%d, level=%d)", player.ID, gain, categoryPower, exp, level)
	return evoInfo
}

func checkEvolutionByIDs(pokemonID, level int) (*EvolutionInfo, error) {
	var evolvesToID, evolutionLevel int
	var evolvesToName string
	err := db.DB.QueryRow(`
		SELECT id, evolution_level, name FROM pokemons
		WHERE evolves_from_id = ? AND evolution_method_id = 1 AND evolution_level <= ?
		ORDER BY evolution_level
		LIMIT 1
	`, pokemonID, level).Scan(&evolvesToID, &evolutionLevel, &evolvesToName)
	if err != nil {
		return &EvolutionInfo{CanEvolve: false}, nil
	}
	return &EvolutionInfo{
		CanEvolve:      true,
		EvolvesToID:    evolvesToID,
		EvolvesToName:  evolvesToName,
		EvolutionLevel: evolutionLevel,
	}, nil
}

func saveBattleState(bs *BattleState) {
	for _, p := range bs.PlayerTeam {
		statusID := p.StatusID
		if statusID < 1 || statusID > 7 {
			statusID = 1
		}
		db.DB.Exec("UPDATE user_pokemons SET current_hp = ?, status_id = ? WHERE id = ?",
			p.CurrentHP, statusID, p.ID)

		// Save current_pp for each move
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
