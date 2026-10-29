package api

import (
	"database/sql"
	"math"
	"math/rand"
	"pokemon-game/server/db"
	"strconv"
	"strings"
	"time"
)

type WildPokemonGenerate struct {
	PokemonID       int
	Name            string
	Level           int
	ImageURL        string
	PrimaryTypeID   int
	SecondaryTypeID *int
	MaxHP           int
	Attack          int
	Defense         int
	Speed           int
	SpecialAttack   int
	SpecialDefense  int
	Weight          float64
	NatureID        int
	NatureName      string
	NatureInc       string
	NatureDec       string
	// IV дикого — порядок как везде: HP,Atk,Def,Spd,SpAtk,SpDef. Протягивается
	// в BattleState и сохраняется при поимке (иначе IV/натура терялись).
	IV    [6]int
	Moves []GeneratedMove
}

type GeneratedMove struct {
	ID           int
	Name         string
	EngName      string
	TypeID       int
	CategoryID   int
	Power        int
	Accuracy     int
	PP           int
	Priority     int
	TargetID     int
	Description  string
	EffectID     int
	EffectChance int
	NeverMisses  bool
	Disabled     bool
	Drain        int
	HealPercent  int
	CritRate     int
	HitsMin      int
	HitsMax      int
	Contact      bool
	Sound        bool
}

type wildCandidate struct {
	ID       int
	MinLevel int
	MaxLevel int
	Weight   int
}

func generateWildPokemon(locationID int) *WildPokemonGenerate {
	// Выбираем ВИД из локации с учётом rarity (вес), уровень — из
	// min_level..max_level. Дальше — общее ядро buildGenerateFor.
	rows, err := db.DB.Query(`
		SELECT pl.pokemon_id,
		       COALESCE(pl.min_level, 1),
		       COALESCE(pl.max_level, 20),
		       COALESCE(pl.rarity, '')
		FROM pokemon_locations pl
		WHERE pl.location_id = ?
	`, locationID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var candidates []wildCandidate
	totalWeight := 0
	for rows.Next() {
		var c wildCandidate
		var rarity string
		if err := rows.Scan(&c.ID, &c.MinLevel, &c.MaxLevel, &rarity); err != nil {
			continue
		}
		c.Weight = rarityWeight(rarity)
		totalWeight += c.Weight
		candidates = append(candidates, c)
	}
	if len(candidates) == 0 || totalWeight <= 0 {
		return nil
	}

	roll := rand.Intn(totalWeight)
	chosen := candidates[len(candidates)-1]
	for _, c := range candidates {
		roll -= c.Weight
		if roll < 0 {
			chosen = c
			break
		}
	}

	minLevel, maxLevel := chosen.MinLevel, chosen.MaxLevel
	if minLevel < 1 {
		minLevel = 1
	}
	if maxLevel < minLevel {
		maxLevel = minLevel
	}
	if maxLevel > 100 {
		maxLevel = 100
	}
	level := minLevel + rand.Intn(maxLevel-minLevel+1)
	return buildGenerateFor(chosen.ID, level)
}

// rarityWeight превращает rarity в вес: больше = чаще.
// Поддерживает числа ('98') и текстовые метки; мусор/пусто = 50.
func rarityWeight(raw string) int {
	s := strings.TrimSpace(strings.ToLower(raw))
	if s == "" {
		return 50
	}
	if n, err := strconv.Atoi(s); err == nil {
		if n < 1 {
			return 1
		}
		if n > 1000 {
			return 1000
		}
		return n
	}
	switch s {
	case "common", "обычный", "часто":
		return 80
	case "uncommon", "необычный":
		return 50
	case "rare", "редкий":
		return 20
	case "very_rare", "very rare", "очень редкий":
		return 5
	case "legendary", "легендарный":
		return 1
	default:
		return 50
	}
}

// buildGenerateFor — построить «сгенерированного» покемона по виду и уровню:
// базовые статы вида -> IV/натура/статы/атаки. Используется и диким боем
// (generateWildPokemon), и командой NPC (поток B). Возвращает nil, если вида нет.
func buildGenerateFor(pokemonID, level int) *WildPokemonGenerate {
	if level < 1 {
		level = 1
	}
	var name, imageURL string
	var primaryTypeID int
	var secondaryTypeID sql.NullInt64
	var baseHP, baseAtk, baseDef, baseSpd, baseSpAtk, baseSpDef int
	var weight float64
	err := db.DB.QueryRow(`
		SELECT p.name, p.image_url, p.primary_type_id, p.secondary_type_id,
		       p.base_hp, p.base_attack, p.base_defense, p.base_speed,
		       p.base_special_attack, p.base_special_defense, COALESCE(p.weight, 0)
		FROM pokemons p WHERE p.id = ?
	`, pokemonID).Scan(&name, &imageURL, &primaryTypeID, &secondaryTypeID,
		&baseHP, &baseAtk, &baseDef, &baseSpd, &baseSpAtk, &baseSpDef, &weight)
	if err != nil {
		return nil
	}

	ivHP := rand.Intn(32)
	ivAtk := rand.Intn(32)
	ivDef := rand.Intn(32)
	ivSpd := rand.Intn(32)
	ivSpAtk := rand.Intn(32)
	ivSpDef := rand.Intn(32)

	var natureID int
	var natureName, natureInc, natureDec string
	err = db.DB.QueryRow(`
		SELECT id, name, COALESCE(increased_stat,''), COALESCE(decreased_stat,'')
		FROM natures ORDER BY RANDOM() LIMIT 1
	`).Scan(&natureID, &natureName, &natureInc, &natureDec)
	if err != nil {
		natureID = 13
		natureName = "Обычный"
	}

	stat := func(base, iv int, inc, dec, statName string) int {
		val := ((2*base + iv) * level / 100) + 5
		if inc == statName {
			val = int(math.Floor(float64(val) * 1.1))
		} else if dec == statName {
			val = int(math.Floor(float64(val) * 0.9))
		}
		return val
	}

	maxHP := ((2*baseHP + ivHP) * level / 100) + level + 10
	attack := stat(baseAtk, ivAtk, natureInc, natureDec, "attack")
	defense := stat(baseDef, ivDef, natureInc, natureDec, "defense")
	speed := stat(baseSpd, ivSpd, natureInc, natureDec, "speed")
	specialAttack := stat(baseSpAtk, ivSpAtk, natureInc, natureDec, "special_attack")
	specialDefense := stat(baseSpDef, ivSpDef, natureInc, natureDec, "special_defense")

	moves := generateWildMoves(pokemonID, level)

	var secType *int
	if secondaryTypeID.Valid {
		st := int(secondaryTypeID.Int64)
		secType = &st
	}

	return &WildPokemonGenerate{
		PokemonID:       pokemonID,
		Name:            name,
		Level:           level,
		ImageURL:        imageURL,
		PrimaryTypeID:   primaryTypeID,
		SecondaryTypeID: secType,
		MaxHP:           maxHP,
		Attack:          attack,
		Defense:         defense,
		Speed:           speed,
		SpecialAttack:   specialAttack,
		SpecialDefense:  specialDefense,
		Weight:          weight,
		NatureID:        natureID,
		NatureName:      natureName,
		NatureInc:       natureInc,
		NatureDec:       natureDec,
		IV:              [6]int{ivHP, ivAtk, ivDef, ivSpd, ivSpAtk, ivSpDef},
		Moves:           moves,
	}
}

func generateWildMoves(pokemonID, level int) []GeneratedMove {
	rows, err := db.DB.Query(`
		SELECT a.id, a.name, COALESCE(a.eng_name, ''), a.type_id, a.category_id, a.power, a.accuracy, a.pp,
		       a.priority,
		       COALESCE(a.target_id, 1), COALESCE(a.description, ''),
		       COALESCE(a.effect_status_id, 0), COALESCE(a.effect_chance, 0),
		       COALESCE(a.never_misses, 0), COALESCE(a.drain, 0),
		       COALESCE(a.crit_rate, 0), COALESCE(a.hits_min, 1), COALESCE(a.hits_max, 1),
		       COALESCE(a.disabled, 0), COALESCE(a.heal_percent, 0), COALESCE(a.contact, 0),
		       COALESCE(a.sound, 0)
		FROM pokemon_attacks pa
		JOIN attacks a ON pa.attack_id = a.id
		WHERE pa.pokemon_id = ? AND pa.learn_method_id = 1 AND pa.level_learned_at <= ?
		ORDER BY pa.level_learned_at
	`, pokemonID, level)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var all []GeneratedMove
	for rows.Next() {
		var m GeneratedMove
		var neverMisses, disabled, contact, sound int
		if err := rows.Scan(&m.ID, &m.Name, &m.EngName, &m.TypeID, &m.CategoryID, &m.Power, &m.Accuracy, &m.PP, &m.Priority, &m.TargetID, &m.Description, &m.EffectID, &m.EffectChance, &neverMisses, &m.Drain, &m.CritRate, &m.HitsMin, &m.HitsMax, &disabled, &m.HealPercent, &contact, &sound); err != nil {
			continue
		}
		m.NeverMisses = neverMisses != 0
		m.Disabled = disabled != 0
		m.Contact = contact != 0
		m.Sound = sound != 0
		all = append(all, m)
	}
	if len(all) == 0 {
		return nil
	}

	// Pick up to 4 random ones (or all if < 4)
	rand.Shuffle(len(all), func(i, j int) { all[i], all[j] = all[j], all[i] })
	if len(all) > 4 {
		all = all[:4]
	}
	return all
}

// Used to seed the random generator once at startup
func init() {
	rand.Seed(time.Now().UnixNano())
}
