package api

import (
	"database/sql"
	"fmt"
	"log"
	mathrand "math/rand"
	"pokemon-game/server/db"
	"sort"
	"strings"
)

// Умная генерация команды NPC (поток [12]). Эвристический взвешенный «драфт»:
// на каждый слот считаем score всем кандидатам, берём топ-K и выбираем одного
// взвешенно-случайно (команды «умные», но не идентичные). Надстройка над потоком
// B: собираем []BattlePokemon через buildGenerateFor/wildToBattlePokemon, уровень
// у всех фиксированный. Ветку mode=='smart' вызывает buildNPCTeam (npc_battle.go);
// при пустом пуле возвращаем ошибку -> фолбэк на random.

// --- РУЧКИ НАСТРОЙКИ (веса скоринга + размер топ-K для выбора слота) ---
// Скоринг «за очки»: у каждого кандидата на слот считается score (сумма слагаемых),
// сортируем по убыванию, берём топ-K и тянем одного взвешенно-случайно.
const (
	wCoverage = 3.0 // новые типы, которые команда начинает бить супер-эффективно (STAB)
	wTypeDiv  = 2.0 // разнообразие типов: + за новый тип, − за дубль
	wRole     = 1.5 // закрытие пустующей роли (physical/special/fast/bulky)
	wStrength = 1.0 // нормированная сила (сумма base-статов); слабый вес
	wDefense  = 2.5 // ЗАЩИТА: нормированный бульк + число резистов (чтобы брал стенки)
	wWall     = 8.0 // БОНУС кандидату, закрывающему НЕДОБРАННУЮ обяз. стенку (Д или СД)
	wallThrMul = 1.15 // порог «стенки»: стат защиты >= среднего по пулу * этот множитель
	topK      = 5   // из скольких лучших кандидатов тянем слот взвешенно-случайно
)

// smartCandidate — вид-кандидат в умный драфт (одна строка на вид, дедуп в SQL).
type smartCandidate struct {
	id          int
	name        string
	primaryType int
	secondType  *int
	statSum     int    // сумма base-статов — для нормированной «силы»
	role        string // доминирующая роль (см. classifyRole)
	baseHP      int    // для бульк-оценки (защитный член скоринга)
	baseDef     int    // физ. защита — порог «Д-стенки»
	baseSpDef   int    // спец. защита — порог «СД-стенки»
}

// bulk — суммарная «толщина» (HP + обе защиты) для нормированного защитного члена.
func (c smartCandidate) bulk() int { return c.baseHP + c.baseDef + c.baseSpDef }

// stabTypes — STAB-типы кандидата (оба типа у двутипных).
func (c smartCandidate) stabTypes() []int {
	if c.secondType != nil {
		return []int{c.primaryType, *c.secondType}
	}
	return []int{c.primaryType}
}

// classifyRole — роль вида по доминирующему base-стату.
func classifyRole(hp, atk, def, spd, spAtk, spDef int) string {
	bulky := (hp + def + spDef) / 3
	best := "physical"
	bestVal := atk
	if spAtk > bestVal {
		best, bestVal = "special", spAtk
	}
	if spd > bestVal {
		best, bestVal = "fast", spd
	}
	if bulky > bestVal {
		best, bestVal = "bulky", bulky
	}
	return best
}

// loadSmartTypeEffectiveness — карта attacking->defending->multiplier одним SELECT
// (в память, чтобы не бить БД в цикле скоринга). Плюс множество всех типов-целей.
func loadSmartTypeEffectiveness() (map[int]map[int]float64, map[int]bool, error) {
	rows, err := db.DB.Query("SELECT attacking_type_id, defending_type_id, multiplier FROM type_effectiveness")
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	eff := map[int]map[int]float64{}
	defenders := map[int]bool{}
	for rows.Next() {
		var atk, def int
		var mult float64
		if rows.Scan(&atk, &def, &mult) != nil {
			continue
		}
		if eff[atk] == nil {
			eff[atk] = map[int]float64{}
		}
		eff[atk][def] = mult
		defenders[def] = true
	}
	return eff, defenders, nil
}

// loadSmartCandidates — пул видов, у которых есть урон-атака, обучаемая к уровню
// level (learn_method_id=1, power>0, категория != статус). Дедуп по виду в SQL.
func loadSmartCandidates(level int) ([]smartCandidate, error) {
	rows, err := db.DB.Query(`
		SELECT p.id, p.name, p.primary_type_id, p.secondary_type_id,
		       p.base_hp, p.base_attack, p.base_defense, p.base_speed,
		       p.base_special_attack, p.base_special_defense
		FROM pokemons p
		WHERE p.id NOT IN (
			-- «сильнейшая форма»: исключаем базовые/промежуточные виды, из которых
			-- кто-то эволюционирует; оставляем только финальные (и не эволюционирующих).
			SELECT evolves_from_id FROM pokemons WHERE evolves_from_id IS NOT NULL
		)
		  AND EXISTS (
			SELECT 1 FROM pokemon_attacks pa
			JOIN attacks a ON a.id = pa.attack_id
			WHERE pa.pokemon_id = p.id
			  AND ((pa.learn_method_id = 1 AND pa.level_learned_at <= ?)
			       OR pa.learn_method_id IN (2, 3, 4))
			  AND a.power > 0
			  AND a.category_id != 3
		)`, level)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []smartCandidate
	for rows.Next() {
		var c smartCandidate
		var sec sql.NullInt64
		var hp, atk, def, spd, spAtk, spDef int
		if rows.Scan(&c.id, &c.name, &c.primaryType, &sec,
			&hp, &atk, &def, &spd, &spAtk, &spDef) != nil {
			continue
		}
		if sec.Valid {
			s := int(sec.Int64)
			c.secondType = &s
		}
		c.statSum = hp + atk + def + spd + spAtk + spDef
		c.role = classifyRole(hp, atk, def, spd, spAtk, spDef)
		c.baseHP, c.baseDef, c.baseSpDef = hp, def, spDef
		out = append(out, c)
	}
	return out, nil
}

// teamState — состояние команды по мере набора слотов.
type teamState struct {
	types       map[int]bool // типы, уже представленные в команде (разнообразие)
	coverage    map[int]bool // defending-типы, которые команда уже бьёт супер-эффективно
	roles       map[string]int
	hasPhysWall bool // в команде уже есть физ-стенка (высокая Защита)
	hasSpecWall bool // в команде уже есть спец-стенка (высокая СпецЗащита)
}

// defenseScore — защитный член: нормированный бульк + доля типов, которые кандидат
// РЕЗИСТИТ своими типами (mult<1). Награда за «толстых» и удобных по резистам видов.
func (c smartCandidate) defenseScore(eff map[int]map[int]float64, defenders map[int]bool, maxBulk int) float64 {
	bulkN := 0.0
	if maxBulk > 0 {
		bulkN = float64(c.bulk()) / float64(maxBulk)
	}
	ctypes := c.stabTypes()
	resists := 0
	total := 0
	for a := range defenders { // a — атакующий тип; универсум типов = ключи defenders
		total++
		mult := 1.0
		for _, ct := range ctypes {
			if eff[a] != nil {
				if v, ok := eff[a][ct]; ok {
					mult *= v
				}
			}
		}
		if mult < 1.0 {
			resists++
		}
	}
	resistN := 0.0
	if total > 0 {
		resistN = float64(resists) / float64(total)
	}
	return bulkN + resistN
}

// coversNew — сколько НОВЫХ типов-целей кандидат начинает бить супер-эффективно
// (mult>1) своими STAB-типами (то, чего команда ещё не покрывает).
func (c smartCandidate) coversNew(st teamState, eff map[int]map[int]float64, defenders map[int]bool) int {
	n := 0
	for d := range defenders {
		if st.coverage[d] {
			continue
		}
		for _, s := range c.stabTypes() {
			if eff[s] != nil && eff[s][d] > 1 {
				n++
				break
			}
		}
	}
	return n
}

// scoreCandidate — взвешенная сумма слагаемых (см. §4.3 ТЗ) + защитный член.
// Обязательная квота стенок (Д/СД) считается ОТДЕЛЬНО в буилд-цикле (бонус wWall
// и жёсткая резервация слотов), т.к. зависит от того, что уже набрано.
func (c smartCandidate) score(st teamState, eff map[int]map[int]float64, defenders map[int]bool, maxStatSum, maxBulk int) float64 {
	// покрытие
	cov := float64(c.coversNew(st, eff, defenders))
	// разнообразие типов: + за новый тип, − за дубль
	div := 0.0
	for _, t := range c.stabTypes() {
		if st.types[t] {
			div--
		} else {
			div++
		}
	}
	// роль: бонус за пустующую
	roleBonus := 0.0
	if st.roles[c.role] == 0 {
		roleBonus = 1.0
	}
	// сила (нормированная)
	strength := 0.0
	if maxStatSum > 0 {
		strength = float64(c.statSum) / float64(maxStatSum)
	}
	// защита (бульк + резисты) — чтобы команда брала стенки, а не только дамаг
	def := c.defenseScore(eff, defenders, maxBulk)
	return wCoverage*cov + wTypeDiv*div + wRole*roleBonus + wStrength*strength + wDefense*def
}

// pickWeighted — взвешенно-случайный выбор индекса из топ-K по score.
// Веса сдвигаем в положительную область (score может быть отрицательным).
func pickWeighted(scores []float64) int {
	if len(scores) == 0 {
		return -1
	}
	min := scores[0]
	for _, s := range scores {
		if s < min {
			min = s
		}
	}
	total := 0.0
	weights := make([]float64, len(scores))
	for i, s := range scores {
		w := s - min + 1.0 // минимальный вес 1, чтобы у аутсайдера топ-K был шанс
		weights[i] = w
		total += w
	}
	r := mathrand.Float64() * total
	for i, w := range weights {
		r -= w
		if r <= 0 {
			return i
		}
	}
	return len(scores) - 1
}

// buildSmartNPCTeam — собрать умную команду из size видов уровня level.
func buildSmartNPCTeam(size, level int) ([]BattlePokemon, error) {
	if size < 1 {
		size = 1
	} else if size > 6 {
		size = 6
	}
	if level < 1 {
		level = 1
	}

	pool, err := loadSmartCandidates(level)
	if err != nil {
		return nil, err
	}
	if len(pool) == 0 {
		return nil, fmt.Errorf("умный пул кандидатов пуст")
	}

	eff, defenders, err := loadSmartTypeEffectiveness()
	if err != nil {
		return nil, err
	}

	// Нормировки (сила/бульк) и пороги «стенок» по пулу.
	maxStatSum, maxBulk := 0, 0
	sumDef, sumSpDef := 0, 0
	for _, c := range pool {
		if c.statSum > maxStatSum {
			maxStatSum = c.statSum
		}
		if c.bulk() > maxBulk {
			maxBulk = c.bulk()
		}
		sumDef += c.baseDef
		sumSpDef += c.baseSpDef
	}
	physThr := float64(sumDef) / float64(len(pool)) * wallThrMul   // порог Д-стенки
	specThr := float64(sumSpDef) / float64(len(pool)) * wallThrMul // порог СД-стенки
	isPhysWall := func(c smartCandidate) bool { return float64(c.baseDef) >= physThr }
	isSpecWall := func(c smartCandidate) bool { return float64(c.baseSpDef) >= specThr }

	st := teamState{types: map[int]bool{}, coverage: map[int]bool{}, roles: map[string]int{}}
	used := map[int]bool{}
	var team []BattlePokemon

	for slot := 0; slot < size && len(used) < len(pool); slot++ {
		// Обязательные стенки: сколько ещё не набрано и сколько слотов осталось.
		needPhys := !st.hasPhysWall
		needSpec := !st.hasSpecWall
		neededWalls := 0
		if needPhys {
			neededWalls++
		}
		if needSpec {
			neededWalls++
		}
		remaining := size - len(team)
		// Жёсткая резервация: если слотов осталось ровно (или меньше) под недобранные
		// стенки — слот ПРИНУДИТЕЛЬНО отдаём стенке (гарантия 1 Д + 1 СД, «штраф»
		// за отказ фактически бесконечный).
		forceWall := neededWalls > 0 && remaining <= neededWalls

		// Собираем доступных кандидатов + их score.
		type scored struct {
			c smartCandidate
			s float64
		}
		var cands []scored
		for _, c := range pool {
			if used[c.id] {
				continue
			}
			var sc float64
			switch {
			case forceWall:
				// Только «стеночная ценность» по недобранным сторонам; квалифицированные
				// (выше порога) гарантированно обходят прочих — в слот войдёт стенка.
				ws := 0.0
				if needPhys {
					ws += float64(c.baseDef)
				}
				if needSpec {
					ws += float64(c.baseSpDef)
				}
				if (needPhys && isPhysWall(c)) || (needSpec && isSpecWall(c)) {
					ws += 100000 // квалифицированная стенка доминирует над обычными
				}
				sc = ws
			case slot == 0:
				// Семя: баланс силы и защиты + небольшой рандом (вариативность стартов).
				strength := 0.0
				if maxStatSum > 0 {
					strength = float64(c.statSum) / float64(maxStatSum)
				}
				sc = wStrength*strength + wDefense*c.defenseScore(eff, defenders, maxBulk) + mathrand.Float64()*0.5
			default:
				sc = c.score(st, eff, defenders, maxStatSum, maxBulk)
				// Бонус за закрытие ОБЯЗАТЕЛЬНОЙ недобранной стенки (без него такой
				// кандидат проигрывает в очках — это и есть «штраф за отказ»).
				if needPhys && isPhysWall(c) {
					sc += wWall
				}
				if needSpec && isSpecWall(c) {
					sc += wWall
				}
			}
			cands = append(cands, scored{c, sc})
		}
		if len(cands) == 0 {
			break
		}
		// Топ-K по score.
		sort.Slice(cands, func(i, j int) bool { return cands[i].s > cands[j].s })
		k := topK
		if k > len(cands) {
			k = len(cands)
		}
		top := cands[:k]
		scores := make([]float64, k)
		for i := range top {
			scores[i] = top[i].s
		}
		chosen := top[pickWeighted(scores)].c

		// Материализуем вид на фиксированном уровне.
		gen := buildGenerateFor(chosen.id, level)
		if gen == nil {
			used[chosen.id] = true // битый вид — не зациклиться
			slot--
			continue
		}
		// Умный набор атак (§замечание 2): бот берёт нужные ходы из ВСЕХ методов
		// обучения (level-up/ТМ/яйцо/репетитор), а не 4 случайных level-up.
		if smart := generateSmartMoves(chosen.id, level, chosen.stabTypes()); len(smart) > 0 {
			gen.Moves = smart
		}
		team = append(team, *wildToBattlePokemon(gen))

		// Пометка стенок для лога/квоты.
		wallTag := ""
		if isPhysWall(chosen) {
			wallTag += " [Д-стенка]"
		}
		if isSpecWall(chosen) {
			wallTag += " [СД-стенка]"
		}

		// Лог решения бота: кого и с какими атаками поставил в команду (замечание 2).
		log.Printf("[smart-team] слот %d/%d: #%d %s ур.%d роль=%s%s STAB-типы=%v атаки=[%s]",
			slot+1, size, chosen.id, chosen.name, level, chosen.role, wallTag,
			chosen.stabTypes(), moveNamesList(gen.Moves))

		// Обновляем состояние команды.
		used[chosen.id] = true
		st.roles[chosen.role]++
		if isPhysWall(chosen) {
			st.hasPhysWall = true
		}
		if isSpecWall(chosen) {
			st.hasSpecWall = true
		}
		for _, t := range chosen.stabTypes() {
			st.types[t] = true
		}
		for d := range defenders {
			if st.coverage[d] {
				continue
			}
			for _, s := range chosen.stabTypes() {
				if eff[s] != nil && eff[s][d] > 1 {
					st.coverage[d] = true
					break
				}
			}
		}
	}

	// Гарантия боеспособности (инвариант потока B).
	alive := 0
	for i := range team {
		if !team[i].IsFainted && team[i].CurrentHP > 0 {
			alive++
		}
	}
	if len(team) == 0 || alive == 0 {
		return nil, fmt.Errorf("умная команда пуста/без боеспособных")
	}

	// Итоговый состав одной строкой — быстрый взгляд на решение бота (замечание 2).
	names := make([]string, len(team))
	for i := range team {
		names[i] = team[i].Name
	}
	log.Printf("[smart-team] итог: %d покемонов ур.%d -> [%s]", len(team), level, strings.Join(names, ", "))

	return team, nil
}

// logSmartSwitch — лог решения о замене. Вынесен из processTurn, т.к. там
// локальная переменная log ([]BattleLogEntry) перекрывает пакет log.
func logSmartSwitch(from, to, target string) {
	log.Printf("[smart-team] замена: %s -> %s (текущий не пробивал %s)", from, to, target)
}

// moveNamesList — имена ходов через запятую (для лога решения бота).
func moveNamesList(moves []GeneratedMove) string {
	names := make([]string, len(moves))
	for i, m := range moves {
		names[i] = m.Name
	}
	return strings.Join(names, ", ")
}

// ==================== Умный набор атак (замечание 2) ====================

// generateSmartMoves — до 4 атак из ВСЕХ методов обучения (level-up c учётом
// уровня + ТМ/яйцо/репетитор без ограничения по уровню), выбранных «с умом»:
// сначала лучшая STAB-атака, затем добор урон-атак НОВЫХ типов (покрытие), потом
// добор по «эффективной силе». Не случайно (в отличие от generateWildMoves).
// stab — STAB-типы вида. Пусто -> nil (buildGenerateFor оставит level-up-фолбэк).
func generateSmartMoves(pokemonID, level int, stab []int) []GeneratedMove {
	rows, err := db.DB.Query(`
		SELECT a.id, a.name, COALESCE(a.eng_name, ''), a.type_id, a.category_id, a.power, a.accuracy, a.pp,
		       a.priority,
		       COALESCE(a.target_id, 1), COALESCE(a.description, ''),
		       COALESCE(a.effect_status_id, 0), COALESCE(a.effect_chance, 0),
		       COALESCE(a.never_misses, 0), COALESCE(a.drain, 0),
		       COALESCE(a.crit_rate, 0), COALESCE(a.hits_min, 1), COALESCE(a.hits_max, 1),
		       COALESCE(a.disabled, 0), COALESCE(a.heal_percent, 0), COALESCE(a.contact, 0)
		FROM pokemon_attacks pa
		JOIN attacks a ON pa.attack_id = a.id
		WHERE pa.pokemon_id = ?
		  AND ((pa.learn_method_id = 1 AND pa.level_learned_at <= ?) OR pa.learn_method_id IN (2, 3, 4))
		GROUP BY a.id`, pokemonID, level)
	if err != nil {
		return nil
	}
	defer rows.Close()

	stabSet := map[int]bool{}
	for _, t := range stab {
		stabSet[t] = true
	}

	var all []GeneratedMove
	for rows.Next() {
		var m GeneratedMove
		var neverMisses, disabled, contact int
		if err := rows.Scan(&m.ID, &m.Name, &m.EngName, &m.TypeID, &m.CategoryID, &m.Power, &m.Accuracy, &m.PP, &m.Priority, &m.TargetID, &m.Description, &m.EffectID, &m.EffectChance, &neverMisses, &m.Drain, &m.CritRate, &m.HitsMin, &m.HitsMax, &disabled, &m.HealPercent, &contact); err != nil {
			continue
		}
		m.NeverMisses = neverMisses != 0
		m.Disabled = disabled != 0
		m.Contact = contact != 0
		all = append(all, m)
	}
	if len(all) == 0 {
		return nil
	}

	// Только урон-атаки (категория != статус, power>0) — из них строим набор.
	var dmg []GeneratedMove
	for _, m := range all {
		if m.CategoryID != 3 && m.Power > 0 {
			dmg = append(dmg, m)
		}
	}
	if len(dmg) == 0 {
		return nil
	}

	// «Эффективная сила» = power * (1.5 STAB) * точность (never_misses -> 100%).
	effPower := func(m GeneratedMove) float64 {
		acc := m.Accuracy
		if acc <= 0 || m.NeverMisses {
			acc = 100
		}
		f := float64(m.Power) * float64(acc) / 100.0
		if stabSet[m.TypeID] {
			f *= 1.5
		}
		return f
	}
	sort.Slice(dmg, func(i, j int) bool { return effPower(dmg[i]) > effPower(dmg[j]) })

	chosen := make([]GeneratedMove, 0, 4)
	takenID := map[int]bool{}
	takenType := map[int]bool{}
	add := func(m GeneratedMove) {
		chosen = append(chosen, m)
		takenID[m.ID] = true
		takenType[m.TypeID] = true
	}

	// 1) Лучшая STAB-атака — гарантированный основной урон.
	for _, m := range dmg {
		if stabSet[m.TypeID] {
			add(m)
			break
		}
	}
	// 2) Добор атак НОВЫХ типов (покрытие) по убыванию эффективной силы.
	for _, m := range dmg {
		if len(chosen) >= 4 {
			break
		}
		if takenID[m.ID] || takenType[m.TypeID] {
			continue
		}
		add(m)
	}
	// 3) Добить слоты сильнейшими из оставшихся (даже если тип повторяется).
	for _, m := range dmg {
		if len(chosen) >= 4 {
			break
		}
		if takenID[m.ID] {
			continue
		}
		add(m)
	}
	return chosen
}

// ==================== Поиск на 1 ход: ходы и замены врага ====================
// (замечание 3 + выбор направления «эвристика + поиск»). Оценка урона —
// упрощённая копия боевой формулы (battle.go): без крита/погоды/экранов, но с
// STAB, типовой эффективностью и точностью. Достаточно для решения «чем бить» и
// «на кого менять». Работает только у smart-тренера (bs.EnemySmart).

// estimateMoveDamage — прикидка урона хода attacker->defender (без мутаций).
func estimateMoveDamage(attacker, defender *BattlePokemon, m *BattleMove) float64 {
	if m.CategoryID == 3 || m.Power <= 0 {
		return 0
	}
	var a, d int
	if m.CategoryID == 1 { // физическая: Атака vs Защита
		a, d = attacker.Attack, defender.Defense
	} else { // специальная: СпецАтака vs СпецЗащита
		a, d = attacker.SpecialAttack, defender.SpecialDefense
	}
	if a < 1 {
		a = 1
	}
	if d < 1 {
		d = 1
	}
	base := (float64(2*attacker.Level+10)/250.0)*(float64(a)/float64(d))*float64(m.Power) + 2
	stab := 1.0
	for _, t := range getPokemonTypes(attacker) {
		if t == m.TypeID {
			stab = 1.5
			break
		}
	}
	eff := getTypeEffectiveness(m.TypeID, getPokemonTypes(defender))
	acc := float64(m.Accuracy)
	if m.NeverMisses || acc <= 0 {
		acc = 100
	}
	return base * stab * eff * (acc / 100.0)
}

// bestDamageFrac — лучший урон атакующего по цели как ДОЛЯ макс-HP цели (0..>1).
func bestDamageFrac(attacker, defender *BattlePokemon) float64 {
	best := 0.0
	for i := range attacker.Moves {
		m := &attacker.Moves[i]
		if d := estimateMoveDamage(attacker, defender, m); d > best {
			best = d
		}
	}
	if defender.MaxHP <= 0 {
		return 0
	}
	return best / float64(defender.MaxHP)
}

// matchupScore — ценность расклада для «me» против «opp»: мой лучший урон минус
// урон соперника (в долях HP). >0 — я в выигрыше, <0 — в проигрыше.
func matchupScore(me, opp *BattlePokemon) float64 {
	return bestDamageFrac(me, opp) - bestDamageFrac(opp, me)
}

// chooseSmartEnemyMove — ход врага с максимальной прикидкой урона по игроку
// (1-ply). Нет доступных урон-атак -> прежнее поведение (статус/рандом).
func chooseSmartEnemyMove(wild, player *BattlePokemon) *BattleMove {
	best := -1
	bestDmg := -1.0
	for i := range wild.Moves {
		m := &wild.Moves[i]
		if moveBlockedFor(wild, m.ID) || m.CategoryID == 3 || m.Power <= 0 {
			continue
		}
		if d := estimateMoveDamage(wild, player, m); d > bestDmg {
			bestDmg, best = d, i
		}
	}
	if best < 0 {
		return chooseWildMove(wild)
	}
	return &wild.Moves[best]
}

// chooseEnemyMove — единая точка выбора хода врага: у smart-тренера — поиском
// (chooseSmartEnemyMoveOrCharge), иначе старое поведение (chooseWildMoveOrCharge).
// opp — покемон игрока (цель оценки урона).
func chooseEnemyMove(bs *BattleState, wild, opp *BattlePokemon) *BattleMove {
	if bs != nil && bs.EnemySmart {
		return chooseSmartEnemyMoveOrCharge(wild, opp)
	}
	return chooseWildMoveOrCharge(wild)
}

// chooseSmartEnemyMoveOrCharge — как chooseWildMoveOrCharge, но обычный ход выбирает
// поиском. Форс-состояния (зарядка/лок/выжидание/овации) отдаём штатному обработчику.
func chooseSmartEnemyMoveOrCharge(wild, player *BattlePokemon) *BattleMove {
	if wild.ChargingMoveID != 0 || wild.LockedMoveID != 0 || wild.BideTurns > 0 ||
		(wild.EncoreTurns != 0 && wild.EncoreMoveID != 0) || player == nil {
		return chooseWildMoveOrCharge(wild)
	}
	return chooseSmartEnemyMove(wild, player)
}

// chooseEnemySwitch — индекс скамеечного врага для замены, или -1. Меняемся, только
// если ТЕКУЩИЙ в проигрыше по раскладу (matchupScore<0), а на скамейке есть
// боеспособный с ВЫИГРЫШНЫМ раскладом (>0) — берём лучший. Циклов нет: вошедший
// (расклад>0) в следующий ход не в проигрыше и не меняется.
func chooseEnemySwitch(bs *BattleState, player, wild *BattlePokemon) int {
	// Не вмешиваемся в двухходовые/лок/выжидание/овации — пусть отработают.
	if wild.ChargingMoveID != 0 || wild.LockedMoveID != 0 || wild.BideTurns > 0 || wild.EncoreTurns > 0 {
		return -1
	}
	if matchupScore(wild, player) >= 0 {
		return -1 // текущий не в проигрыше — незачем менять
	}
	best := -1
	bestScore := 0.0 // кандидат обязан быть в плюсе (выигрывать расклад)
	for i := range bs.EnemyTeam {
		if i == bs.EnemyActiveIdx {
			continue
		}
		c := &bs.EnemyTeam[i]
		if c.IsFainted || c.CurrentHP <= 0 {
			continue
		}
		if s := matchupScore(c, player); s > bestScore {
			bestScore, best = s, i
		}
	}
	return best
}
