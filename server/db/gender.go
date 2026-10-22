package db

import (
	"database/sql"
	mathrand "math/rand"
)

// gender.go — ЕДИНАЯ точка правды по полу и «букве привлекательности»
// конкретного экземпляра покемона (user_pokemons).
//
// Пол ('M'/'F'/'N') назначается ОДИН раз при создании экземпляра по ратио вида
// (pokemons.male_percentage/female_percentage). Буква ('A'/'B'/'C') назначается
// случайно при создании и ведёт себя как IV (у двух особей одного вида может
// отличаться). Обе величины хранятся на user_pokemons.
//
// ВАЖНО: любая будущая точка создания экземпляра (ловля дикого, разведение и т.п.)
// ОБЯЗАНА звать RollGender + RollAttractiveness, а не дублировать логику.

// Коды пола.
const (
	GenderMale     = "M"
	GenderFemale   = "F"
	GenderNeutral  = "N" // бесполый (обе доли ратио = 0)
)

// attractivenessLetters — допустимые буквы и их веса при случайном розыгрыше.
// По умолчанию равновероятно (1/3 каждая). Владелец может перекрутить веса
// ЗДЕСЬ — это единственное место.
var attractivenessLetters = []struct {
	Letter string
	Weight int
}{
	{"A", 1},
	{"B", 1},
	{"C", 1},
}

// RollGender возвращает пол ('M'/'F'/'N') для НОВОГО экземпляра вида pokemonID
// по ратио вида. Логика совпадает с бэкфиллом блока A:
//   - обе доли 0            -> 'N' (бесполый);
//   - иначе roll 0..99; roll < male_percentage -> 'M', иначе 'F'
//     (при (100,0) всегда 'M', при (0,100) всегда 'F').
func RollGender(pokemonID int) (string, error) {
	var male, female sql.NullInt64
	err := DB.QueryRow(
		"SELECT male_percentage, female_percentage FROM pokemons WHERE id = ?",
		pokemonID,
	).Scan(&male, &female)
	if err != nil {
		return "", err
	}
	m := int(male.Int64)
	f := int(female.Int64)
	if m == 0 && f == 0 {
		return GenderNeutral, nil
	}
	if mathrand.Intn(100) < m {
		return GenderMale, nil
	}
	return GenderFemale, nil
}

// RollAttractiveness возвращает случайную букву 'A'/'B'/'C' по весам из
// attractivenessLetters (по умолчанию равновероятно).
func RollAttractiveness() string {
	total := 0
	for _, l := range attractivenessLetters {
		total += l.Weight
	}
	if total <= 0 {
		return "A"
	}
	roll := mathrand.Intn(total)
	for _, l := range attractivenessLetters {
		if roll < l.Weight {
			return l.Letter
		}
		roll -= l.Weight
	}
	return attractivenessLetters[len(attractivenessLetters)-1].Letter
}

// UserPokemon — минимальный слепок экземпляра для правил, не завязанных на бой
// (сейчас — только CanBreed). Расширяется по мере надобности.
type UserPokemon struct {
	ID             int
	PokemonID      int
	Gender         string // 'M'/'F'/'N'
	Attractiveness string // 'A'/'B'/'C'
	HeldEffect     string // effect_type надетого предмета ('' если ничего); для Амурита == AmuriteEffect
}

// AmuriteEffect — метка предмета «Амурит» (items.effect_type). По ней опознаём
// Амурит на бесполых; отдельных колонок под него нет.
const AmuriteEffect = "breed_amurite"

// CanBreed — ЗАГЛУШКА-правило совместимости под будущее спаривание.
// Механика разведения (яйца/потомство) будет позже; здесь ТОЛЬКО правило,
// чтобы оно жило в одном месте и не размазывалось по коду.
//
// true только если:
//   - буквы привлекательности равны (A+A / B+B / C+C), И
//   - полы противоположны ('M' vs 'F').
// Бесполые ('N') не спариваются никогда.
func CanBreed(a, b *UserPokemon) bool {
	if a == nil || b == nil {
		return false
	}
	if a.Gender == GenderNeutral || b.Gender == GenderNeutral {
		return false
	}
	if a.Attractiveness != b.Attractiveness {
		return false
	}
	// противоположные полы: ровно один 'M' и один 'F'
	return (a.Gender == GenderMale && b.Gender == GenderFemale) ||
		(a.Gender == GenderFemale && b.Gender == GenderMale)
}

// BreedCompat — правило совместимости пары для РЕАЛЬНОГО разведения (поток [13]).
// Возвращает (можно ли, причина-отказ на русском). Требования (решение владельца):
//   - это два РАЗНЫХ экземпляра;
//   - ОДИН вид (pokemon_id равны) — потомок будет базовой формой этого вида;
//   - одна БУКВА привлекательности;
//   - пол: либо один 'M' + один 'F'; либо оба 'N', но тогда на ОБОИХ надет Амурит.
//
// Любой иной случай (разные виды, разные буквы, оба 'M'/оба 'F', один 'N' без пары,
// 'N' без Амурита и т.п.) — нельзя, с понятной причиной.
func BreedCompat(a, b *UserPokemon) (bool, string) {
	if a == nil || b == nil {
		return false, "не найден покемон"
	}
	if a.ID == b.ID {
		return false, "нужны два разных покемона"
	}
	if a.PokemonID != b.PokemonID {
		return false, "нужны покемоны одного вида"
	}
	if a.Attractiveness != b.Attractiveness {
		return false, "буквы привлекательности не совпадают"
	}
	// Оба бесполые — только через Амурит на обоих.
	if a.Gender == GenderNeutral && b.Gender == GenderNeutral {
		if a.HeldEffect == AmuriteEffect && b.HeldEffect == AmuriteEffect {
			return true, ""
		}
		return false, "бесполых разводит только Амурит на обоих"
	}
	// Обычная пара: ровно один 'M' и один 'F' (без 'N').
	if (a.Gender == GenderMale && b.Gender == GenderFemale) ||
		(a.Gender == GenderFemale && b.Gender == GenderMale) {
		return true, ""
	}
	return false, "нужны разнополые (♂ и ♀) или бесполые с Амуритом"
}
