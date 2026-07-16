package api

// VOLATILE-СТАТУСЫ (временные) — Фаза B.
//
// В отличие от основных статусов (StatusDef, один слот p.StatusID), volatile
// хранятся отдельными полями покемона (p.Flinched, p.Confused/ConfusedTurns,
// p.LeechSeeded, p.Trapped) и могут висеть НЕСКОЛЬКО одновременно.
//
// У них три разные фазы в бою, поэтому один общий цикл им не подходит:
//   - "испуг" (Flinch)        — ДО действия покемона (может пропустить ход);
//   - "замешательство"        — тоже перед действием, но ПОСЛЕ урона от статуса;
//   - "семена-пиявки"/"ловушка" — фаза "по ходам" (обе стороны, порядок важен).
// Поэтому pre-act-функции вызываются напрямую из processPreTurnStatus (в своих
// местах), а по-ходовые собраны в реестр endTurnVolatiles и гоняются одним
// проходом runEndTurnVolatiles — сюда же добавляются будущие по-ходовые volatile.
//
// Поведение перенесено из battle.go БЕЗ изменений (тексты/формулы/порядок).

import (
	mathrand "math/rand"
	"strconv"
)

// volatileFlinch: испуг. Возвращает false, если покемон пропускает ход.
func volatileFlinch(p *BattlePokemon, log *[]BattleLogEntry) bool {
	if p.Flinched {
		p.Flinched = false
		*log = append(*log, BattleLogEntry{
			Round:       0,
			PokemonName: p.Name,
			Message:     p.Name + " испуган(а)!",
		})
		return false
	}
	return true
}

// volatileConfusion: замешательство. Возвращает false, если покемон ударил
// себя (пропуск хода); во всех прочих случаях true.
func volatileConfusion(p *BattlePokemon, log *[]BattleLogEntry) bool {
	if !p.Confused {
		return true
	}
	p.ConfusedTurns--
	if p.ConfusedTurns <= 0 {
		p.Confused = false
		*log = append(*log, BattleLogEntry{
			Round:       0,
			PokemonName: p.Name,
			Message:     p.Name + " больше не замешана(н)!",
		})
		return true
	}
	*log = append(*log, BattleLogEntry{
		Round:       0,
		PokemonName: p.Name,
		Message:     p.Name + " в замешательстве!",
	})
	if mathrand.Intn(100) < 33 {
		confDmg := p.Level * 2
		p.CurrentHP -= confDmg
		*log = append(*log, BattleLogEntry{
			Round:       0,
			PokemonName: p.Name,
			Message:     p.Name + " ударяет себя в замешательстве! -" + strconv.Itoa(confDmg) + " HP",
		})
		if p.CurrentHP <= 0 {
			p.CurrentHP = 0
			p.IsFainted = true
			*log = append(*log, BattleLogEntry{
				Round:       0,
				PokemonName: p.Name,
				Fainted:     true,
			})
		}
		return false
	}
	return true
}

// volatileLeechSeed: семена-пиявки. seeded теряет HP, seeder восстанавливает.
func volatileLeechSeed(seeder, seeded *BattlePokemon, log *[]BattleLogEntry) {
	if !seeded.LeechSeeded || seeded.IsFainted || seeder.IsFainted {
		return
	}
	dmg := seeded.MaxHP / 8
	if dmg < 1 {
		dmg = 1
	}
	seeded.CurrentHP -= dmg
	seeder.CurrentHP += dmg
	if seeder.CurrentHP > seeder.MaxHP {
		seeder.CurrentHP = seeder.MaxHP
	}
	*log = append(*log, BattleLogEntry{
		Round:   0,
		Message: seeded.Name + " теряет " + strconv.Itoa(dmg) + " HP от семян-пиявок! " + seeder.Name + " восстанавливает " + strconv.Itoa(dmg) + " HP!",
	})
	if seeded.CurrentHP <= 0 {
		seeded.CurrentHP = 0
		seeded.IsFainted = true
		*log = append(*log, BattleLogEntry{
			Round:       0,
			PokemonName: seeded.Name,
			Fainted:     true,
		})
	}
}

// volatileTrap: связывание (Спираль огня/Зажим/Зыбучие пески/Водоворот/Заражение/
// Ураган магмы). Носитель 4-5 ходов теряет 1/16 макс. HP и не может сменить
// покемона. TrapTurns считает оставшиеся ходы; на нуле связывание спадает.
func volatileTrap(p *BattlePokemon, log *[]BattleLogEntry) {
	if !p.Trapped || p.IsFainted {
		return
	}
	dmg := p.MaxHP / 16
	if dmg < 1 {
		dmg = 1
	}
	p.CurrentHP -= dmg
	*log = append(*log, BattleLogEntry{
		Round:   0,
		Message: p.Name + " получает " + strconv.Itoa(dmg) + " урона от ловушки!",
	})
	if p.CurrentHP <= 0 {
		p.CurrentHP = 0
		p.IsFainted = true
		*log = append(*log, BattleLogEntry{
			Round:       0,
			PokemonName: p.Name,
			Fainted:     true,
		})
		return
	}
	// Отсчёт длительности: на нуле связывание спадает (цель снова может уйти).
	p.TrapTurns--
	if p.TrapTurns <= 0 {
		p.Trapped = false
		p.TrapTurns = 0
		*log = append(*log, BattleLogEntry{
			Round:   0,
			Message: p.Name + " освободился из ловушки!",
		})
	}
}

// volatileCurse: проклятие. Носитель теряет 1/4 макс. HP по ходам.
func volatileCurse(p *BattlePokemon, log *[]BattleLogEntry) {
	if !p.Cursed || p.IsFainted {
		return
	}
	dmg := p.MaxHP / 4
	if dmg < 1 {
		dmg = 1
	}
	p.CurrentHP -= dmg
	*log = append(*log, BattleLogEntry{
		Round:   0,
		Message: p.Name + " страдает от проклятия! -" + strconv.Itoa(dmg) + " HP",
	})
	if p.CurrentHP <= 0 {
		p.CurrentHP = 0
		p.IsFainted = true
		*log = append(*log, BattleLogEntry{
			Round:       0,
			PokemonName: p.Name,
			Fainted:     true,
		})
	}
}

// volatileNightmare: Кошмарные сны. Спящая цель теряет 1/4 макс. HP по ходам;
// при пробуждении эффект спадает.
func volatileNightmare(p *BattlePokemon, log *[]BattleLogEntry) {
	if !p.Nightmared || p.IsFainted {
		return
	}
	if p.StatusID != 5 { // проснулся — кошмары кончились
		p.Nightmared = false
		*log = append(*log, BattleLogEntry{
			Round: 0, Message: p.Name + " просыпается — кошмары рассеиваются.",
		})
		return
	}
	dmg := p.MaxHP / 4
	if dmg < 1 {
		dmg = 1
	}
	p.CurrentHP -= dmg
	*log = append(*log, BattleLogEntry{
		Round:   0,
		Message: p.Name + " терзают кошмары! -" + strconv.Itoa(dmg) + " HP",
	})
	if p.CurrentHP <= 0 {
		p.CurrentHP = 0
		p.IsFainted = true
		*log = append(*log, BattleLogEntry{Round: 0, PokemonName: p.Name, Fainted: true})
	}
}

// inflictNightmare — Кошмарные сны(488): действует ТОЛЬКО на спящую цель.
func inflictNightmare(t *BattlePokemon, round int, log *[]BattleLogEntry) {
	if t.StatusID != 5 {
		*log = append(*log, BattleLogEntry{
			Round: round, PokemonName: t.Name, MoveFailed: "Кошмары насылают только на спящих — не сработало!",
		})
		return
	}
	if t.Nightmared {
		return
	}
	t.Nightmared = true
	*log = append(*log, BattleLogEntry{
		Round: round, PokemonName: t.Name, PokemonImage: t.ImageURL,
		StatusEvent: "кошмары",
	})
}

// ---- Наложение volatile-статусов атакой (из applyEffectByID по EffectID) ----

// inflictVolatile: EffectID атаки -> функция наложения volatile на цель.
// 8 испуг, 10 семена-пиявки, 11 ловушка, 13 замешательство, 21 кошмары.
var inflictVolatile = map[int]func(target *BattlePokemon, round int, log *[]BattleLogEntry){
	8:  inflictFlinch,
	10: inflictLeechSeed,
	11: inflictTrap,
	13: inflictConfusion,
	21: inflictNightmare, // Кошмарные сны (в БД у 488 effect_status_id=21)
}

func inflictFlinch(t *BattlePokemon, round int, log *[]BattleLogEntry) {
	t.Flinched = true
	*log = append(*log, BattleLogEntry{
		Round:        round,
		PokemonName:  t.Name,
		PokemonImage: t.ImageURL,
	})
}

func inflictLeechSeed(t *BattlePokemon, round int, log *[]BattleLogEntry) {
	if t.LeechSeeded {
		return
	}
	t.LeechSeeded = true
	*log = append(*log, BattleLogEntry{
		Round:        round,
		PokemonName:  t.Name,
		PokemonImage: t.ImageURL,
		StatusEvent:  "семена-пиявки",
	})
}

func inflictTrap(t *BattlePokemon, round int, log *[]BattleLogEntry) {
	if t.Trapped {
		return
	}
	t.Trapped = true
	t.TrapTurns = mathrand.Intn(2) + 4 // связывание держится 4-5 ходов
	*log = append(*log, BattleLogEntry{
		Round:        round,
		PokemonName:  t.Name,
		PokemonImage: t.ImageURL,
		StatusEvent:  "ловушка",
	})
}

// inflictMeanLook — Пристальный взгляд (387): цель не может сменить покемона
// (без урона по ходам, без таймера — держит, пока связавший в бою). Призрачный
// тип иммунен. Повторное применение — без эффекта.
func inflictMeanLook(t *BattlePokemon, round int, log *[]BattleLogEntry) {
	for _, ty := range getPokemonTypes(t) {
		if ty == typeGhost {
			*log = append(*log, BattleLogEntry{
				Round:   round,
				Message: "Но на " + t.Name + " это не действует!",
			})
			return
		}
	}
	if t.NoEscape {
		return
	}
	t.NoEscape = true
	*log = append(*log, BattleLogEntry{
		Round:        round,
		PokemonName:  t.Name,
		PokemonImage: t.ImageURL,
		StatusEvent:  "нельзя сбежать",
	})
}

func inflictConfusion(t *BattlePokemon, round int, log *[]BattleLogEntry) {
	if t.Confused {
		return
	}
	t.Confused = true
	t.ConfusedTurns = mathrand.Intn(3) + 2
	*log = append(*log, BattleLogEntry{
		Round:        round,
		PokemonName:  t.Name,
		PokemonImage: t.ImageURL,
		StatusEvent:  "замешательство",
	})
}

// inflictCurse — CUSTOM-обработчик атаки "Проклятье" (EffectID effectCurse=12).
// Уникальная механика, зависит от типа применяющего (attacker):
//   ПРИЗРАК: жертвует 1/2 от МАКС. HP (может уйти в обморок) и проклинает
//     цель (−1/4 HP по ходам). Если цель уже проklята — провал, HP не тратится.
//   НЕ ПРИЗРАК: себе Атака +1, Защита +1, Скорость −1 (статуса не накладывает).
// Всё поведение атаки живёт здесь (у атаки НЕТ строк attack_stat_changes),
// иначе не-призрак-статы протекли бы и в призрак-ветку.
func inflictCurse(attacker, defender *BattlePokemon, round int, log *[]BattleLogEntry) {
	isGhost := false
	for _, t := range getPokemonTypes(attacker) {
		if t == typeGhost {
			isGhost = true
			break
		}
	}
	if !isGhost {
		curseBump(attacker, 1, 1, round, log)  // Атака +1
		curseBump(attacker, 2, 1, round, log)  // Защита +1
		curseBump(attacker, 3, -1, round, log) // Скорость −1
		return
	}
	// Призрак-ветка.
	if defender.Cursed {
		*log = append(*log, BattleLogEntry{
			Round:   round,
			Message: "Но это не сработало!",
		})
		return
	}
	cost := attacker.MaxHP / 2
	if cost < 1 {
		cost = 1
	}
	attacker.CurrentHP -= cost
	if attacker.CurrentHP < 0 {
		attacker.CurrentHP = 0
	}
	*log = append(*log, BattleLogEntry{
		Round:       round,
		PokemonName: attacker.Name,
		Message:     attacker.Name + " жертвует половиной HP! -" + strconv.Itoa(cost) + " HP",
	})
	defender.Cursed = true
	*log = append(*log, BattleLogEntry{
		Round:        round,
		PokemonName:  defender.Name,
		PokemonImage: defender.ImageURL,
		StatusEvent:  "проклятие",
	})
	if attacker.CurrentHP <= 0 {
		attacker.IsFainted = true
		*log = append(*log, BattleLogEntry{
			Round:       round,
			PokemonName: attacker.Name,
			Fainted:     true,
		})
	}
}

// curseBump — изменить ступень стата на delta с клэмпом [-6,6] и записью в лог
// (для не-призрак-ветки Проклятья; величина всегда ±1, поэтому без "резко").
func curseBump(p *BattlePokemon, statID, delta, round int, log *[]BattleLogEntry) {
	p.StatStages[statID] += delta
	if p.StatStages[statID] > 6 {
		p.StatStages[statID] = 6
	} else if p.StatStages[statID] < -6 {
		p.StatStages[statID] = -6
	}
	name := statChangeNames[statID]
	if name == "" {
		name = "Стат"
	}
	verb := "повышена"
	if delta < 0 {
		verb = "снижена"
	}
	*log = append(*log, BattleLogEntry{
		Round:      round,
		StatChange: name + " " + p.Name + " " + verb + "!",
	})
}

// endTurnVolatiles — реестр по-ходовых volatile-эффектов. Порядок сохраняет
// прежний: сначала семена-пиявки (для обоих покемонов), потом ловушка.
// Новый по-ходовой volatile добавляется сюда одной строкой.
var endTurnVolatiles = []struct {
	name  string
	apply func(a, b *BattlePokemon, log *[]BattleLogEntry)
}{
	{"семена-пиявки", func(a, b *BattlePokemon, log *[]BattleLogEntry) {
		volatileLeechSeed(a, b, log)
		volatileLeechSeed(b, a, log)
	}},
	{"ловушка", func(a, b *BattlePokemon, log *[]BattleLogEntry) {
		volatileTrap(a, log)
		volatileTrap(b, log)
	}},
	{"проклятие", func(a, b *BattlePokemon, log *[]BattleLogEntry) {
		volatileCurse(a, log)
		volatileCurse(b, log)
	}},
	{"кошмары", func(a, b *BattlePokemon, log *[]BattleLogEntry) {
		volatileNightmare(a, log)
		volatileNightmare(b, log)
	}},
	{"лечение-поля", func(a, b *BattlePokemon, log *[]BattleLogEntry) {
		volatileHealOverTime(a, log) // Укоренение / Водный покров (heal_moves.go)
		volatileHealOverTime(b, log)
	}},
}

// runEndTurnVolatiles прогоняет по-ходовые volatile-эффекты для пары покемонов
// в том же порядке, что и раньше: leech(a,b), leech(b,a), trap(a), trap(b).
func runEndTurnVolatiles(a, b *BattlePokemon, log *[]BattleLogEntry) {
	for _, v := range endTurnVolatiles {
		v.apply(a, b, log)
	}
}
