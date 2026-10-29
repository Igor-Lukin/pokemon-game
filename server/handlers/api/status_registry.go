package api

import mathrand "math/rand"

// РЕЕСТР КАРТОЧЕК СТАТУСОВ (Фаза 0).
//
// Идея: у каждого статуса — одна "карточка" (StatusDef), где собрано ВСЁ его
// поведение, вместо того чтобы размазывать его по switch'ам в battle.go.
// Движок боя дёргает общие графы карточки, не зная про конкретный статус.
//
// Фаза 0 переносит на карточки то, что легко проверяется и не меняет поведение:
//   - урон по ходам (ожог / отравление / сильное отравление)  -> графа OnTurn
//   - множители статов, пока статус висит (ожог, паралич)      -> графа StatMult
// Пропуск хода/разморозка/сон и volatile-статусы пока остаются в processPreTurnStatus
// (переносятся в Фазе 1 через графы CanAct / MoveFilter / Custom).

// StatusDef — "бланк" карточки статуса. 8 граф, общие для всех статусов.
// Пустая графа (nil / 0) = "на этом этапе статус ничего не делает".
type StatusDef struct {
	Name        string                                             // 1. Имя — как показывать в логе (именительный падеж)
	TickWord    string                                             // 1б. форма для сообщения об уроне ("получает урон от <TickWord>")
	SkipMessage string                                             // 1в. сообщение при пропуске хода из-за статуса ("Покемон парализован")
	ImmuneTypes []int                                              // 2. ИммунныТипы — типы, на которых статус не действует
	Duration    int                                                // 3. Длительность — сколько ходов живёт (0 = до лечения)
	OnTurn      func(p *BattlePokemon) int                         // 4. КаждыйХод — изменение HP за ход (- урон / + лечение)
	StatMult    map[int]float64                                    // 5. МножительСтата — постоянные множители статов, пока висит
	CanAct      func(p *BattlePokemon, log *[]BattleLogEntry) bool // 6. МожетДействовать — false = пропускает ход; может менять статус и писать в лог (разморозка/пробуждение)
	MoveFilter  func(p *BattlePokemon, m *BattleMove) bool         // 7. ФильтрАтак — какие атаки запрещены (Фаза 1)
	Custom      func(bs *BattleState, p *BattlePokemon)            // 8. СвойКод — заглушка для уникальной экзотики (Фаза 1+)
	OnInflict   func(p *BattlePokemon)                             // 9. что сделать при наложении статуса (сон -> SleepTurns, сильный яд -> PoisonTurns)
	UpgradeFrom int                                                // 10. можно навесить поверх этого статуса (сильный яд=7 поверх яда=6); 0 = только на чистого
}

// Индексы статов в StatStages (для графы StatMult): 1 Атака, 3 Скорость и т.д.
// (0 HP, 1 Атака, 2 Защита, 3 Скорость, 4 СпАтака, 5 СпЗащита, 6 Точность, 7 Ловкость)

// statusRegistry — карточки по id статуса (таблица statuses в БД).
var statusRegistry = map[int]StatusDef{
	// 3 — Ожог: каждый ход 1/16 макс. HP; физ. атака носителя ×0.5.
	3: {
		Name:     "ожог",
		TickWord: "ожога",
		StatMult: map[int]float64{1: 0.5}, // Атака ×0.5
		OnTurn: func(p *BattlePokemon) int {
			d := p.MaxHP / 16
			if d < 1 {
				d = 1
			}
			return -d
		},
	},

	// 6 — Отравление: каждый ход 1/8 макс. HP.
	6: {
		Name:     "отравление",
		TickWord: "отравления",
		OnTurn: func(p *BattlePokemon) int {
			d := p.MaxHP / 8
			if d < 1 {
				d = 1
			}
			return -d
		},
	},

	// 7 — Сильное отравление: нарастающий урон 1/16, 2/16 ... (потолок 15/16).
	// "Память" статуса — поле p.PoisonTurns (карточка его читает и двигает).
	7: {
		Name:        "сильное отравление",
		TickWord:    "сильного отравления",
		UpgradeFrom: 6, // можно навесить поверх обычного отравления
		OnInflict:   func(p *BattlePokemon) { p.PoisonTurns = 1 },
		OnTurn: func(p *BattlePokemon) int {
			factor := p.PoisonTurns
			if factor < 1 {
				factor = 1
			}
			if factor > 15 {
				factor = 15
			}
			d := p.MaxHP * factor / 16
			if d < 1 {
				d = 1
			}
			p.PoisonTurns++
			return -d
		},
	},

	// 2 — Паралич: скорость носителя ×0.5; 25% шанс пропустить ход.
	2: {
		Name:        "паралич",
		SkipMessage: "Покемон парализован",
		StatMult:    map[int]float64{3: 0.5}, // Скорость ×0.5
		CanAct: func(p *BattlePokemon, log *[]BattleLogEntry) bool {
			if mathrand.Intn(100) < 25 {
				return false // парализован, ход пропущен
			}
			return true
		},
	},

	// 4 — Заморозка: 20% шанс оттаять (и ходить), иначе пропуск хода.
	4: {
		Name:        "заморозка",
		SkipMessage: "Покемон заморожен",
		CanAct: func(p *BattlePokemon, log *[]BattleLogEntry) bool {
			if mathrand.Intn(100) < 20 {
				p.StatusID = 1
				*log = append(*log, BattleLogEntry{
					Round:       0,
					PokemonName: p.Name,
					Message:     p.Name + " оттаивает!",
				})
				return true
			}
			return false
		},
	},

	// 5 — Сон: счётчик SleepTurns; при обнулении просыпается и ходит.
	5: {
		Name:        "сон",
		SkipMessage: "Покемон спит",
		OnInflict:   func(p *BattlePokemon) { p.SleepTurns = mathrand.Intn(3) + 1 },
		CanAct: func(p *BattlePokemon, log *[]BattleLogEntry) bool {
			// [batch7] Кома: постоянно спит, но действует.
			if p.AbilityKey == "coma" {
				return true
			}
			step := 1
			if p.AbilityKey == "early_bird" {
				step = 2
			}
			if p.SleepTurns > 0 {
				p.SleepTurns -= step
			}
			if p.SleepTurns > 0 {
				return false
			}
			p.StatusID = 1
			*log = append(*log, BattleLogEntry{
				Round:       0,
				PokemonName: p.Name,
				Message:     p.Name + " просыпается!",
			})
			return true
		},
	},
}

// weatherEffects: EffectID атаки (14-17) -> id погоды, которую он ставит.
// (Само изменение погоды делает setBattleWeather выше по стеку — сюда только id.)
var weatherEffects = map[int]int{14: 2, 15: 3, 16: 4, 17: 5}

// inflictMajorStatus накладывает основной статус (2-7) на цель через карточку.
// Возвращает true, если статус применён. Нельзя навесить второй основной статус,
// кроме разрешённого апгрейда (сильный яд поверх обычного — UpgradeFrom).
func inflictMajorStatus(target *BattlePokemon, statusID, round int, log *[]BattleLogEntry) bool {
	def := statusRegistry[statusID]
	if target.StatusID > 1 && !(def.UpgradeFrom != 0 && target.StatusID == def.UpgradeFrom) {
		return false
	}
	setStatus(target, statusID, round, log)
	if def.OnInflict != nil {
		def.OnInflict(target)
	}
	return true
}

// statMultiplier возвращает множитель статуса для стата statIdx (или 1, если нет).
func statMultiplier(statusID, statIdx int) float64 {
	if def, ok := statusRegistry[statusID]; ok {
		if m, has := def.StatMult[statIdx]; has {
			return m
		}
	}
	return 1.0
}

// ---------------------------------------------------------------------------
// ПОМОЩНИК УСЛОВИЙ (заготовка для Фазы 2 — условные атаки типа "Отростки +2
// в солнце" и иммунитеты по типам). Пока НЕ подключён к бою: поведение не
// меняет, ждёт слоя "атака -> статус + условие".
// ---------------------------------------------------------------------------

// Condition — простое условие про состояние боя. Нулевое поле = "не задано".
type Condition struct {
	Weather     int // требуется эта погода (id из таблицы weather)
	WeatherNot  int // требуется НЕ эта погода
	SelfHPBelow int // HP атакующего ниже N %
	TargetType  int // цель этого типа (для иммунитетов)
}

// conditionMet — общий "судья": выполнено ли условие сейчас.
// self — тот, кто применяет; opp — цель; weatherID — текущая погода.
func conditionMet(c Condition, self, opp *BattlePokemon, weatherID int) bool {
	if c.Weather != 0 && weatherID != c.Weather {
		return false
	}
	if c.WeatherNot != 0 && weatherID == c.WeatherNot {
		return false
	}
	if c.SelfHPBelow != 0 && self != nil {
		if self.MaxHP <= 0 || self.CurrentHP*100/self.MaxHP >= c.SelfHPBelow {
			return false
		}
	}
	if c.TargetType != 0 && opp != nil {
		has := false
		for _, t := range getPokemonTypes(opp) {
			if t == c.TargetType {
				has = true
				break
			}
		}
		if !has {
			return false
		}
	}
	return true
}
