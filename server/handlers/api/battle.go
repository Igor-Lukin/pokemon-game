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
	ID           int          `json:"id"`
	Name         string       `json:"name"`
	EngName      string       `json:"eng_name"`
	TypeID       int          `json:"type_id"`
	CategoryID   int          `json:"category_id"`
	Power        int          `json:"power"`
	Accuracy     int          `json:"accuracy"`
	PP           int          `json:"pp"`
	MaxPP        int          `json:"max_pp"`
	Priority     int          `json:"priority"`
	TargetID     int          `json:"target_id"`
	Description  string       `json:"description"`
	EffectID     int          `json:"effect_id"`
	EffectChance int          `json:"effect_chance"`
	NeverMisses  bool         `json:"never_misses"`
	Drain        int          `json:"drain"`
	HealPercent  int          `json:"heal_percent"` // мгновенное лечение носителя на % от MaxHP (Восстановление)
	CritRate     int          `json:"crit_rate"`
	HitsMin      int          `json:"hits_min"`
	HitsMax      int          `json:"hits_max"`
	Disabled     bool         `json:"disabled"`
	Contact      bool         `json:"contact"` // контактная атака (для Каштанового/Королевского щита)
	Sound        bool         `json:"sound"`   // звуковая атака (Звукоизоляция/Панк-рок/Жидкий голос)
	StatChanges  []StatChange `json:"stat_changes"`
}

type StatStages [8]int // 0=HP,1=Atk,2=Def,3=Spd,4=SpAtk,5=SpDef,6=Acc,7=Eva

type BattlePokemon struct {
	ID             int    `json:"id"`
	PokemonID      int    `json:"pokemon_id"`
	Name           string `json:"name"`
	Level          int    `json:"level"`
	CurrentHP      int    `json:"current_hp"`
	MaxHP          int    `json:"max_hp"`
	Attack         int    `json:"attack"`
	Defense        int    `json:"defense"`
	Speed          int    `json:"speed"`
	SpecialAttack  int    `json:"special_attack"`
	SpecialDefense int    `json:"special_defense"`
	// Снимок базовых статов при загрузке — для восстановления при смене после
	// обмена/усреднения (Ловкая подмена/Подмена скорости/Разделение атаки/защиты).
	BaseAttack         int     `json:"-"`
	BaseDefense        int     `json:"-"`
	BaseSpeed          int     `json:"-"`
	BaseSpecialAttack  int     `json:"-"`
	BaseSpecialDefense int     `json:"-"`
	Weight             float64 `json:"weight,omitempty"` // вес покемона (для атак по весу)
	PrimaryTypeID      int     `json:"primary_type_id"`
	SecondaryTypeID    *int    `json:"secondary_type_id,omitempty"`
	ImageURL           string  `json:"image_url"`
	StatusID           int     `json:"status_id"`
	IsStarter          bool    `json:"is_starter,omitempty"`
	Happiness          int     `json:"-"`
	Gender             string  `json:"gender,omitempty"` // 'M'/'F'/'N' — для гейта гендерных атак (не волатиль, при смене НЕ сбрасывается)
	// [ability] Способность экземпляра, загруженная в бой. Эффекты пока НЕ пишем —
	// диспетчер abilities_battle.go читает AbilityKey (effect_key). При смене НЕ
	// сбрасывается (свойство особи, не волатиль).
	AbilityID        int    `json:"ability_id,omitempty"`
	AbilityKey       string `json:"ability_key,omitempty"`
	AbilityName      string `json:"ability_name,omitempty"`
	FlashFireBoosted bool   `json:"-"` // Внутренний огонь: поглотил огонь -> огненные атаки ×1.5 (batch2)
	// [batch5] Форма по HP (Стайность/Сброс брони/Тотем). Снимок исходных
	// значений — для возврата при смене/условии.
	FormKey                string `json:"form_key,omitempty"`
	formBaseName           string
	formBaseImage          string
	formBaseTypes          []int
	formBaseAttack         int
	formBaseDefense        int
	formBaseSpeed          int
	formBaseSpecialAttack  int
	formBaseSpecialDefense int
	abilityLogSeen         map[string]bool // [ability log] once-per-battle сообщения
	MovedSecond            bool            // [batch7] ходил вторым в текущем раунде (Аналитик)
	SwitchedInTurn         int             // [batch7] раунд выхода на поле (Засада)
	DisguiseBroken         bool            // [batch7] Маскировка/Ледяное лицо: щит сломан
	illusionName           string          // [batch7] Иллюзия: исходное имя/спрайт
	illusionImage          string
	Moves                  []BattleMove `json:"moves"`
	IsFainted              bool         `json:"is_fainted"`
	// In-battle volatile state
	SleepTurns    int  `json:"-"`
	Confused      bool `json:"-"`
	ConfusedTurns int  `json:"-"`
	Infatuated    bool `json:"infatuated,omitempty"` // Влюблённость (Соблазн, 392): ~50% пропуск хода; снимается при уходе
	Flinched      bool `json:"-"`
	PoisonTurns   int  `json:"-"`
	LeechSeeded   bool `json:"leech_seeded,omitempty"`
	Trapped       bool `json:"trapped,omitempty"`
	TrapTurns     int  `json:"-"`                   // связывание (Спираль огня/Зажим/…): осталось ходов урона-удержания
	NoEscape      bool `json:"no_escape,omitempty"` // Пристальный взгляд: нельзя сменить (без урона)
	Cursed        bool `json:"cursed,omitempty"`
	Nightmared    bool `json:"nightmared,omitempty"` // Кошмарные сны: спящая цель теряет 1/4 HP/ход
	// Жертвенные ходы (sacrifice_moves.go):
	DestinyBond     bool `json:"-"` // Связь судеб: если носитель гибнет от удара — убийца тоже
	DestinyBondTurn int  `json:"-"` // раунд установки Связи судеб (для истечения)
	Grudge          bool `json:"-"` // Жалостливый взор: гибель от удара обнуляет PP того хода
	GrudgeTurn      int  `json:"-"`
	PerishCount     int  `json:"-"` // Скорбная песнь: обморок через N ходов (0 = нет)
	PerishSetTurn   int  `json:"-"`
	Electrified     bool `json:"-"` // Электризация: следующая атака этого покемона станет Электрической
	MagnetRiseTurns int  `json:"-"` // Магнетизм: невосприимчивость к Земле N ходов (field_effects.go)
	LevitatedTurns  int  `json:"-"` // Левитация: иммунитет к Земле + по цели всегда попадают N ходов
	YawnTurns       int  `json:"-"` // Зевок: заснёт, когда счётчик истечёт (status_extra_moves.go)
	YawnSetTurn     int  `json:"-"`
	// Трансформация (transform_moves.go): снимок для отката при смене. Типы/базовые
	// статы откатываются через BattleTypes=nil / restoreBaseStats.
	Transformed   bool         `json:"-"`
	SavedMoves    []BattleMove `json:"-"`
	SavedImageURL string       `json:"-"`
	SavedStages   StatStages   `json:"-"`
	// Урон, полученный за ТЕКУЩИЙ ход, по категориям — для Контрудара (counter_moves.go).
	// Сбрасывается в начале processTurn; накапливается в executeDamagingMove.
	DamageTakenPhys int `json:"-"`
	DamageTakenSpec int `json:"-"`
	// Выжидание (counter_moves.go): копит ВЕСЬ полученный урон BideTurns ходов, затем
	// возвращает ×2. BideDamage НЕ сбрасывается по ходам (копится через раунды).
	BideTurns  int  `json:"-"`
	BideDamage int  `json:"-"`
	MagicCoat  bool `json:"-"` // Магический плащ: отражает статусные ходы противника (текущий ход)
	// Двухходовые атаки (charge_moves.go): ChargingMoveID — id атаки, которая
	// сейчас заряжается (0 = нет); Invulnerable — покемон в неуязвимой фазе
	// (в воздухе/под землёй/под водой/недоступен).
	ChargingMoveID int  `json:"charging_move_id,omitempty"`
	Invulnerable   bool `json:"invulnerable,omitempty"`
	SkyHeld        bool `json:"-"` // схвачен Небесным броском: неуязвим и пропускает ход
	// Перезарядка после мощной атаки (charge_moves.go): следующий ход — пропуск.
	MustRecharge bool `json:"must_recharge,omitempty"`
	// Для нарастающих атак (Эхо-Волны): id и число ПОДРЯД одинаковых ходов.
	LastMoveID int  `json:"-"`
	SameStreak int  `json:"-"`
	Raging     bool `json:"-"`
	// Запас Stockpile (Накопление/Плевок/Глоток): 0..3.
	Stockpile int `json:"stockpile,omitempty"`
	// Мультиход-лок (Ледяной шар/Танец лепестков/Шум): принудительный повтор
	// хода LockedMoveID ещё LockTurns раундов.
	LockedMoveID int `json:"-"`
	LockTurns    int `json:"-"`
	// Отложенная атака по ЭТОМУ покемону (Светопреставление/Предсказание):
	// сработает через FutureTurns раундов, нанеся заранее посчитанный FutureDamage.
	FutureMoveID  int    `json:"-"`
	FutureTurns   int    `json:"-"`
	FutureSetTurn int    `json:"-"`
	FutureDamage  int    `json:"-"`
	FutureName    string `json:"-"`
	// Огнеупорная пыльца (fire_effects.go): покрыт пыльцой — следующая ЕГО
	// огненная атака в этот ход провалится и ударит его самого (снимается в
	// конце хода). Держится только текущий ход.
	Powdered bool `json:"-"`
	// Лечащие волатили (heal_moves.go): Укоренение и Водный покров лечат 1/16
	// макс. HP в конце каждого хода; Желание лечит отложенно на следующем ходу.
	Rooted         bool `json:"-"` // Укоренение: +1/16 в конце хода; нельзя сменить
	AquaRing       bool `json:"-"` // Водный покров: +1/16 в конце хода
	WishPending    bool `json:"-"` // Желание: ждёт срабатывания
	WishSetTurn    int  `json:"-"`
	HealBlockTurns int  `json:"-"` // Запрет регенерации: нельзя восстанавливать HP N ходов
	Roosting       bool `json:"-"` // Насест: до конца хода нет Летающего типа (снимается в tickSideTimers)
	WishHeal       int  `json:"-"`
	// Защитные ходы (protect_moves.go), действуют текущий ход:
	Protected        bool  `json:"-"` // Защита/Засечка — блок любой атаки
	SpikyShield      bool  `json:"-"` // Каштановый щит — блок + 1/8 контактнику
	KingShield       bool  `json:"-"` // Королевский щит — блок + Атака-2 контактнику
	CraftyShield     bool  `json:"-"` // Волшебный щит — блок ТОЛЬКО статусных ходов
	Enduring         bool  `json:"-"` // Терпение — переживает удар с 1 HP
	MatBlocking      bool  `json:"-"` // Татами Блок — блок ТОЛЬКО урона, лишь в первый ход после выхода
	Snatching        bool  `json:"-"` // Грабёж — крадёт положительный статусный эффект противника (текущий ход)
	ProtectPrimed    bool  `json:"-"` // прошлый ход защита удалась -> шанс 50%
	protectPrimeNext bool  `json:"-"` // транзиент: удачная считаемая защита в этот ход
	ActiveTurns      int   `json:"-"` // сколько ходов покемон реально ходит на поле (для Татами Блок; 1 = первый)
	UsedMoveIDs      []int `json:"-"` // id ходов, применённых в бою (для Последнего средства)
	BattleTypes      []int `json:"-"` // перекрытие типа в бою (Адаптация/Дублирование/Камуфляж)
	LastHitByType    int   `json:"-"` // тип последней атаки ПО этому покемону (для Адаптации-2)
	// Блок/форс ходов (block_moves.go). *MoveID отдаются на фронт для грейаута.
	LastUsedMoveID    int   `json:"last_used_move_id,omitempty"`
	EncoreMoveID      int   `json:"encore_move_id,omitempty"` // Овации: форсить этот ход
	EncoreTurns       int   `json:"-"`
	DisabledMoveID    int   `json:"disabled_move_id,omitempty"` // Запрет: этот ход нельзя
	DisableTurns      int   `json:"-"`
	Tormented         bool  `json:"tormented,omitempty"`           // Подстрекательство: нельзя повтор LastUsedMoveID
	TauntTurns        int   `json:"taunt_turns,omitempty"`         // Насмешка: нельзя статусные ходы
	ImprisonedMoveIDs []int `json:"imprisoned_move_ids,omitempty"` // Потеря памяти: эти ходы запрещены
	// Подготовительные ходы (focus_moves.go):
	ChargedElectric bool       `json:"-"` // Заряд: следующая электрическая атака ×2
	SureHit         bool       `json:"-"` // Прицел/Сосредоточение: следующая атака не промахнётся
	SureCrit        bool       `json:"-"` // Ультрафокусировка: следующая атака критическая
	Foresighted     bool       `json:"-"` // Обнаружение/Чутьё: Норм/Боевой бьют этого Призрака
	MiracleEyed     bool       `json:"-"` // Третий глаз: Психика бьёт этого Тёмного
	StatStages      StatStages `json:"stat_stages"`
	// Надетый (held) предмет носителя. HeldAtkMult — плоский множитель урона при
	// совпадении типа предмета с типом покемона; дефолт 1.0 (эффект на урон пока
	// не подключён — см. HANDOFF_battle_items §4C, обсуждается отдельно).
	HeldItemID          int     `json:"held_item_id,omitempty"`
	HeldItemTypeID      int     `json:"held_item_type_id,omitempty"`
	HeldItemEffect      string  `json:"-"` // effect_type из items (burn_holder и т.п.)
	HeldAtkMult         float64 `json:"-"`
	HeldItemName        string  `json:"held_item_name,omitempty"`
	HeldItemImage       string  `json:"held_item_image,omitempty"`
	HeldItemDescription string  `json:"held_item_description,omitempty"`
	// Снимок билда (только для AI-команд): нужен, чтобы сохранить/восстановить
	// состав в npc_ai_teams без похода в user_pokemons. В бою не используется.
	NatureID int    `json:"-"`
	IV       [6]int `json:"-"`
	EV       [6]int `json:"-"`
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
	Heal              int     `json:"heal,omitempty"`          // >0 = восстановление HP (зелёная строка)
	WeatherID         int     `json:"weather_id,omitempty"`    // >0 = запись о смене погоды
	WeatherTurns      int     `json:"weather_turns,omitempty"` // сколько ходов держится
}

type BattleState struct {
	ID          string
	PlayerTeam  []BattlePokemon
	ActiveIdx   int
	WildPokemon *BattlePokemon
	// Тренерский бой (BattleType=="trainer", поток B): команда врага. WildPokemon
	// ВСЕГДА указывает на активного врага = &EnemyTeam[EnemyActiveIdx]. Срез строим
	// ОДИН раз и не append-им (иначе указатель повиснет); смена врага = только
	// переустановка индекса и указателя. Для "wild" EnemyTeam пустой.
	EnemyTeam        []BattlePokemon
	EnemyActiveIdx   int
	EnemySmart       bool   // team_mode=='smart' (поток [12]): включает ИИ-замены врага
	AILevel          int    // 0 = старый smart-AI, 1/2 = новый бот, 3/4 = задел
	NoSwitch         bool   // бенчмарк: отключить добровольные ИИ-замены
	AILastSwitchTurn int    // анти-петля: раунд последней ИИ-замены
	AILastSwitchFrom int    // индекс, с которого ушли (не возвращаться сразу)
	TrainerName      string // имя NPC-тренера (для лога «отправляет …»)
	Turn             int
	StartedAt        time.Time
	BattleType       string
	Finished         bool
	PlayerName       string
	// Протянутые IV/натура дикого (wild.go) — для сохранения «настоящего»
	// экземпляра при поимке (catch.go). Пол берётся из WildPokemon.Gender.
	WildIV            [6]int
	WildNatureID      int
	WeatherTurns      int
	BattleWeatherID   int
	BattleWeatherName string
	WeatherSetTurn    int       // раунд, в котором погоду поставили (его не считаем)
	PlayerSide        SideState // экраны/таймеры стороны игрока (screens.go)
	WildSide          SideState // экраны/таймеры стороны дикого
	WaterSportTurns   int       // Увлажнение: сила Огня ×0.33, общеполе (fire_effects.go)
	WaterSportSetTurn int
	MudSportTurns     int // Осушение: сила Электричества ×0.33, общеполе (field_effects.go)
	MudSportSetTurn   int
	GravityTurns      int // Гравитация: Летающие уязвимы к Земле, общеполе
	GravitySetTurn    int
	TerrainID         int // Арена/терраин (terrain_moves.go): 0=нет,1 Трав,2 Туман,3 Электр,4 Мент
	TerrainTurns      int // взаимоисключающие, 5 ходов
	TerrainSetTurn    int
	// Комнаты/поле (room_moves.go), независимые общеполевые таймеры 5 ходов:
	IonDelugeTurns    int // Ионное поле: Нормальные атаки -> Электрические
	IonDelugeSetTurn  int
	TrickRoomTurns    int // Комната смеха: медленные ходят первыми
	TrickRoomSetTurn  int
	WonderRoomTurns   int // Комната чудес: Защита<->СпЗащита у всех
	WonderRoomSetTurn int
}

type BattleStartRequest struct {
	LocationID int `json:"location_id"`
}

type BattleMoveRequest struct {
	BattleID string `json:"battle_id"`
	AttackID int    `json:"attack_id"`
	SwitchTo int    `json:"switch_to,omitempty"` // для Подставного хода/Тока: на кого сменить после удара
}

type BattleResponse struct {
	Type         string           `json:"type"`
	Log          []BattleLogEntry `json:"log"`
	Player       *BattlePokemon   `json:"player,omitempty"`
	Wild         *BattlePokemon   `json:"wild,omitempty"`
	Team         []BattlePokemon  `json:"team,omitempty"`
	BattleID     string           `json:"battle_id,omitempty"`
	Evolution    *EvolutionInfo   `json:"evolution,omitempty"`
	WeatherID    int              `json:"weather_id,omitempty"`
	WeatherName  string           `json:"weather_name,omitempty"`
	WeatherTurns int              `json:"weather_turns,omitempty"`
	// Тренерский бой (поток B): индикатор отряда врага и имя тренера.
	EnemyCount     int    `json:"enemy_count,omitempty"`
	EnemyRemaining int    `json:"enemy_remaining,omitempty"`
	TrainerName    string `json:"trainer_name,omitempty"`
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

const typeGhost = 14 // id призрачного типа (таблица types)

// effectCurse — EffectID атаки "Проклятье": кастомная механика (см. inflictCurse).
const effectCurse = 12

// effectMeanLook — EffectID запрета смены: кастом-обработчик inflictMeanLook
// (нельзя сменить покемона, без урона; призраки иммунны). Значение 20 — свободный
// слот effect_status_id. В БД проставлен у 387 Пристальный взгляд, 370 Блок,
// 373 Волшебные оковы, 379 Ловчая сеть (все — одна механика).
const effectMeanLook = 20

func getPokemonTypes(p *BattlePokemon) []int {
	var types []int
	// Изменённый в бою тип (Адаптация/Дублирование/Камуфляж) перекрывает природный.
	if len(p.BattleTypes) > 0 {
		types = p.BattleTypes
	} else {
		types = []int{p.PrimaryTypeID}
		if p.SecondaryTypeID != nil {
			types = append(types, *p.SecondaryTypeID)
		}
	}
	// Насест (72): до конца хода покемон теряет Летающий тип (уязвим к Земле).
	// Чисто Летающий становится БЕСТИПОВЫМ (пустой список) — все атаки бьют ×1;
	// подменять на Нормальный НЕЛЬЗЯ (иначе Боевой ошибочно дал бы ×2).
	if p.Roosting {
		filtered := make([]int, 0, len(types))
		for _, t := range types {
			if t != typeFlying {
				filtered = append(filtered, t)
			}
		}
		types = filtered // может остаться пустым — это корректно (бестиповый, ×1 на всё)
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

func calculateDamage(bs *BattleState, attacker, defender *BattlePokemon, move *BattleMove, defSide *SideState, round int, log *[]BattleLogEntry) (damage int, isCrit bool, effectiveness float64) {
	return calculateDamageCore(bs, attacker, defender, move, defSide, round, log, false)
}

// calculateDamageCore — общая формула. deterministic=true используется AI-мозгом:
// без случайного разброса и крита (кроме «всегда крит»), с пустым логом.
func calculateDamageCore(bs *BattleState, attacker, defender *BattlePokemon, move *BattleMove, defSide *SideState, round int, log *[]BattleLogEntry, deterministic bool) (damage int, isCrit bool, effectiveness float64) {
	if move.CategoryID == 3 {
		return 0, false, 1
	}
	// Формульные атаки (фикс. урон / половина HP цели) — считаются до обычной
	// формулы и в обход проверки Power==0.
	if dmg, eff, ok := formulaFixedDamage(attacker, defender, move); ok {
		return dmg, false, eff
	}
	// Эффективная мощность: Водный смерч/Извержение зависят от HP пользователя,
	// у остальных — move.Power. Атаки с нулевой мощностью урона не наносят.
	effPower := movePower(attacker, defender, move)
	if effPower == 0 {
		return 0, false, 1
	}

	// Random factor. Для AI-оценки — математическое ожидание (1.0).
	r := 1.0
	if !deterministic {
		r = 0.85 + mathrand.Float64()*0.15
	}

	// Critical hit: шанс зависит от уровня крита (базовый 1 + crit_rate атаки),
	// при максимальном счастье атакующего (255) итоговый шанс +50%.
	if deterministic {
		// Ледяное дыхание (266) — всегда критический; Ультрафокусировка — тоже.
		isCrit = move.ID == 266 || attacker.SureCrit || abilityAlwaysCrit(attacker)
	} else {
		critChance := critChanceForLevel(1 + move.CritRate + abilityCritStageExtra(attacker))
		if attacker.Happiness >= 255 {
			critChance *= 1.5
		}
		isCrit = mathrand.Float64() < critChance
		if move.ID == 266 || abilityAlwaysCrit(attacker) {
			isCrit = true
		}
		if attacker.SureCrit {
			isCrit = true
		}
	}
	// Наговор: по защищающейся стороне крит невозможен (field_effects.go) —
	// перебивает любые «гарантированные криты».
	if defSide != nil && defSide.LuckyChantTurns > 0 {
		isCrit = false
	}
	// [ability hook] Боевая броня/Крепкий панцирь: защищающийся не получает критов.
	if isCrit && abilityBlocksCrit(defender) {
		abilityLog(defender, round, log, "crit_block", "отменяет критический удар")
		isCrit = false
	}
	critMult := 1.0
	if isCrit {
		critMult = 1.5
		if attacker.AbilityKey == "sniper" { // Снайпер: крит бьёт на 225%
			critMult = 2.25
			abilityLog(attacker, round, log, "sniper", "усиливает критический удар ×2.25")
		}
	}

	// Комната чудес: Защита и Спец.Защита меняются местами (room_moves.go).
	defStage := defender.StatStages[2]
	specDefStage := defender.StatStages[5]
	if attacker.AbilityKey == "unaware" {
		defStage, specDefStage = 0, 0
	}
	physDef := effectiveStat(defender.Defense, defStage)
	specDef := effectiveStat(defender.SpecialDefense, specDefStage)
	if bs != nil && bs.WonderRoomTurns > 0 {
		physDef, specDef = specDef, physDef
	}
	// Attack / Defense selection based on category
	var a, d int
	if move.CategoryID == 1 { // physical
		atkStage := attacker.StatStages[1]
		if defender.AbilityKey == "unaware" {
			atkStage = 0
		}
		a = effectiveStat(attacker.Attack, atkStage)
		d = physDef
		if attacker.AbilityKey == "guts" && attacker.StatusID > 1 {
			// [batch6] Болевой порог: Атака ×1.5 при статусе, ожог не режет урон.
			a = int(float64(a) * 1.5)
			abilityLog(attacker, round, log, "guts", "в статусе усиливает Атаку ×1.5")
		} else {
			// Множитель атаки от статуса (ожог ×0.5) — из карточки статуса.
			a = int(float64(a) * statMultiplier(attacker.StatusID, 1))
		}
		if attacker.AbilityKey == "hustle" {
			// [batch6] Нетерпеливость: Атака ×1.5 (точность режется отдельно).
			a = int(float64(a) * 1.5)
			abilityLog(attacker, round, log, "hustle", "усиливает Атаку ×1.5")
		}
	} else { // special
		spAtkStage := attacker.StatStages[4]
		if defender.AbilityKey == "unaware" {
			spAtkStage = 0
		}
		a = effectiveStat(attacker.SpecialAttack, spAtkStage)
		if physicalHitSpecialMoves[move.ID] {
			// Психовыстрел/Психошок — спец-атака, но бьёт по физической Защите.
			d = physDef
		} else {
			d = specDef
		}
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
			if attacker.AbilityKey == "adaptability" {
				stab = 2.0
			}
			break
		}
	}

	// Type effectiveness
	defTypes := getPokemonTypes(defender)
	effectiveness = getTypeEffectiveness(move.TypeID, defTypes)
	// Сухой лёд (329): эффективность по Воде принудительно ×2 (обход обычной
	// «Лёд по Воде = ½»); прочие типы считаются штатно (formula_moves.go).
	if move.ID == 329 {
		effectiveness = freezeDryEffectiveness(defTypes)
	}
	// Обнаружение/Чутьё: снят иммунитет Призрака к Норм(1)/Боевому(7).
	if effectiveness == 0 && (defender.Foresighted || (attacker.AbilityKey == "scrappy" && (move.TypeID == 1 || move.TypeID == 7))) && (move.TypeID == 1 || move.TypeID == 7) {
		effectiveness = effectivenessIgnoringImmunity(move.TypeID, defTypes, typeGhost)
	}
	// Третий глаз: снят иммунитет Тёмного(16) к Психическому(11).
	if effectiveness == 0 && defender.MiracleEyed && move.TypeID == 11 {
		effectiveness = effectivenessIgnoringImmunity(move.TypeID, defTypes, 16)
	}
	// Земля-взаимодействия (field_effects.go). Гравитация имеет приоритет над
	// иммунитетами к Земле; иначе Магнетизм/Летающий дают иммунитет.
	if move.TypeID == typeGround {
		gravityOn := bs != nil && bs.GravityTurns > 0
		if gravityOn {
			// Летающие под Гравитацией теряют иммунитет к Земле.
			if effectiveness == 0 && pokemonHasType(defender, typeFlying) {
				effectiveness = effectivenessIgnoringImmunity(typeGround, defTypes, typeFlying)
			}
		} else if defender.MagnetRiseTurns > 0 || defender.LevitatedTurns > 0 || abilityHovering(defender, bs) {
			if abilityHovering(defender, bs) {
				abilityLog(defender, round, log, "levitate", "парит — атаки Землёй не действуют")
			}
			effectiveness = 0 // Магнетизм/Левитация (ход/способность): иммунитет к Земле
		}
	}

	// Полный иммунитет по типу (в т.ч. Левитация к Земле): урона нет. Без этого
	// нижний кламп формулы поднимал бы 0 до 1 HP.
	if effectiveness == 0 {
		return 0, false, 0
	}

	// Экран защищающейся стороны (Защитный экран / Экран света) — урон ×0.5,
	// крит пробивает (screens.go). Без экрана множитель 1.0.
	// [batch6] Преодоление барьеров игнорирует экраны.
	screenMult := 1.0
	if abilityIgnoresScreens(attacker) {
		if defSide != nil && (defSide.ReflectTurns > 0 || defSide.LightScreenTurns > 0) {
			abilityLog(attacker, round, log, "infiltrator", "проходит сквозь защитные экраны")
		}
	} else {
		screenMult = screenDamageMultiplier(defSide, move, isCrit)
	}

	// Заряд: следующая электрическая атака ×2 (focus_moves.go). Флаг гасится в
	// конце executeDamagingMove (consumeFocusOnAttack).
	chargeMult := 1.0
	if move.TypeID == 4 && attacker.ChargedElectric {
		chargeMult = 2.0
	}
	// Арена/терраин: множитель по типу (terrain_moves.go).
	terrainMult := terrainDamageMult(bs, move)
	// Типовой усилитель (held-предмет): плоский множитель урона носителя, если тип
	// ПРЕДМЕТА совпал с типом ПОКЕМОНА (pokemonHasType) — двутипный: любой из двух.
	// Тип атаки не важен (усиливает все урон-атаки); stab считается отдельно. Входит
	// в общий множитель "other" канонической формулы league17.
	heldMult := 1.0
	if attacker.AbilityKey != "klutz" && attacker.HeldAtkMult > 1.0 && attacker.HeldItemTypeID != 0 &&
		pokemonHasType(attacker, attacker.HeldItemTypeID) {
		heldMult = attacker.HeldAtkMult
	}
	// [ability hook] Множители способностей: исходящий (у атакующего) и входящий
	// (у защищающегося). Пусто => 1.0 (урон НЕ меняется — регрессия).
	abilOutMult := abilityModifyOutgoingDamage(bs, attacker, defender, move, round, log)
	abilInMult := abilityModifyIncomingDamage(bs, attacker, defender, move, round, log)
	baseDamage := (float64(2*attacker.Level+10)/250.0)*(float64(a)/float64(d))*float64(effPower) + 2
	damage = int(math.Floor(baseDamage * stab * effectiveness * critMult * screenMult * r * chargeMult * terrainMult * heldMult * abilOutMult * abilInMult))
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

func checkHit(bs *BattleState, move *BattleMove, attacker, defender *BattlePokemon, log *[]BattleLogEntry) bool {
	// Цель в неуязвимой фазе двухходовки (в воздухе/под землёй/под водой/
	// недоступна) — атака мимо, кроме «пробивателей» (Землетрясение по зарытому,
	// Воздушный апперкот по летящему).
	if defender.Invulnerable && !piercesInvuln(move.ID, defender.ChargingMoveID) {
		return false
	}
	// [batch7] Спарринг (No Guard): обе стороны всегда попадают.
	if abilityAlwaysHits(attacker, defender) {
		return true
	}
	// OHKO-атаки: особая точность (30% + разница уровней) в обход модификаторов
	// точности/уклонения; провал, если цель выше уровнем.
	if ohkoMoves[move.ID] {
		// [batch6] Здоровяк полностью блокирует OHKO-атаки.
		if defender.AbilityKey == "sturdy" && !abilityIgnoredBy(attacker) {
			abilityLog(defender, 0, log, "", "не поддаётся атаке с одного удара")
			return false
		}
		return mathrand.Intn(100) < ohkoAccuracy(attacker, defender)
	}
	if move.NeverMisses {
		return true
	}
	// Прицел/Сосредоточение: следующая атака гарантированно попадает (focus_moves.go).
	if attacker.SureHit {
		return true
	}
	// Левитация: по поднятой в воздух цели атаки всегда попадают (field_effects.go).
	if defender.LevitatedTurns > 0 {
		return true
	}
	effAcc := float64(move.Accuracy)
	if attacker.StatStages[6] != 0 {
		effAcc *= accuracyEvasionMultiplier(attacker.StatStages[6])
	}
	// [ability hook] Уклонение в погоду (Пескоструй/Белотелый): +1 ступень в
	// песчаную бурю/град. Соколиный глаз игнорирует ПОВЫШЕННОЕ уклонение цели.
	round := 0
	if bs != nil {
		round = bs.Turn
	}
	evasionStage := defender.StatStages[7]
	if bs != nil {
		if bonus := abilityEvasionBonus(defender, bs.BattleWeatherID); bonus > 0 {
			abilityLog(defender, round, log, "weather_evasion", "в этой погоде уклонение +1")
			evasionStage += bonus
		}
	}
	if evasionStage > 6 {
		evasionStage = 6
	}
	if evasionStage != 0 && !(abilityIgnoresEvasion(attacker) && evasionStage > 0) {
		effAcc /= accuracyEvasionMultiplier(evasionStage)
	} else if evasionStage > 0 && abilityIgnoresEvasion(attacker) {
		abilityLog(attacker, round, log, "keen_eye", "игнорирует повышенное уклонение цели")
	}
	// [ability hook] Множитель точности от способностей (Фасеточные глаза/Блестящая
	// кожа). Пусто => ×1.0.
	if attacker.AbilityKey == "compound_eyes" {
		abilityLog(attacker, round, log, "compound_eyes", "повышает точность атак ×1.3")
	}
	effAcc *= abilityAccuracyMult(attacker, defender, move)
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

func executeDamagingMove(bs *BattleState, attacker, defender *BattlePokemon, move *BattleMove, weatherID, round int, log *[]BattleLogEntry, defSide, atkSide *SideState, fireWeaken bool) (fainted bool, weatherChange int) {
	origMove := move // до возможных копий (погода/спорт/Электризация) — для Жалостливого взора
	// [ability hook] Формы, переключаемые ходом (Королевское построение).
	abilityOnMoveUsed(bs, attacker, move, round, log)
	// Ментальная арена: приоритетные урон-атаки (prio>0) блокируются (terrain_moves.go).
	if terrainBlocksPriority(bs, move) {
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: attacker.Name, PokemonImage: attacker.ImageURL,
			MoveName: move.Name, MoveFailed: "Ментальная арена гасит приоритетную атаку!",
		})
		return false, 0
	}
	// [batch7] Дивный шарм/Проворство блокируют приоритетные атаки.
	if defender != nil && abilityKeyIn(defender, "queenly_majesty", "dazzling") &&
		abilityMovePriority(bs, attacker, move) > 0 {
		abilityLog(defender, round, log, "", "блокирует приоритетную атаку")
		return false, 0
	}
	// Храп (343): работает только во сне; иначе провал (sleep_moves.go).
	if move.ID == moveSnore && attacker.StatusID != 5 {
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: attacker.Name, PokemonImage: attacker.ImageURL,
			MoveName: move.Name, MoveFailed: "Храп не сработал — пользователь не спит!",
		})
		return false, 0
	}
	// Огнеупорная пыльца: если атакующий покрыт пыльцой и ходит Огнём — атака
	// проваливается, он теряет 1/4 HP (fire_effects.go). До всех прочих веток.
	if firePowderBurst(attacker, move, round, log) {
		return false, 0
	}
	// Двухходовые атаки: ход зарядки не наносит урона (состояние сохранено,
	// удар произойдёт на следующем ходу). Покрывает все места вызова.
	if beginChargeIfNeeded(attacker, move, weatherID, round, log) {
		// Небесный бросок: в ход захвата ещё и хватает цель (charge_moves.go).
		if move.ID == moveSkyDrop {
			skyDropGrab(defender, round, log)
		}
		return false, 0
	}
	// Небесный бросок, ход БРОСКА: отпускаем цель перед уроном. Летающая цель
	// урона от броска не получает.
	if move.ID == moveSkyDrop {
		skyDropRelease(defender)
		if pokemonHasType(defender, typeFlying) {
			*log = append(*log, BattleLogEntry{
				Round: round, PokemonName: defender.Name, PokemonImage: defender.ImageURL,
				Message: defender.Name + " — Летающий: бросок с неба не причинил вреда.",
			})
			return false, 0
		}
	}
	// Отложенные атаки (Светопреставление/Предсказание): планируем удар через
	// 2 раунда, немедленного урона нет.
	if futureMoves[move.ID] {
		scheduleFutureAttack(attacker, defender, move, round, log)
		return false, 0
	}
	// Перезарядка: после применения такой атаки покемон пропустит следующий ход
	// (ставится даже при промахе — см. описание Взрывной волны).
	if rechargeMoves[move.ID] {
		attacker.MustRecharge = true
	}
	// Счётчик подряд одинаковых ходов (для нарастающих: Эхо-Волны).
	if attacker.LastMoveID == move.ID {
		attacker.SameStreak++
	} else {
		attacker.SameStreak = 1
	}
	attacker.LastMoveID = move.ID
	if move.ID == moveRage {
		attacker.Raging = true
	} else {
		attacker.Raging = false
	}
	// Плевок без запаса Stockpile — провал (урона нет).
	if move.ID == 293 && attacker.Stockpile == 0 {
		*log = append(*log, BattleLogEntry{
			Round:        round,
			PokemonName:  attacker.Name,
			PokemonImage: attacker.ImageURL,
			Message:      "Но запас пуст — не сработало!",
		})
		return false, 0
	}
	// Синхровзрыв бьёт только по цели, имеющей общий тип с пользователем.
	if move.ID == 324 && !sharesType(attacker, defender) {
		*log = append(*log, BattleLogEntry{
			Round:        round,
			PokemonName:  attacker.Name,
			PokemonImage: attacker.ImageURL,
			Message:      "Синхровзрыв не подействовал — нет общего типа!",
		})
		return false, 0
	}
	// Погодные правки хода (точность Грозы/Урагана/Метели, тип и мощность
	// Метеозонда). Для обычных атак вернётся тот же указатель без изменений.
	move = weatherAdjustMove(move, weatherID)
	// [batch7] Преобразование типа/контактности способностью (Pixilate, Long Reach
	// и т.п.) и смена типа носителя (Protean/Libero).
	move = abilityAdjustMove(attacker, move)
	abilityOnMoveTypeChange(attacker, move)
	// Увлажнение: сила огненных атак ×0.33, пока эффект активен (fire_effects.go).
	move = waterSportAdjust(move, fireWeaken)
	// Осушение: сила электрических атак ×0.33 (field_effects.go).
	move = mudSportAdjust(move, bs != nil && bs.MudSportTurns > 0)
	// Ионное поле: Нормальные атаки становятся Электрическими (room_moves.go).
	move = ionDelugeAdjust(bs, move)
	// Электризация: тип этой атаки становится Электрическим (field_effects.go).
	if attacker.Electrified {
		m := *move
		m.TypeID = typeElectric
		move = &m
		attacker.Electrified = false
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: attacker.Name, PokemonImage: attacker.ImageURL,
			Message: attacker.Name + ": атака наэлектризована!",
		})
	}
	// Защитные щиты (protect_moves.go): цель под Защитой/Засечкой/Каштановым/
	// Королевским — атака проваливается; контактнику прилетает наказание.
	if defenderBlocksDamage(defender) {
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: defender.Name, PokemonImage: defender.ImageURL,
			Message: defender.Name + " защитился от атаки!",
		})
		applyContactPunish(attacker, defender, move, round, log)
		if isSelfDestructMove(move.ID) {
			selfDestructFaint(attacker, round, log)
		}
		return false, 0
	}
	// Пожиратель снов (298): действует ТОЛЬКО по спящей цели (иначе провал).
	if move.ID == 298 && defender.StatusID != 5 {
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: attacker.Name, PokemonImage: attacker.ImageURL,
			MoveName: move.Name, MoveFailed: "Цель не спит — не сработало!",
		})
		return false, 0
	}
	// Подарок (204): случайный исход — лечение цели (урона нет) или случайная мощность.
	if move.ID == 204 {
		if m2, healed := applyPresentRoll(move, defender, round, log); healed {
			return false, 0
		} else {
			move = m2
		}
	}
	// Последнее средство (212): доступно, только когда все прочие ходы применялись.
	if move.ID == 212 && !lastResortReady(attacker) {
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: attacker.Name, PokemonImage: attacker.ImageURL,
			MoveName: move.Name, MoveFailed: "Ещё не все атаки в ход пошли — рано!",
		})
		return false, 0
	}
	// [batch6] Сырость (damp): блокирует саморазрушающие атаки у любой стороны.
	if isSelfDestructMove(move.ID) {
		if attacker.AbilityKey == "damp" {
			abilityLog(attacker, round, log, "", "гасит саморазрушающую атаку")
			return false, 0
		}
		if defender != nil && defender.AbilityKey == "damp" {
			abilityLog(defender, round, log, "", "гасит саморазрушающую атаку противника")
			return false, 0
		}
	}
	// [ability hook] Поглощение по типу (Внутренний огонь/Аккумулятор/Дренаж/Генератор/
	// Травоядность/Пористое тело): цель невосприимчива к типу атаки — урон не наносится,
	// применяется эффект. Пусто => false (ничего не поглощается).
	if abilityAbsorbsMove(bs, attacker, defender, move, round, log) {
		if isSelfDestructMove(move.ID) {
			selfDestructFaint(attacker, round, log)
		}
		return false, 0
	}

	hit := false
	// [batch6] Здоровяк срабатывает только если удар пришёл по ПОЛНОМУ HP.
	sturdyReady := defender != nil && defender.AbilityKey == "sturdy" &&
		defender.CurrentHP == defender.MaxHP && !abilityIgnoredBy(attacker)
	// [batch6] Грубая сила: вторичный эффект подавляется, урон уже усилен в
	// calculateDamage. Для применения эффектов берём копию без вторички.
	effectMove := move
	if attacker.AbilityKey == "sheer_force" && abilitySheerForceApplies(move) {
		effectMove = abilitySuppressSecondaryMove(move)
	}

	if move.HitsMin > 1 {
		totalDmg := 0
		hitCount := 0
		anyCrit := false
		bestEff := 1.0

		if move.ID == moveTripleKick {
			for i := 0; i < move.HitsMax && !defender.IsFainted; i++ {
				if !checkHit(bs, move, attacker, defender, log) {
					break
				}
				hitMove := *move
				hitMove.Power = tripleKickPower(move.Power, i)
				dmg, crit, eff := calculateDamage(bs, attacker, defender, &hitMove, defSide, round, log)
				if abilityBlocksFirstHit(defender, move, round, log) {
					dmg, crit, eff = 0, false, 1
				}
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
				applyFalseSwipe(defender, move)
				if applySturdy(defender, attacker, sturdyReady, round, log) {
					sturdyReady = false
				}
				absorbWithEndure(defender, round, log)
				if defender.CurrentHP <= 0 {
					defender.IsFainted = true
					fainted = true
				}
				if !defender.IsFainted {
					abilityAfterBeingHit(bs, attacker, defender, &hitMove, round, log)
					applyRageOnHit(defender, round, log)
				}
			}
		} else if checkHit(bs, move, attacker, defender, log) {
			hits := move.HitsMin
			if abilityMaxHits(attacker) {
				hits = move.HitsMax
			} else if move.HitsMax > move.HitsMin {
				switch r := mathrand.Intn(8); {
				case r < 3:
					hits = 2
				case r < 6:
					hits = 3
				case r == 6:
					hits = 4
				default:
					hits = 5
				}
				if hits < move.HitsMin {
					hits = move.HitsMin
				}
				if hits > move.HitsMax {
					hits = move.HitsMax
				}
			}
			for i := 0; i < hits && !defender.IsFainted; i++ {
				dmg, crit, eff := calculateDamage(bs, attacker, defender, move, defSide, round, log)
				if abilityBlocksFirstHit(defender, move, round, log) {
					dmg, crit, eff = 0, false, 1
				}
				totalDmg += dmg
				hitCount++
				hit = true
				if crit {
					anyCrit = true
				}
				if eff > bestEff {
					bestEff = eff
				}
				hpBefore := defender.CurrentHP
				defender.CurrentHP -= dmg
				if defender.CurrentHP < 0 {
					defender.CurrentHP = 0
				}
				applyFalseSwipe(defender, move)
				if applySturdy(defender, attacker, sturdyReady, round, log) {
					sturdyReady = false
				}
				absorbWithEndure(defender, round, log)
				if defender.CurrentHP <= 0 {
					defender.IsFainted = true
					fainted = true
				}
				if !defender.IsFainted {
					if crit {
						abilityOnCritical(defender, round, log)
					}
					abilityColorChange(defender, move)
					abilityExtraOnHit(bs, attacker, defender, move, round, log)
					abilityExtraAfterHit(bs, attacker, defender, move, round, log)
					abilityAfterBeingHit(bs, attacker, defender, move, round, log)
					applyRageOnHit(defender, round, log)
				} else {
					abilityOnFaintRevenge(attacker, defender, move, hpBefore, round, log)
					abilityExtraOnKO(bs, attacker, defender, round, log)
				}
			}
		}

		hpBeforeDrain := attacker.CurrentHP
		applyDrain(attacker, move, totalDmg, round, log)
		abilityLiquidOoze(attacker, defender, move, totalDmg, hpBeforeDrain, round, log)
		recordDamageTaken(defender, move, totalDmg) // для Контрудара (counter_moves.go)
		if hit {
			applyThief(attacker, defender, move, round, log)
		}

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
		if checkHit(bs, move, attacker, defender, log) {
			// Прорыв барьера ломает экраны цели ДО расчёта урона — свой удар не
			// ослаблен ломаемым экраном (screens.go).
			if move.ID == moveBrickBreak {
				breakScreensOn(defSide, round, log)
			}
			dmg, crit, eff := calculateDamage(bs, attacker, defender, move, defSide, round, log)
			if abilityBlocksFirstHit(defender, move, round, log) {
				dmg, crit, eff = 0, false, 1
			}
			hpBefore := defender.CurrentHP
			defender.CurrentHP -= dmg
			if defender.CurrentHP < 0 {
				defender.CurrentHP = 0
			}
			applyFalseSwipe(defender, move) // Ложный выпад: минимум 1 HP
			if applySturdy(defender, attacker, sturdyReady, round, log) {
				sturdyReady = false
			}
			absorbWithEndure(defender, round, log) // Терпение: минимум 1 HP
			hpBeforeDrain := attacker.CurrentHP
			applyDrain(attacker, move, dmg, round, log)
			abilityLiquidOoze(attacker, defender, move, dmg, hpBeforeDrain, round, log)
			recordDamageTaken(defender, move, dmg) // для Контрудара (counter_moves.go)
			applyThief(attacker, defender, move, round, log)
			fainted = defender.CurrentHP <= 0
			if fainted {
				defender.IsFainted = true
			}
			// [ability hook] Реакция носителя на ЛЮБОЙ физический удар (Хрупкая броня).
			if !defender.IsFainted {
				if crit {
					abilityOnCritical(defender, round, log)
				}
				abilityColorChange(defender, move)
				abilityExtraOnHit(bs, attacker, defender, move, round, log)
				abilityExtraAfterHit(bs, attacker, defender, move, round, log)
				abilityAfterBeingHit(bs, attacker, defender, move, round, log)
				applyRageOnHit(defender, round, log)
			} else {
				abilityOnFaintRevenge(attacker, defender, move, hpBefore, round, log)
				abilityExtraOnKO(bs, attacker, defender, round, log)
			}
			hit = true
			*log = append(*log, makeEntry(round, attacker, move, dmg, crit, eff, false))
		} else {
			*log = append(*log, makeEntry(round, attacker, move, 0, false, 1, false, true))
		}
	}

	// Вращение при попадании сметает экраны и ловушки с ОБЕИХ сторон (screens.go).
	// В 1v1 две стороны — защищающегося (defSide) и атакующего (atkSide).
	if hit && move.ID == moveRapidSpin {
		clearBothSides(defSide, atkSide, round, log)
	}
	// Тип последней атаки, попавшей по цели — для Адаптации-2 (type_moves.go).
	if hit {
		defender.LastHitByType = move.TypeID
		breakIllusion(defender, round, log) // [batch7] Иллюзия раскрывается от урона
		if defender.AbilityKey == "electromorphosis" {
			defender.ChargedElectric = true
		}
	}
	if hit && move.ID == moveSacredFire {
		thawBySacredFire(defender, round, log)
	}
	// [ability hook] Ответ при КОНТАКТЕ (Грубая кожа/Стальные шипы/Статическое
	// напряжение/Горящее тело/Ядовитая кожа/Липкая слизь/Привлекательность): носитель
	// (defender) отвечает атакующему после контактной атаки. Пусто => no-op.
	if hit && move.Contact {
		abilityOnHitByMove(bs, attacker, defender, move, round, log)
	}
	// Связь судеб: если удар отправил цель в обморок, атакующий гибнет тоже.
	if hit && fainted {
		applyDestinyBond(attacker, defender, round, log)
		applyGrudge(attacker, defender, origMove, round, log) // Жалостливый взор: обнулить PP оригинала
	}

	if hit && !fainted {
		weatherChange = applyEffectByID(bs, attacker, defender, effectMove, round, log, defSide)
		applyStatChanges(attacker, defender, effectMove, weatherID, round, log, defSide)
		// Тройная атака: случайный статус — вторичка, Грубая сила/Чешуекрылое
		// её подавляют.
		if !(attacker.AbilityKey == "sheer_force" && abilitySheerForceApplies(move)) &&
			!abilityBlocksSecondary(defender) {
			applyTriAttackEffect(defender, move, round, log)
		}
		applyClearSmog(defender, move, round, log) // Грязевая ванна: сброс бустов цели
		// Неистовый бросок (177): урон + фазинг — выгнать цель после удара
		// (handlePhazeMove проверит id и правила PvE; для не-фазинг ходов — no-op).
		handlePhazeMove(bs, attacker, defender, move, log)
	} else {
		applyStatChanges(attacker, nil, effectMove, weatherID, round, log, defSide)
	}

	// Пробуждающий удар (218) будит спящую цель после удара.
	if hit && move.ID == 218 && defender.StatusID == 5 {
		defender.StatusID = 1
		defender.SleepTurns = 0
		*log = append(*log, BattleLogEntry{
			Round:        round,
			PokemonName:  defender.Name,
			PokemonImage: defender.ImageURL,
			Message:      defender.Name + " просыпается от удара!",
		})
	}

	// Плевок израсходовал запас Stockpile (обнуление + снижение Защ/СпЗащ).
	if move.ID == 293 {
		consumeStockpile(attacker, round, log)
	}

	// Финальный ход: пользователь падает в обморок после удара.
	if hit && move.ID == 342 {
		attacker.CurrentHP = 0
		attacker.IsFainted = true
		*log = append(*log, BattleLogEntry{
			Round:        round,
			PokemonName:  attacker.Name,
			PokemonImage: attacker.ImageURL,
			Message:      attacker.Name + " жертвует собой!",
		})
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: attacker.Name, Fainted: true,
		})
	}

	if isSelfDestructMove(move.ID) {
		selfDestructFaint(attacker, round, log)
	}

	// Мультиход-лок (Ледяной шар/Танец/Шум): продлить/завершить повтор.
	manageLock(attacker, move, hit, round, log)

	// Одноразовые подготовительные флаги израсходованы этим ударом (focus_moves.go):
	// Прицел/Сосредоточение (SureHit), Ультрафокусировка (SureCrit), Заряд (×электро).
	consumeFocusOnAttack(attacker)

	return
}

func processPreTurnStatus(p *BattlePokemon, log *[]BattleLogEntry) bool {
	if p.IsFainted {
		return true
	}

	// Небесный бросок: удерживаемый в небе покемон не может действовать (charge_moves.go).
	if p.SkyHeld {
		*log = append(*log, BattleLogEntry{
			PokemonName:  p.Name,
			PokemonImage: p.ImageURL,
			Message:      p.Name + " удерживается в небе и не может действовать!",
		})
		return false
	}

	// Перезарядка после мощной атаки (Гиперлуч и т.п.): пропуск хода.
	if p.MustRecharge {
		p.MustRecharge = false
		*log = append(*log, BattleLogEntry{
			PokemonName:  p.Name,
			PokemonImage: p.ImageURL,
			Message:      p.Name + " перезаряжается и пропускает ход!",
		})
		return false
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
	// [batch7] Магический щит блокирует косвенный урон статусов.
	if def, ok := statusRegistry[p.StatusID]; ok && def.OnTurn != nil && !abilityBlocksIndirect(p) {
		delta := def.OnTurn(p) // отрицательное = урон, положительное = лечение
		// [batch5] Токсифаг: тик яда (обычного/сильного) обращается в лечение 1/8.
		// OnTurn уже сдвинул счётчик сильного яда, поэтому порядок сохранён.
		if p.AbilityKey == "poison_heal" && (p.StatusID == 6 || p.StatusID == 7) && delta < 0 {
			delta = p.MaxHP / 8
			if delta < 1 {
				delta = 1
			}
		}
		if delta != 0 {
			p.CurrentHP += delta
			if delta < 0 {
				*log = append(*log, BattleLogEntry{
					Round:       0,
					PokemonName: p.Name,
					Message:     p.Name + " получает урон от " + def.TickWord + "! -" + strconv.Itoa(-delta) + " HP",
				})
			} else if p.AbilityKey == "poison_heal" && (p.StatusID == 6 || p.StatusID == 7) {
				*log = append(*log, BattleLogEntry{
					Round:       0,
					PokemonName: p.Name,
					Message:     p.Name + " обращает яд в здоровье! +" + strconv.Itoa(delta) + " HP",
				})
			}
			if p.CurrentHP > p.MaxHP {
				p.CurrentHP = p.MaxHP
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

	// Влюблённость (volatile, Соблазн) — ~50% пропуск хода без урона. Логика в gender_moves.go.
	if !volatileInfatuation(p, log) {
		return false
	}

	return true
}

func applyEffectByID(bs *BattleState, attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry, defSide *SideState) int {
	if move.EffectID == 0 || move.EffectChance == 0 {
		return 0
	}
	if mathrand.Intn(100) >= abilityEffectChance(attacker, move.EffectChance) {
		return 0
	}
	// [batch7] Чешуекрылое: вторичные эффекты урон-атак не срабатывают.
	if defender != nil && move.CategoryID != 3 && abilityBlocksSecondary(defender) {
		abilityLog(defender, round, log, "", "чешуя блокирует дополнительный эффект")
		return 0
	}

	id := move.EffectID
	// Арена: Туманная блокирует любой мажорный статус, Электрическая — сон
	// (terrain_moves.go). Действует на цель эффекта (defender).
	if defender != nil && terrainBlocksStatus(bs, id) {
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: defender.Name, PokemonImage: defender.ImageURL,
			Message: defender.Name + " защищён ареной — статус не наложился!",
		})
		return 0
	}
	// Магический плащ (protect_moves.go): вредный статусный эффект противника
	// ОТРАЖАЕТСЯ обратно на применившего (только статусные ходы кат.3).
	if move.CategoryID == 3 && defender != nil &&
		(defender.MagicCoat || defender.AbilityKey == "magic_bounce") && isBounceableEffect(id) {
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: defender.Name, PokemonImage: defender.ImageURL,
			Message: defender.Name + " отражает ход магическим плащом!",
		})
		reflectEffectOnto(attacker, id, round, log)
		return 0
	}
	// Погода (14-17): вернуть id погоды — её ставит setBattleWeather выше по стеку.
	if w, ok := weatherEffects[id]; ok {
		return w
	}
	// Проклятье (12): уникальная механика по типу применяющего — кастом-обработчик.
	if id == effectCurse {
		inflictCurse(attacker, defender, round, log)
		return 0
	}
	// Пристальный взгляд (387): запрет смены цели без урона; призраки иммунны.
	if id == effectMeanLook {
		inflictMeanLook(defender, round, log)
		return 0
	}
	// Защитные щиты (protect_moves.go): цель под щитом (в т.ч. Волшебным) —
	// статусный эффект противника не накладывается. Погода/Проклятье выше уже
	// отработали и под защиту не попадают.
	if defender != nil && defenderBlocksStatus(defender) {
		*log = append(*log, BattleLogEntry{
			Round:        round,
			PokemonName:  defender.Name,
			PokemonImage: defender.ImageURL,
			Message:      defender.Name + " защитился — статус не подействовал!",
		})
		return 0
	}
	// Ограждение (screens.go): сторона защищена от статусов противника —
	// основные статусы и замешательство не накладываются (погода/Проклятье выше
	// уже отработали и под защиту не попадают).
	if defSide != nil && defSide.SafeguardTurns > 0 && safeguardBlocks(id) {
		if abilityIgnoresScreens(attacker) { // [batch6] Преодоление барьеров
			abilityLog(attacker, round, log, "infiltrator", "проходит сквозь Ограждение")
		} else {
			*log = append(*log, BattleLogEntry{
				Round:        round,
				PokemonName:  defender.Name,
				PokemonImage: defender.ImageURL,
				Message:      defender.Name + " защищён Ограждением — статус не подействовал!",
			})
			return 0
		}
	}
	// [batch7] Сталь/Яд иммунны к отравлению; Коррозия это обходит.
	if defender != nil && (id == 6 || id == 7) && (attacker == nil || attacker.AbilityKey != "corrosion") {
		if pokemonHasType(defender, typeSteel) || pokemonHasType(defender, typePoison) {
			*log = append(*log, BattleLogEntry{
				Round: round, PokemonName: defender.Name, PokemonImage: defender.ImageURL,
				Message: defender.Name + " иммунен к отравлению!",
			})
			return 0
		}
	}
	// [ability hook] Иммунитет к статусу от способности (напр. Immunity/Limber).
	// Пусто => false (ничего не блокирует — регрессия). Рядом со щитами/Ограждением:
	// погода/Проклятье/Пристальный взгляд выше уже отработали и сюда не попадают.
	if defender != nil && abilityBlocksStatus(bs, defender, id) {
		statusName := statusRegistry[id].Name
		if statusName == "" {
			statusName = "статус"
		}
		abilityLog(defender, round, log, "status_block_"+strconv.Itoa(id), "защищает от статуса: "+statusName)
		return 0
	}
	// Основной статус (2-7): наложение через карточку статуса.
	if _, ok := statusRegistry[id]; ok {
		applied := inflictMajorStatus(defender, id, round, log)
		if applied && defender != nil {
			// [batch7] Синхронизация: статус возвращается атакующему.
			if defender.AbilityKey == "synchronize" && attacker != nil && attacker != defender {
				if !abilityBlocksStatus(bs, attacker, id) {
					inflictMajorStatus(attacker, id, round, log)
				}
			}
			// [batch7] Ядовитый кукловод: отравленная цель сбивается с толку.
			if attacker != nil && attacker.AbilityKey == "poison_puppeteer" && (id == 6 || id == 7) {
				inflictConfusion(defender, round, log)
			}
		}
		return 0
	}
	// Volatile-статус (8,10,11,13): наложение через реестр volatiles.
	if fn, ok := inflictVolatile[id]; ok {
		wasTrapped := defender != nil && defender.Trapped
		fn(defender, round, log)
		if !wasTrapped && defender != nil && id == moveTrapStatus &&
			(move.ID == moveBind || move.ID == moveWrap) {
			defender.TrapTurns = mathrand.Intn(4) + 2
		}
		return 0
	}
	return 0
}

func setStatus(p *BattlePokemon, statusID int, round int, log *[]BattleLogEntry) {
	p.StatusID = statusID
	*log = append(*log, BattleLogEntry{
		Round:        round,
		PokemonName:  p.Name,
		PokemonImage: p.ImageURL,
		StatusEvent:  statusRegistry[statusID].Name,
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
	bs.WeatherSetTurn = bs.Turn
	bs.BattleWeatherName = weatherNames[weatherID]
	*log = append(*log, BattleLogEntry{
		Round:        bs.Turn,
		Message:      "Погода изменилась: " + weatherNames[weatherID],
		WeatherID:    weatherID,
		WeatherTurns: bs.WeatherTurns,
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
	if move.Drain < 0 && (abilityBlocksIndirect(attacker) || attacker.AbilityKey == "rock_head") {
		return
	}
	attacker.CurrentHP += drainHP
	if move.Drain < 0 {
		if attacker.CurrentHP < 0 {
			attacker.CurrentHP = 0
		}
		if attacker.AbilityKey == "wind_power" {
			attacker.ChargedElectric = true
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
			Round:        turn,
			PokemonName:  attacker.Name,
			PokemonImage: attacker.ImageURL,
			Heal:         drainHP,
		})
	}
}

// applyHeal — мгновенное лечение носителя атаки на % от MaxHP (напр. Восстановление).
// Не путать с applyDrain (вампиризм от урона): здесь лечение без нанесения урона.
func applyHeal(p *BattlePokemon, snatcher *BattlePokemon, move *BattleMove, weatherID, round int, log *[]BattleLogEntry) {
	// Погодо-зависимые лечащие атаки (Фотосинтез/Лунный свет/Рассветный луч/
	// Укрепление) задают процент через погоду; обычные — через move.HealPercent.
	healPct := move.HealPercent
	if wp, ok := weatherHealPercent(move.ID, weatherID); ok {
		healPct = wp
	}
	if healPct <= 0 {
		return
	}
	// Грабёж: %-лечение противника крадёт грабитель — лечится он вместо инициатора.
	if snatcher != nil && snatcher.Snatching && move.CategoryID == 3 {
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: snatcher.Name, PokemonImage: snatcher.ImageURL,
			Message: snatcher.Name + " крадёт лечение противника грабежом!",
		})
		p = snatcher
	}
	if p.IsFainted {
		return
	}
	if isHealBlocked(p, round, log) { // Запрет регенерации (heal_moves.go)
		return
	}
	if p.CurrentHP >= p.MaxHP {
		*log = append(*log, BattleLogEntry{
			Round:       round,
			PokemonName: p.Name,
			Message:     "HP " + p.Name + " уже полны!",
		})
		return
	}
	heal := p.MaxHP * healPct / 100
	if heal < 1 {
		heal = 1
	}
	p.CurrentHP += heal
	if p.CurrentHP > p.MaxHP {
		p.CurrentHP = p.MaxHP
	}
	*log = append(*log, BattleLogEntry{
		Round:        round,
		PokemonName:  p.Name,
		PokemonImage: p.ImageURL,
		Heal:         heal,
	})
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

func applyStatChanges(attacker, defender *BattlePokemon, move *BattleMove, weatherID, round int, log *[]BattleLogEntry, defSide *SideState) {
	if len(move.StatChanges) == 0 {
		return
	}
	// Суммируем изменения по (цель, стат): базовая строка и бонус за погоду
	// (напр. Отростки: +1/+1 базово и ещё +1/+1 в солнце) должны дать ОДНУ
	// строку лога на стат с итоговой величиной, а не по строке на запись.
	type statKey struct {
		target *BattlePokemon
		stat   int
	}
	var order []statKey
	sums := map[statKey]int{}
	names := map[statKey]string{}
	magicCoatStatLogged := false // Магический плащ: лог отражения — один раз (protect_moves.go)
	snatchLogged := false        // Грабёж: лог кражи усиления — один раз (protect_moves.go)
	// Вторичный эффект хода — ОДИН бросок на весь ход, а не по строке. Иначе атаки
	// «10% ко ВСЕМ статам» (Древняя сила 62 / Зловещий ветер 253 / Серебряный ветер
	// 319) катили бы каждый стат независимо и апали лишь часть. Общий бросок на
	// каждое значение chance: строки с одинаковым шансом срабатывают/проваливаются
	// вместе. Строки @100 всегда проходят (roll в [0,99] < 100).
	chanceRolls := map[int]int{}
	for _, sc := range move.StatChanges {
		// Условие погоды: 0 = любая; иначе строка срабатывает только при нужной погоде.
		if sc.ReqWeather != 0 && weatherID != sc.ReqWeather {
			continue
		}
		// [batch7] Изящество удваивает шанс вторичного эффекта.
		chance := sc.Chance
		if attacker != nil {
			chance = abilityEffectChance(attacker, chance)
		}
		// [batch7] Чешуекрылое блокирует вторичные изменения статов.
		if defender != nil && defender.AbilityKey == "shield_dust" &&
			move.CategoryID != 3 && sc.Chance < 100 {
			continue
		}
		roll, ok := chanceRolls[chance]
		if !ok {
			roll = mathrand.Intn(100)
			chanceRolls[chance] = roll
		}
		if roll >= chance {
			continue
		}
		var target *BattlePokemon
		var targetName string
		if sc.Target == 1 {
			// Грабёж: положительное САМО-усиление противника крадёт грабитель
			// (защищающийся под флагом Snatching) — уходит на него (protect_moves.go).
			if move.CategoryID == 3 && sc.Stages > 0 && defender != nil && defender.Snatching {
				if !snatchLogged {
					*log = append(*log, BattleLogEntry{
						Round: round, PokemonName: defender.Name, PokemonImage: defender.ImageURL,
						Message: defender.Name + " крадёт усиление противника грабежом!",
					})
					snatchLogged = true
				}
				target = defender
				targetName = defender.Name
			} else {
				target = attacker
				targetName = attacker.Name
			}
		} else {
			if defender == nil {
				continue
			}
			// Магический плащ: снижение статов статусным ходом отражается на каста
			// (protect_moves.go). Только для статусных ходов кат.3.
			if move.CategoryID == 3 && (defender.MagicCoat || defender.AbilityKey == "magic_bounce") {
				if !magicCoatStatLogged {
					*log = append(*log, BattleLogEntry{
						Round: round, PokemonName: defender.Name, PokemonImage: defender.ImageURL,
						Message: defender.Name + " отражает ход магическим плащом!",
					})
					magicCoatStatLogged = true
				}
				target = attacker
				targetName = attacker.Name
			} else {
				target = defender
				targetName = defender.Name
			}
		}
		k := statKey{target, sc.StatID}
		if _, ok := sums[k]; !ok {
			order = append(order, k)
			names[k] = targetName
		}
		sums[k] += sc.Stages
	}
	shieldLogged := false
	for _, k := range order {
		stages := sums[k]
		if stages == 0 {
			continue
		}
		// Защитные щиты (protect_moves.go): цель под щитом — изменения её статов
		// противником не проходят (блокируется весь ход). Свои бусты не трогаем.
		if defender != nil && k.target == defender && defenderBlocksStatus(defender) {
			if !shieldLogged {
				*log = append(*log, BattleLogEntry{
					Round:      round,
					StatChange: defender.Name + " защитился — статы не тронуты!",
				})
				shieldLogged = true
			}
			continue
		}
		// [ability hook] Иммунитет к падению статов (Чистое тело/Белый дым): снижение
		// статов ПРОТИВНИКОМ не проходит. Само-снижения (отдача) не трогаем — гейт
		// только по цели-защитнику с отрицательным изменением.
		if k.target == defender && stages < 0 && abilityBlocksStatDrop(defender, k.stat) {
			statName := statChangeNames[k.stat]
			if statName == "" {
				statName = "Стат"
			}
			abilityLog(defender, round, log, "statdrop_block_"+strconv.Itoa(k.stat), "защищает стат от снижения: "+statName)
			continue
		}
		// Туман (screens.go): сторона защищена от СНИЖЕНИЯ статов противником.
		// Блокируем только отрицательные изменения по цели-защитнику (свои
		// бусты и само-снижения от отдачи не трогаем).
		if k.target == defender && stages < 0 && defSide != nil && defSide.MistTurns > 0 && !abilityIgnoresScreens(attacker) {
			statName := statChangeNames[k.stat]
			if statName == "" {
				statName = "Стат"
			}
			*log = append(*log, BattleLogEntry{
				Round:      round,
				StatChange: statName + " " + names[k] + " защищена Туманом!",
			})
			continue
		}
		target := k.target
		var skip bool
		stages, target, skip = abilityStatReact(attacker, defender, target, k.stat, stages, round, log, defSide)
		if skip || target == nil || stages == 0 {
			continue
		}
		names[k] = target.Name
		target.StatStages[k.stat] += stages
		if target.StatStages[k.stat] > 6 {
			target.StatStages[k.stat] = 6
		} else if target.StatStages[k.stat] < -6 {
			target.StatStages[k.stat] = -6
		}
		statName := statChangeNames[k.stat]
		if statName == "" {
			statName = "Стат"
		}
		var verb string
		switch {
		case stages >= 2:
			verb = "резко повышена"
		case stages == 1:
			verb = "повышена"
		case stages == -1:
			verb = "снижена"
		default: // stages <= -2
			verb = "резко снижена"
		}
		msg := statName + " " + names[k] + " " + verb + "!"
		*log = append(*log, BattleLogEntry{
			Round:      round,
			StatChange: msg,
		})
	}
}

func effectiveSpeed(p *BattlePokemon) int {
	s := p.Speed
	// Множитель скорости от статуса (паралич ×0.5) — из карточки статуса.
	// [batch7] Быстрые ноги игнорируют снижение Скорости от паралича.
	if !(p.AbilityKey == "quick_feet" && p.StatusID == 2) {
		s = int(float64(s) * statMultiplier(p.StatusID, 3))
	}
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
	return statusRegistry[statusID].SkipMessage
}

func statStageMultiplier(stage int) float64 {
	if stage >= 0 {
		return float64(2+stage) / 2.0
	}
	return 2.0 / float64(2-stage)
}

func processTurn(bs *BattleState, playerMove *BattleMove, switchTo int) []BattleLogEntry {
	return processTurnInternal(bs, playerMove, switchTo, false)
}

// processTurnWithItem — ход, в котором игрок потратил действие на предмет.
// Предмет уже применён вызывающим кодом; здесь только тикает раунд, статусы и
// отвечает враг. playerMove нужен лишь для безопасных обращений в общей логике.
func processTurnWithItem(bs *BattleState) []BattleLogEntry {
	return processTurnInternal(bs, &BattleMove{}, 0, true)
}

func processTurnInternal(bs *BattleState, playerMove *BattleMove, switchTo int, itemUsed bool) []BattleLogEntry {
	bs.Turn++
	log := []BattleLogEntry{}

	player := &bs.PlayerTeam[bs.ActiveIdx]
	wild := bs.WildPokemon
	defer func() {
		if bs.ActiveIdx >= 0 && bs.ActiveIdx < len(bs.PlayerTeam) {
			bs.PlayerTeam[bs.ActiveIdx].Flinched = false
		}
		if bs.WildPokemon != nil {
			bs.WildPokemon.Flinched = false
		}
	}()

	if player.IsFainted || wild.IsFainted {
		return log
	}

	// ИИ-замена умного тренера (поток [12], замечание 3): если активный враг не
	// пробивает игрока, а на скамейке есть выгодный — враг ТРАТИТ ход на смену
	// (заходит новый + ловушки), а игрок бьёт уже по нему. Только BattleType=trainer
	// и EnemySmart, чтобы не менять поведение random/fixed-тренеров и дикого боя.
	enemySwitched := false
	if bs.BattleType == "trainer" && bs.EnemySmart && !bs.NoSwitch {
		if newIdx := chooseEnemySwitch(bs, player, wild); newIdx >= 0 {
			// Через хелпер: здесь локальная переменная log ([]BattleLogEntry)
			// перекрывает пакет log, напрямую log.Printf вызвать нельзя.
			logSmartSwitch(wild.Name, bs.EnemyTeam[newIdx].Name, player.Name)
			bs.AILastSwitchTurn = bs.Turn
			bs.AILastSwitchFrom = bs.EnemyActiveIdx
			abilityOnSwitchOut(wild, bs.Turn, &log) // [batch6] Регенератор врага
			wild.ActiveTurns = 0                    // уходящий сбрасывает счётчик поля
			bs.EnemyActiveIdx = newIdx
			bs.WildPokemon = &bs.EnemyTeam[newIdx]
			wild = bs.WildPokemon
			log = append(log, BattleLogEntry{
				Round: bs.Turn, PokemonName: wild.Name, PokemonImage: wild.ImageURL,
				Message: fmt.Sprintf("%s отправляет %s в бой!", bs.TrainerName, wild.Name),
			})
			if applyEntryHazards(bs, wild, &bs.WildSide, bs.Turn, &log) {
				log = append(log, BattleLogEntry{Round: bs.Turn, PokemonName: wild.Name, Fainted: true})
			}
			enemySwitched = true
		}
	}

	// Счётчик реальных ходов на поле (для Татами Блок): в этот ход оба ходят.
	// ActiveTurns==1 -> первый ход после выхода. Сброс — при смене (=0 у уходящего).
	player.ActiveTurns++
	wild.ActiveTurns++

	// Полученный за ход урон (для Контрудара) обнуляется в начале хода; копится
	// в executeDamagingMove, читается Контрударом (priority -5, ходит последним).
	player.DamageTakenPhys, player.DamageTakenSpec = 0, 0
	wild.DamageTakenPhys, wild.DamageTakenSpec = 0, 0
	player.MovedSecond, wild.MovedSecond = false, false

	// Страховка Небесного броска: снять «зависшее» удержание, если держащий уже
	// не заряжает бросок (срыв/смена) — charge_moves.go.
	releaseStaleSkyHold(player, wild)

	// Pre-turn status processing
	playerCanAct := processPreTurnStatus(player, &log)
	wildCanAct := processPreTurnStatus(wild, &log)

	// Предмет уже применён до вызова: игрок потратил ход, но не атакует.
	if itemUsed {
		playerCanAct = false
	}

	// Враг сменился в этот ход — он уже потратил действие на замену, атаковать
	// новым нельзя (игрок бьёт по вошедшему).
	if enemySwitched {
		wildCanAct = false
	}

	// Храп/Бормотание: покемон действует ВО СНЕ. processPreTurnStatus уже тикнул
	// сон (движение к пробуждению); если игрок всё ещё спит и выбрал такой ход —
	// разрешаем действовать. (У дикого ход выбирается позже — там не покрываем.)
	if !playerCanAct && player.StatusID == 5 && sleepActingMove(playerMove.ID) {
		playerCanAct = true
		log = append(log, BattleLogEntry{
			Round: bs.Turn, PokemonName: player.Name, PokemonImage: player.ImageURL,
			Message: player.Name + " действует во сне!",
		})
	}
	playerCanAct = sacredFireCanAct(player, playerMove, playerCanAct, bs.Turn, &log)

	// Двухходовые атаки: заряжающийся покемон обязан завершить свою атаку
	// (выбор игрока в этот ход игнорируется). Если действовать не может —
	// зарядка срывается, покемон выходит из неуязвимости.
	playerWasCharging := false
	if player.ChargingMoveID != 0 {
		if playerCanAct {
			if m := findMoveByID(player, player.ChargingMoveID); m != nil {
				playerMove = m
				playerWasCharging = true
			}
		} else {
			player.ChargingMoveID = 0
			player.Invulnerable = false
		}
	}
	if wild.ChargingMoveID != 0 && !wildCanAct {
		wild.ChargingMoveID = 0
		wild.Invulnerable = false
	}

	// Мультиход-лок (Ледяной шар/Танец/Шум): захваченный покемон обязан повторить
	// атаку; если действовать не может — лок срывается.
	if player.LockedMoveID != 0 {
		if playerCanAct {
			if m := findMoveByID(player, player.LockedMoveID); m != nil {
				playerMove = m
				playerWasCharging = true
			}
		} else {
			player.LockedMoveID = 0
			player.LockTurns = 0
		}
	}
	if wild.LockedMoveID != 0 && !wildCanAct {
		wild.LockedMoveID = 0
		wild.LockTurns = 0
	}

	// Выжидание: пока идёт ожидание, покемон форсированно продолжает Выжидание
	// (counter_moves.go). Как мультиход-лок; при невозможности действовать — срыв.
	if player.BideTurns > 0 {
		if playerCanAct {
			if m := findMoveByID(player, moveBide); m != nil {
				playerMove = m
				playerWasCharging = true
			}
		} else {
			player.BideTurns, player.BideDamage = 0, 0
		}
	}
	if wild.BideTurns > 0 && !wildCanAct {
		wild.BideTurns, wild.BideDamage = 0, 0
	}

	// Овации/Приказ: игрок форсированно повторяет EncoreMoveID (block_moves.go).
	// PP списывается штатно (не как зарядка), поэтому playerWasCharging не ставим.
	if player.EncoreTurns > 0 && player.EncoreMoveID != 0 && playerCanAct && !playerWasCharging {
		if m := findMoveByID(player, player.EncoreMoveID); m != nil {
			playerMove = m
		}
	}

	if playerMove != nil && playerMove.ID != moveRage {
		player.Raging = false
	}

	// По-ходовые volatile-эффекты (семена-пиявки, ловушка) — порядок сохранён.
	runEndTurnVolatiles(player, wild, &log)
	if player.IsFainted || wild.IsFainted {
		playerCanAct = !player.IsFainted
		wildCanAct = !wild.IsFainted && !enemySwitched
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
			move = chooseEnemyMove(bs, wild, player)
		}
		markMoveUsed(attacker, move.ID) // учёт для Последнего средства
		if move.CategoryID != 3 && move.Power > 0 {
			fainted, wc := executeDamagingMove(bs, attacker, defender, move, bs.BattleWeatherID, bs.Turn, &log, sideFor(bs, defender), sideFor(bs, attacker), bs.WaterSportTurns > 0)
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
			if move.CategoryID == 3 && !abilityAbsorbsStatusMove(bs, attacker, defender, move, bs.Turn, &log) {
				if wc := applyEffectByID(bs, attacker, defender, move, bs.Turn, &log, sideFor(bs, defender)); wc > 0 {
					setBattleWeather(bs, wc, &log)
				}
				applyStatChanges(attacker, defender, move, bs.BattleWeatherID, bs.Turn, &log, sideFor(bs, defender))
				applyHeal(attacker, defender, move, bs.BattleWeatherID, bs.Turn, &log)
				handleCustomStatusMove(bs, attacker, defender, move, bs.Turn, &log)
				applySideMove(bs, attacker, defender, move, &log)
				handlePhazeMove(bs, attacker, defender, move, &log)
				handleCallMove(bs, attacker, defender, move, bs.Turn, &log)
			}
		}
		// Подставной ход/Ток единственным ходящим: удар прошёл — меняемся (дикий
		// в этот ход не бьёт, по новому тоже).
		if playerCanAct && isSwitchMove(playerMove.ID) && switchTo > 0 && !player.IsFainted && !wild.IsFainted {
			performInBattleSwitch(bs, switchTo, &log)
		}
		return log
	}

	wildMove := chooseEnemyMove(bs, wild, player)
	wildCanAct = sacredFireCanAct(wild, wildMove, wildCanAct, bs.Turn, &log)
	if wildMove != nil && wildMove.ID != moveRage {
		wild.Raging = false
	}

	// Опережение (Me First, call_moves.go): игрок копирует урон-атаку дикого и бьёт
	// ею ПЕРВЫМ с силой ×1.5 — если быстрее и ход дикого наносит урон. Иначе провал.
	if playerCanAct && playerMove.ID == moveMeFirst {
		if meFirstOK(bs, player, wild, wildCanAct, wildMove) {
			m := *wildMove
			m.Power = m.Power * 3 / 2
			playerMove = &m
			log = append(log, BattleLogEntry{
				Round: bs.Turn, PokemonName: player.Name, PokemonImage: player.ImageURL,
				Message: player.Name + " опережает и отвечает атакой " + m.Name + "!",
			})
		} else {
			log = append(log, BattleLogEntry{Round: bs.Turn, PokemonName: player.Name, MoveFailed: "Опережение не сработало!"})
			playerCanAct = false // ход потрачен впустую
		}
	}

	// Log MoveFailed entries for pokemon blocked by status
	if !itemUsed && !playerCanAct && !player.IsFainted {
		reason := statusSkipReason(player.StatusID)
		if reason != "" {
			log = append(log, BattleLogEntry{
				Round:        bs.Turn,
				PokemonName:  player.Name,
				PokemonImage: player.ImageURL,
				MoveName:     playerMove.Name,
				MoveFailed:   reason,
			})
		}
	}
	if !wildCanAct && !wild.IsFainted {
		reason := statusSkipReason(wild.StatusID)
		if reason != "" {
			log = append(log, BattleLogEntry{
				Round:        bs.Turn,
				PokemonName:  wild.Name,
				PokemonImage: wild.ImageURL,
				MoveName:     wildMove.Name,
				MoveFailed:   reason,
			})
		}
	}

	if !playerCanAct && !wildCanAct {
		return log
	}

	// Determine turn order (only among those who can act)
	playerFirst := true
	if playerCanAct && wildCanAct {
		playerPriority := abilityMovePriority(bs, player, playerMove)
		wildPriority := abilityMovePriority(bs, wild, wildMove)
		if playerPriority != wildPriority {
			playerFirst = playerPriority > wildPriority
		} else {
			playerSpeed := tailwindSpeed(bs, player)
			wildSpeed := tailwindSpeed(bs, wild)
			playerFirst = playerSpeed > wildSpeed
			if bs.TrickRoomTurns > 0 { // Комната смеха: медленные ходят первыми
				playerFirst = playerSpeed < wildSpeed
			}
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
		// Второй не может действовать, но остаётся ЦЕЛЬЮ первой атаки:
		// без этого executeDamagingMove получал nil-defender (паника, когда
		// один спит/парализован, а другой бьёт).
		second = wild
		firstCanAct = true
		secondCanAct = false
	} else {
		first, firstMove = wild, wildMove
		second = player
		firstCanAct = true
		secondCanAct = false
	}

	if firstCanAct {
		markMoveUsed(first, firstMove.ID) // учёт для Последнего средства
	}
	// Execute first attack
	if firstCanAct && firstMove.CategoryID != 3 && firstMove.Power > 0 {
		fainted, wc := executeDamagingMove(bs, first, second, firstMove, bs.BattleWeatherID, bs.Turn, &log, sideFor(bs, second), sideFor(bs, first), bs.WaterSportTurns > 0)
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
		// Подставной ход/Ток игрока ПЕРВЫМ ходом: меняемся сразу — дикий (second)
		// в этот же ход ударит уже вышедшего покемона. Обновляем first.
		if playerFirst && isSwitchMove(firstMove.ID) && switchTo > 0 && !first.IsFainted {
			if performInBattleSwitch(bs, switchTo, &log) {
				first = &bs.PlayerTeam[bs.ActiveIdx]
			}
		}
	} else if firstCanAct {
		log = append(log, makeEntry(bs.Turn, first, firstMove, 0, false, 1, false))
		if firstMove.CategoryID == 3 && !abilityAbsorbsStatusMove(bs, first, second, firstMove, bs.Turn, &log) {
			if wc := applyEffectByID(bs, first, second, firstMove, bs.Turn, &log, sideFor(bs, second)); wc > 0 {
				setBattleWeather(bs, wc, &log)
			}
			applyStatChanges(first, second, firstMove, bs.BattleWeatherID, bs.Turn, &log, sideFor(bs, second))
			applyHeal(first, second, firstMove, bs.BattleWeatherID, bs.Turn, &log)
			handleCustomStatusMove(bs, first, second, firstMove, bs.Turn, &log)
			applySideMove(bs, first, second, firstMove, &log)
			handlePhazeMove(bs, first, second, firstMove, &log)
			handleCallMove(bs, first, second, firstMove, bs.Turn, &log)
		}
	}

	if secondCanAct && second != nil && second.Flinched {
		second.Flinched = false
		secondCanAct = false
		log = append(log, BattleLogEntry{
			Round:        bs.Turn,
			PokemonName:  second.Name,
			PokemonImage: second.ImageURL,
			Message:      second.Name + " испуган(а) и пропускает ход!",
		})
	}
	if secondCanAct && secondMove != nil {
		markMoveUsed(second, secondMove.ID) // учёт для Последнего средства
		second.MovedSecond = true
	}
	// Execute second attack if applicable
	if secondCanAct && secondMove != nil && !second.IsFainted && !first.IsFainted {
		if secondMove.CategoryID != 3 && secondMove.Power > 0 {
			fainted, wc := executeDamagingMove(bs, second, first, secondMove, bs.BattleWeatherID, bs.Turn, &log, sideFor(bs, first), sideFor(bs, second), bs.WaterSportTurns > 0)
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
			// Подставной ход/Ток игрока ВТОРЫМ ходом: дикий (first) уже отбил по
			// прежнему покемону — теперь просто меняемся, по новому дикий не бьёт.
			if !playerFirst && isSwitchMove(secondMove.ID) && switchTo > 0 && !second.IsFainted {
				performInBattleSwitch(bs, switchTo, &log)
			}
		} else {
			log = append(log, makeEntry(bs.Turn, second, secondMove, 0, false, 1, false))
			if secondMove.CategoryID == 3 && !abilityAbsorbsStatusMove(bs, second, first, secondMove, bs.Turn, &log) {
				if wc := applyEffectByID(bs, second, first, secondMove, bs.Turn, &log, sideFor(bs, first)); wc > 0 {
					setBattleWeather(bs, wc, &log)
				}
				applyStatChanges(second, first, secondMove, bs.BattleWeatherID, bs.Turn, &log, sideFor(bs, first))
				applyHeal(second, first, secondMove, bs.BattleWeatherID, bs.Turn, &log)
				handleCustomStatusMove(bs, second, first, secondMove, bs.Turn, &log)
				applySideMove(bs, second, first, secondMove, &log)
				handlePhazeMove(bs, second, first, secondMove, &log)
				handleCallMove(bs, second, first, secondMove, bs.Turn, &log)
			}
		}
	}

	// Decrement PP for player's move if it was used. На ходу-ударе двухходовки
	// PP уже списан в ход зарядки — повторно не списываем.
	if playerCanAct && !playerWasCharging && playerMove.PP > 0 {
		playerMove.PP--
	}
	// [batch6] Величие: давление цели стоит атакующему ещё 1 PP.
	if playerCanAct && abilityPressureDrain(player, wild, playerMove) {
		abilityLog(wild, bs.Turn, &log, "pressure", "давит: атакующий тратит дополнительный PP")
	}

	// Эстафета (switch_moves.go): игрок сменяет покемона с переносом бустов статов.
	// Выполняется в конце хода; если носитель пал от удара — смена не проходит.
	if switchTo > 0 && playerMove.ID == moveBatonPass && !bs.PlayerTeam[bs.ActiveIdx].IsFainted {
		handleBatonPass(bs, switchTo, &log)
	}
	// Рука помощи (switch_moves.go): +1 Атака выбранному в команде, -1 Защита себе.
	if playerCanAct && playerMove.ID == moveHelpingHand && !bs.PlayerTeam[bs.ActiveIdx].IsFainted {
		handleHelpingHand(bs, switchTo, bs.Turn, &log)
	}
	// Ароматный шлейф (switch_moves.go): +1 Спец.Защита выбранному в команде.
	if playerCanAct && playerMove.ID == moveAromaticMist && !bs.PlayerTeam[bs.ActiveIdx].IsFainted {
		handleAromaticMist(bs, switchTo, bs.Turn, &log)
	}

	return log
}

func chooseWildMove(wild *BattlePokemon) *BattleMove {
	if len(wild.Moves) == 0 {
		return &BattleMove{Name: "Нет атаки", CategoryID: 3}
	}
	// Индексы ходов, разрешённых сейчас (без Запрета/Подстрекательства — block_moves.go).
	allowed := make([]int, 0, len(wild.Moves))
	for i := range wild.Moves {
		if !moveBlockedFor(wild, wild.Moves[i].ID) {
			allowed = append(allowed, i)
		}
	}
	if len(allowed) == 0 { // всё заблокировано — берём как раньше (крайний случай)
		for i := range wild.Moves {
			allowed = append(allowed, i)
		}
	}
	// Среди разрешённых предпочитаем урон-атаки.
	var damaging []int
	for _, i := range allowed {
		if wild.Moves[i].CategoryID != 3 && wild.Moves[i].Power > 0 {
			damaging = append(damaging, i)
		}
	}
	if len(damaging) > 0 {
		return &wild.Moves[damaging[mathrand.Intn(len(damaging))]]
	}
	return &wild.Moves[allowed[mathrand.Intn(len(allowed))]]
}

// ======================== Create Player Battle Pokemon ========================

func playerPokemonToBattle(userPokemonID int) *BattlePokemon {
	row := db.DB.QueryRow(`
		SELECT up.id, up.pokemon_id, p.name, up.level, up.current_hp, up.max_hp,
		       up.attack, up.defense, up.speed, up.special_attack, up.special_defense,
		       p.image_url, p.primary_type_id, p.secondary_type_id, up.status_id,
		       COALESCE(up.is_starter, 0),
		       COALESCE(up.happiness, 0), COALESCE(p.weight, 0), COALESCE(up.gender, ''),
		       up.held_item_id, COALESCE(hi.effect_type, ''), COALESCE(hi.effect_value, 0),
		       COALESCE(hi.type_id, 0), COALESCE(hi.name, ''), COALESCE(hi.image_url, ''),
		       COALESCE(hi.description, '')
		FROM user_pokemons up
		JOIN pokemons p ON up.pokemon_id = p.id
		LEFT JOIN items hi ON up.held_item_id = hi.id
		WHERE up.id = ?
	`, userPokemonID)

	var bp BattlePokemon
	var imageURL string
	var secType sql.NullInt64
	var heldID sql.NullInt64
	var heldEffectType, heldName, heldImage, heldDesc string
	var heldEffectValue, heldTypeID, isStarter int
	err := row.Scan(&bp.ID, &bp.PokemonID, &bp.Name, &bp.Level,
		&bp.CurrentHP, &bp.MaxHP, &bp.Attack, &bp.Defense, &bp.Speed,
		&bp.SpecialAttack, &bp.SpecialDefense,
		&imageURL, &bp.PrimaryTypeID, &secType, &bp.StatusID, &isStarter,
		&bp.Happiness, &bp.Weight, &bp.Gender,
		&heldID, &heldEffectType, &heldEffectValue, &heldTypeID, &heldName, &heldImage, &heldDesc)
	if err != nil {
		return nil
	}
	bp.IsStarter = isStarter != 0
	// Надетый предмет: множитель по умолчанию 1.0.
	bp.HeldAtkMult = 1.0
	if heldID.Valid {
		bp.HeldItemID = int(heldID.Int64)
		bp.HeldItemName = heldName
		bp.HeldItemImage = heldImage
		bp.HeldItemDescription = heldDesc
		bp.HeldItemEffect = heldEffectType
		if heldEffectType == "type_boost" && heldEffectValue > 0 {
			bp.HeldItemTypeID = heldTypeID
			bp.HeldAtkMult = float64(heldEffectValue) / 100.0
		}
	}
	// Страховка на старые/битые данные: пустой пол трактуем как бесполый.
	if bp.Gender != db.GenderMale && bp.Gender != db.GenderFemale {
		bp.Gender = db.GenderNeutral
	}
	bp.ImageURL = imageURL
	snapshotBaseStats(&bp) // для восстановления статов при смене (stat_swap_moves.go)
	if secType.Valid {
		st := int(secType.Int64)
		bp.SecondaryTypeID = &st
	}

	if bp.CurrentHP <= 0 {
		bp.IsFainted = true
	}

	// [ability] Способность экземпляра -> AbilityKey/Name для диспетчера боя.
	var abilityID sql.NullInt64
	db.DB.QueryRow("SELECT ability_id FROM user_pokemons WHERE id = ?", bp.ID).Scan(&abilityID)
	if abilityID.Valid {
		bp.AbilityID = int(abilityID.Int64)
		bp.AbilityName, bp.AbilityKey = db.AbilityNameKey(bp.AbilityID)
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
		       COALESCE(a.disabled, 0), COALESCE(a.heal_percent, 0), COALESCE(a.contact, 0),
		       COALESCE(a.sound, 0)
		FROM user_pokemon_learnable_attacks ula
		JOIN attacks a ON ula.attack_id = a.id
		WHERE ula.user_pokemon_id = ? AND ula.is_learned = 1 AND ula.position IS NOT NULL
		ORDER BY ula.position
	`, bp.ID)
	if err == nil {
		defer moveRows.Close()
		for moveRows.Next() {
			var m BattleMove
			var neverMisses, disabled, contact, sound int
			if moveRows.Scan(&m.ID, &m.Name, &m.EngName, &m.TypeID, &m.CategoryID, &m.Power, &m.Accuracy, &m.PP, &m.MaxPP, &m.Priority, &m.TargetID, &m.Description, &m.EffectID, &m.EffectChance, &neverMisses, &m.Drain, &m.CritRate, &m.HitsMin, &m.HitsMax, &disabled, &m.HealPercent, &contact, &sound) == nil {
				m.NeverMisses = neverMisses != 0
				m.Disabled = disabled != 0
				m.Contact = contact != 0
				m.Sound = sound != 0
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
			ID:           m.ID,
			Name:         m.Name,
			EngName:      m.EngName,
			TypeID:       m.TypeID,
			CategoryID:   m.CategoryID,
			Power:        m.Power,
			Accuracy:     m.Accuracy,
			PP:           -1, // infinite PP
			MaxPP:        -1,
			Priority:     m.Priority,
			TargetID:     m.TargetID,
			Description:  m.Description,
			EffectID:     m.EffectID,
			EffectChance: m.EffectChance,
			NeverMisses:  m.NeverMisses,
			Disabled:     m.Disabled,
			Drain:        m.Drain,
			HealPercent:  m.HealPercent,
			CritRate:     m.CritRate,
			HitsMin:      m.HitsMin,
			HitsMax:      m.HitsMax,
			Contact:      m.Contact,
			Sound:        m.Sound,
			StatChanges:  loadStatChanges(m.ID),
		}
	}

	// Пол дикого разыгрываем тем же хелпером по его виду (одна точка правды).
	gender, err := db.RollGender(w.PokemonID)
	if err != nil {
		gender = db.GenderNeutral
	}

	bp := &BattlePokemon{
		ID:                 0,
		PokemonID:          w.PokemonID,
		Name:               w.Name,
		Level:              w.Level,
		CurrentHP:          w.MaxHP,
		MaxHP:              w.MaxHP,
		Gender:             gender,
		Attack:             w.Attack,
		Defense:            w.Defense,
		Speed:              w.Speed,
		SpecialAttack:      w.SpecialAttack,
		SpecialDefense:     w.SpecialDefense,
		BaseAttack:         w.Attack,
		BaseDefense:        w.Defense,
		BaseSpeed:          w.Speed,
		BaseSpecialAttack:  w.SpecialAttack,
		BaseSpecialDefense: w.SpecialDefense,
		PrimaryTypeID:      w.PrimaryTypeID,
		SecondaryTypeID:    w.SecondaryTypeID,
		ImageURL:           w.ImageURL,
		StatusID:           0,
		Weight:             w.Weight,
		Moves:              moves,
		IsFainted:          false,
		HeldAtkMult:        1.0, // дикие предметов не носят; дефолт 1.0 (безопасно к урону)
	}

	// [ability] Транзиентная СЛУЧАЙНАЯ базовая способность соперника (только на бой,
	// в БД не пишется). Одна точка — покрывает и дикого, и тренерского бойца
	// (buildNPCTeam строит их через эту же функцию). Так эффекты сработают у обоих.
	if abilityID, _, ok := db.RollBasicAbility(w.PokemonID); ok {
		bp.AbilityID = abilityID
		bp.AbilityName, bp.AbilityKey = db.AbilityNameKey(abilityID)
	}

	return bp
}

// ======================== HTTP Handlers ========================

// buildPlayerTeam собирает боевую команду игрока из user_pokemons (позиции 1..6).
// Возвращает команду, индекс первого «в сознании» и ошибку с готовым текстом.
// Единая точка правды: зовётся и диким стартом, и тренерским (поток B).
func buildPlayerTeam(userID int) ([]BattlePokemon, int, error) {
	rows, err := db.DB.Query(`
		SELECT id FROM user_pokemons
		WHERE user_id = ? AND position BETWEEN 1 AND 6
		ORDER BY CASE WHEN COALESCE(is_starter, 0) = 1 THEN 0 ELSE 1 END, position
	`, userID)
	if err != nil {
		return nil, -1, fmt.Errorf("DB error")
	}
	defer rows.Close()

	var teamIDs []int
	for rows.Next() {
		var id int
		rows.Scan(&id)
		teamIDs = append(teamIDs, id)
	}
	if len(teamIDs) == 0 {
		return nil, -1, fmt.Errorf("Нет покемонов в команде")
	}

	var playerTeam []BattlePokemon
	firstAlive := -1
	for _, id := range teamIDs {
		p := playerPokemonToBattle(id)
		if p == nil {
			continue
		}
		if firstAlive < 0 && !p.IsFainted {
			firstAlive = len(playerTeam)
		}
		playerTeam = append(playerTeam, *p)
	}
	if len(playerTeam) == 0 {
		return nil, -1, fmt.Errorf("Нет покемонов в команде")
	}
	if firstAlive < 0 {
		return nil, -1, fmt.Errorf("Все покемоны в команде без сознания")
	}
	return playerTeam, firstAlive, nil
}

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

	playerTeam, firstAlive, err := buildPlayerTeam(userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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
		ID:           battleID,
		PlayerTeam:   playerTeam,
		ActiveIdx:    firstAlive,
		WildPokemon:  wildPoke,
		Turn:         0,
		StartedAt:    time.Now(),
		BattleType:   "wild",
		PlayerName:   playerName,
		WildIV:       wildGen.IV,
		WildNatureID: wildGen.NatureID,
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

	// [ability hook] Способности ЛИДОВ при старте боя (лиды не идут через
	// applyEntryHazards). Пусто => no-op; демо-ветка logtest пишет в лог старта.
	var startLog []BattleLogEntry
	abilityOnSwitchIn(bs, playerPoke, &startLog)
	abilityOnSwitchIn(bs, wildPoke, &startLog)

	resp := BattleResponse{
		Type:         "start",
		Log:          startLog,
		BattleID:     battleID,
		Player:       playerPoke,
		Wild:         wildPoke,
		WeatherID:    weatherID,
		WeatherName:  weatherName,
		WeatherTurns: weatherTurns,
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
			Type:         respType,
			Log:          []BattleLogEntry{{PokemonName: player.Name, Fainted: true, Message: player.Name + " без сознания!"}},
			Player:       copyBattlePokemon(player),
			Wild:         copyBattlePokemon(bs.WildPokemon),
			WeatherID:    bs.BattleWeatherID,
			WeatherName:  bs.BattleWeatherName,
			WeatherTurns: bs.WeatherTurns,
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

	// Блок/форс ходов (block_moves.go): страховка на случай обхода грейаута.
	if moveBlockedFor(player, playerMove.ID) {
		http.Error(w, "Этот ход сейчас недоступен", http.StatusBadRequest)
		return
	}

	// Телепорт (switch_moves.go): игрок покидает бой с диким — завершаем «в ничью».
	if playerMove.ID == moveTeleport {
		battleMu.Lock()
		saveBattleState(bs)
		bs.Finished = true
		delete(battleSessions, req.BattleID)
		battleMu.Unlock()
		resp := BattleResponse{
			Type: "run_success",
			Log: []BattleLogEntry{{
				Round: bs.Turn, PokemonName: player.Name, PokemonImage: player.ImageURL,
				Message: player.Name + " телепортируется прочь — бой окончен.",
			}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	// ИИ-замена врага происходит ВНУТРИ processTurn; запоминаем индекс до хода,
	// чтобы отличить добровольную смену (и перерисовать карточку врага на клиенте).
	prevEnemyIdx := bs.EnemyActiveIdx
	log := processTurn(bs, playerMove, req.SwitchTo)
	enemySwitchedByAI := bs.EnemyActiveIdx != prevEnemyIdx

	resp := finishBattleTurn(bs, req.BattleID, log, enemySwitchedByAI)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// finishBattleTurn — общий конец хода для атаки/смены/предмета: погода, экраны,
// способности конца хода, победа/поражение/замена врага и сборка ответа.
func finishBattleTurn(bs *BattleState, battleID string, log []BattleLogEntry, enemySwitchedByAI bool) *BattleResponse {
	// Подставной ход/Ток мог сменить активного покемона внутри хода — перечитываем.
	player := &bs.PlayerTeam[bs.ActiveIdx]

	// Урон погоды (град/буря) в конце хода — до postProcessWeather, пока
	// погода ещё активна; обморок подхватит логика победы/поражения ниже.
	applyWeatherChip(bs, player, bs.WildPokemon, &log)

	// Созревшие отложенные атаки (Светопреставление/Предсказание).
	applyFutureAttacks(bs, player, bs.WildPokemon, &log)

	postProcessWeather(bs, &log)

	// Отсчёт таймеров стороны поля (экраны) — конец хода (screens.go).
	tickSideTimers(bs, &log)

	// [ability hook] Способности «конец хода» (Форсаж/Дождефаг/Ледяное тело/Змеиная
	// кожа) для обоих активных. Пусто => no-op.
	abilityOnEndTurn(bs, player, bs.Turn, &log)
	abilityOnEndTurn(bs, bs.WildPokemon, bs.Turn, &log)
	heldItemOnEndTurn(bs, player, bs.Turn, &log)
	heldItemOnEndTurn(bs, bs.WildPokemon, bs.Turn, &log)

	// Determine response type.
	// handleEnemyFaint: для "wild" враг пал = конец (старое поведение); для
	// "trainer" — выставляет следующего покемона NPC (switched) или сообщает,
	// что вся команда врага пала (allEnemyDown).
	respType := "turn"
	var evoInfo *EvolutionInfo
	// ВАЖНО: enemySwitched = ИИ-замена внутри хода ИЛИ смена после обморока.
	// Иначе добровольная замена терялась (враг не пал), клиент получал "turn"
	// и не перерисовывал карточку: оставались старая картинка/имя/уровень.
	allEnemyDown, switchedAfterFaint := handleEnemyFaint(bs, &log)
	enemySwitched := enemySwitchedByAI || switchedAfterFaint
	if allEnemyDown {
		respType = "victory"
		bs.Finished = true
		if bs.BattleType == "wild" {
			evoInfo = awardExp(player, bs.WildPokemon.PokemonID) // опыт ТОЛЬКО за дикого
		}
		saveBattleState(bs)
		battleMu.Lock()
		delete(battleSessions, battleID)
		battleMu.Unlock()
	} else if player.IsFainted {
		// Игрок пал (враг при этом мог смениться — уже применено к bs и логу).
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
			delete(battleSessions, battleID)
			battleMu.Unlock()
		}
	} else if enemySwitched {
		respType = "enemy_switch" // враг сменился, бой продолжается
	}

	fillFaintLogImages(bs, log)
	resp := &BattleResponse{
		Type:         respType,
		Log:          log,
		Player:       copyBattlePokemon(player),
		Wild:         copyBattlePokemon(bs.WildPokemon),
		WeatherID:    bs.BattleWeatherID,
		WeatherName:  bs.BattleWeatherName,
		WeatherTurns: bs.WeatherTurns,
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

	fillEnemyIndicator(bs, resp) // индикатор отряда врага (только trainer)
	return resp
}

// fillFaintLogImages подставляет спрайт в записи «X теряет сознание!» — клиент
// рисует мини-спрайт перед текстом, как в строке хода. Картинку берём из команд
// боя по имени (одинаковые имена = один вид = один спрайт).
func fillFaintLogImages(bs *BattleState, log []BattleLogEntry) {
	if bs == nil || len(log) == 0 {
		return
	}
	lookup := func(name string) string {
		for _, team := range [][]BattlePokemon{bs.PlayerTeam, bs.EnemyTeam} {
			for i := range team {
				if team[i].Name == name && team[i].ImageURL != "" {
					return team[i].ImageURL
				}
			}
		}
		return ""
	}
	for i := range log {
		e := &log[i]
		if !e.Fainted || e.PokemonImage != "" {
			continue
		}
		e.PokemonImage = lookup(e.PokemonName)
	}
}

func SwitchHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := session.Store.Get(r, "pokemon-session")
	_, ok := sess.Values["user_id"].(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		BattleID      string `json:"battle_id"`
		UserPokemonID int    `json:"user_pokemon_id"`
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
	// Вынужденная замена: уходящий покемон в обмороке (враг его сбил). Это НЕ ход —
	// игрок бесплатно выставляет нового, враг НЕ бьёт и конец хода не тикает.
	// Добровольная замена (уходящий жив) — как раньше: стоит хода, враг атакует.
	forcedSwitch := bs.PlayerTeam[oldIdx].IsFainted
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
	// Пристальный взгляд (387): нельзя сменить, пока действует (без урона).
	if oldActive.NoEscape {
		http.Error(w, "Покемон не может сбежать из-за Пристального взгляда!", http.StatusBadRequest)
		return
	}
	// Укоренение (heal_moves.go): прикованного покемона нельзя сменить, кроме
	// призраков (тип 14).
	if oldActive.Rooted && !hasTypeID(getPokemonTypes(oldActive), typeGhost) {
		http.Error(w, "Покемон пустил корни и не может быть заменён!", http.StatusBadRequest)
		return
	}

	// Смена покемона прерывает его двухходовую зарядку и мультиход-лок.
	oldActive.ChargingMoveID = 0
	oldActive.Invulnerable = false
	oldActive.LockedMoveID = 0
	oldActive.LockTurns = 0
	// Связывание/запрет смены заканчиваются при уходе покемона.
	oldActive.Trapped = false
	oldActive.TrapTurns = 0
	oldActive.NoEscape = false
	oldActive.Infatuated = false // Влюблённость (Соблазн) спадает при уходе (gender_moves.go)
	// Лечащие волатили заканчиваются при уходе покемона (heal_moves.go).
	oldActive.Rooted = false
	oldActive.AquaRing = false
	oldActive.WishPending = false
	oldActive.HealBlockTurns = 0 // Запрет регенерации спадает при уходе (heal_moves.go)
	// Подготовительные флаги спадают при уходе (focus_moves.go).
	oldActive.SureHit = false
	oldActive.SureCrit = false
	oldActive.ChargedElectric = false
	oldActive.Foresighted = false
	oldActive.MiracleEyed = false
	oldActive.DestinyBond = false                    // Связь судеб спадает при уходе (sacrifice_moves.go)
	oldActive.Grudge = false                         // Жалостливый взор спадает при уходе (sacrifice_moves.go)
	oldActive.Electrified = false                    // Электризация спадает при уходе (field_effects.go)
	oldActive.MagnetRiseTurns = 0                    // Магнетизм спадает при уходе (field_effects.go)
	oldActive.LevitatedTurns = 0                     // Левитация спадает при уходе (field_effects.go)
	oldActive.ActiveTurns = 0                        // счётчик ходов на поле сбрасывается (Татами Блок)
	oldActive.BideTurns, oldActive.BideDamage = 0, 0 // Выжидание прерывается (counter_moves.go)
	oldActive.YawnTurns = 0                          // дремота Зевка спадает при уходе (status_extra_moves.go)
	oldActive.Raging = false
	oldActive.Flinched = false
	oldActive.BattleTypes = nil // смена типа заканчивается при уходе (type_moves.go)
	var formResetLog []BattleLogEntry
	resetAbilityForm(oldActive, bs.Turn, &formResetLog) // форма по HP откатывается (batch5)
	restoreTransform(oldActive)                         // Трансформация откатывается (transform_moves.go)
	restoreBaseStats(oldActive)                         // обмен/усреднение статов отменяется (stat_swap_moves.go)
	resetBlockState(oldActive)                          // Овации/Запрет/Подстрекательство снимаются (block_moves.go)

	bs.ActiveIdx = newIdx
	bs.Turn++

	active := &bs.PlayerTeam[bs.ActiveIdx]
	wild := bs.WildPokemon
	logMsg := fmt.Sprintf("%s : %s в бой!", bs.PlayerName, active.Name)
	logEntries := []BattleLogEntry{
		{Round: bs.Turn, PokemonName: active.Name, PokemonImage: active.ImageURL, Message: logMsg},
	}
	logEntries = append(logEntries, formResetLog...)
	// [batch6] Регенератор: уходящий покемон восстанавливает 1/3 HP (не павший).
	abilityOnSwitchOut(oldActive, bs.Turn, &logEntries)

	// Исцеляющее желание/Лунный танец: входящий полностью исцеляется (до ловушек).
	applyHealingWish(&bs.PlayerSide, active, bs.Turn, &logEntries)

	// Ловушки на поле игрока бьют входящего покемона (hazards.go). Если он пал —
	// просим сменить (или поражение, если боеспособных не осталось).
	if applyEntryHazards(bs, active, &bs.PlayerSide, bs.Turn, &logEntries) {
		logEntries = append(logEntries, BattleLogEntry{
			Round: bs.Turn, PokemonName: active.Name, Fainted: true,
		})
		hasMore := false
		for _, p := range bs.PlayerTeam {
			if !p.IsFainted {
				hasMore = true
				break
			}
		}
		respType := "need_switch"
		if !hasMore {
			respType = "defeat"
			bs.Finished = true
			saveBattleState(bs)
			battleMu.Lock()
			delete(battleSessions, req.BattleID)
			battleMu.Unlock()
		}
		fillFaintLogImages(bs, logEntries)
		resp := BattleResponse{
			Type:         respType,
			Log:          logEntries,
			Player:       copyBattlePokemon(active),
			Wild:         copyBattlePokemon(wild),
			WeatherID:    bs.BattleWeatherID,
			WeatherName:  bs.BattleWeatherName,
			WeatherTurns: bs.WeatherTurns,
		}
		if respType == "need_switch" {
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

	// Вынужденная замена после обморока — это НЕ ход: враг не атакует, край-эффекты
	// (статус/семена/погода/таймеры) не тикают. Ловушки на входящего уже применены выше.
	if forcedSwitch {
		fillFaintLogImages(bs, logEntries)
		resp := BattleResponse{
			Type:         "switch_done",
			Log:          logEntries,
			Player:       copyBattlePokemon(active),
			Wild:         copyBattlePokemon(wild),
			WeatherID:    bs.BattleWeatherID,
			WeatherName:  bs.BattleWeatherName,
			WeatherTurns: bs.WeatherTurns,
		}
		fillEnemyIndicator(bs, &resp)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
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
		// Для "wild" — конец (победа без опыта, как было). Для "trainer" — NPC
		// выставляет следующего врага, и бой продолжается (новый враг атакует ниже).
		allEnemyDown, _ := handleEnemyFaint(bs, &logEntries)
		if allEnemyDown {
			bs.Finished = true
			saveBattleState(bs)
			battleMu.Lock()
			delete(battleSessions, req.BattleID)
			battleMu.Unlock()
			fillFaintLogImages(bs, logEntries)
			resp := BattleResponse{
				Type:         "victory",
				Log:          logEntries,
				Player:       copyBattlePokemon(active),
				Wild:         copyBattlePokemon(wild),
				WeatherID:    bs.BattleWeatherID,
				WeatherName:  bs.BattleWeatherName,
				WeatherTurns: bs.WeatherTurns,
			}
			fillEnemyIndicator(bs, &resp)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		// Тренерский бой: враг сменился — дальше по коду атакует уже новый враг.
		wild = bs.WildPokemon
	}

	postProcessWeather(bs, &logEntries)

	// Отсчёт таймеров стороны поля (экраны) — конец хода (screens.go).
	tickSideTimers(bs, &logEntries)

	// Wild pokemon attacks after switch
	if !wild.IsFainted {
		wildMove := chooseEnemyMove(bs, wild, active)
		if wildMove != nil {
			if wildMove.CategoryID != 3 && wildMove.Power > 0 {
				fainted, wc := executeDamagingMove(bs, wild, active, wildMove, bs.BattleWeatherID, bs.Turn, &logEntries, sideFor(bs, active), sideFor(bs, wild), bs.WaterSportTurns > 0)
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
				if wildMove.CategoryID == 3 && !abilityAbsorbsStatusMove(bs, wild, active, wildMove, bs.Turn, &logEntries) {
					if wc := applyEffectByID(bs, wild, active, wildMove, bs.Turn, &logEntries, sideFor(bs, active)); wc > 0 {
						setBattleWeather(bs, wc, &logEntries)
					}
					applyStatChanges(wild, active, wildMove, bs.BattleWeatherID, bs.Turn, &logEntries, sideFor(bs, active))
					applyHeal(wild, active, wildMove, bs.BattleWeatherID, bs.Turn, &logEntries)
					handleCustomStatusMove(bs, wild, active, wildMove, bs.Turn, &logEntries)
					applySideMove(bs, wild, active, wildMove, &logEntries)
					handlePhazeMove(bs, wild, active, wildMove, &logEntries)
				}
			}
			// Дикий мог выгнать игрока Рыком/Вихрем/Неистовым броском — перечитываем.
			active = &bs.PlayerTeam[bs.ActiveIdx]
		}
	}

	// [batch5] onEndTurn parity: смена тоже завершает ход, поэтому способности
	// конца хода тикают у обоих активных (как в MoveHandler).
	abilityOnEndTurn(bs, active, bs.Turn, &logEntries)
	abilityOnEndTurn(bs, bs.WildPokemon, bs.Turn, &logEntries)
	heldItemOnEndTurn(bs, active, bs.Turn, &logEntries)
	heldItemOnEndTurn(bs, bs.WildPokemon, bs.Turn, &logEntries)

	// Враг мог сбить только что вышедшего покемона одним ударом (или его
	// добили ловушки/волатайлы). Без этой проверки сервер отвечал switch_done,
	// и клиент считал покемона живым — смену не предлагал.
	active = &bs.PlayerTeam[bs.ActiveIdx]
	if active.IsFainted {
		hasMore := false
		for _, p := range bs.PlayerTeam {
			if !p.IsFainted {
				hasMore = true
				break
			}
		}
		respType := "need_switch"
		if !hasMore {
			respType = "defeat"
			bs.Finished = true
			saveBattleState(bs)
			battleMu.Lock()
			delete(battleSessions, req.BattleID)
			battleMu.Unlock()
		}
		fillFaintLogImages(bs, logEntries)
		resp := BattleResponse{
			Type:         respType,
			Log:          logEntries,
			Player:       copyBattlePokemon(active),
			Wild:         copyBattlePokemon(bs.WildPokemon),
			WeatherID:    bs.BattleWeatherID,
			WeatherName:  bs.BattleWeatherName,
			WeatherTurns: bs.WeatherTurns,
		}
		if respType == "need_switch" {
			team := make([]BattlePokemon, len(bs.PlayerTeam))
			for i := range bs.PlayerTeam {
				team[i] = *copyBattlePokemon(&bs.PlayerTeam[i])
			}
			resp.Team = team
		}
		fillEnemyIndicator(bs, &resp)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	fillFaintLogImages(bs, logEntries)
	resp := BattleResponse{
		Type:         "switch_done",
		Log:          logEntries,
		Player:       copyBattlePokemon(active),
		Wild:         copyBattlePokemon(wild),
		WeatherID:    bs.BattleWeatherID,
		WeatherName:  bs.BattleWeatherName,
		WeatherTurns: bs.WeatherTurns,
	}
	fillEnemyIndicator(bs, &resp) // враг мог смениться от остаточного урона

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
	// В раунд, когда погоду только что поставили, счётчик не уменьшаем —
	// иначе 5 сразу превращается в 4.
	if bs.WeatherTurns > 0 && bs.Turn != bs.WeatherSetTurn {
		bs.WeatherTurns--
		if bs.WeatherTurns == 0 && bs.BattleWeatherID != 1 {
			bs.BattleWeatherID = 1
			bs.BattleWeatherName = "Обычная"
			// Round: 0 — служебная строка вне блока раунда (рисуется после
			// раунда, между разделителями), как просил владелец.
			*log = append(*log, BattleLogEntry{
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

func saveBattleState(bs *BattleState) {
	for _, p := range bs.PlayerTeam {
		statusID := p.StatusID
		if statusID < 1 || statusID > 7 {
			statusID = 1
		}
		db.DB.Exec("UPDATE user_pokemons SET current_hp = ?, status_id = ? WHERE id = ?",
			p.CurrentHP, statusID, p.ID)
		db.DB.Exec("UPDATE user_pokemons SET held_item_id = NULLIF(?, 0) WHERE id = ?",
			p.HeldItemID, p.ID)

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
