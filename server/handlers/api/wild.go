package api

import (
	"database/sql"
	"math"
	"math/rand"
	"pokemon-game/server/db"
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
	IV              [6]int
	Moves           []GeneratedMove
}

type GeneratedMove struct {
	ID             int
	Name           string
	EngName        string
	TypeID         int
	CategoryID     int
	Power          int
	Accuracy       int
	PP             int
	Priority       int
	TargetID       int
	Description    string
	EffectID int
	EffectChance   int
	NeverMisses    bool
	Disabled       bool
	Drain          int
	HealPercent    int
	CritRate       int
	HitsMin        int
	HitsMax        int
	Contact        bool
}

func generateWildPokemon(locationID int) *WildPokemonGenerate {
	rows, err := db.DB.Query(`
		SELECT p.id, p.name, p.image_url, p.primary_type_id, p.secondary_type_id,
		       p.base_hp, p.base_attack, p.base_defense, p.base_speed,
		       p.base_special_attack, p.base_special_defense, COALESCE(p.weight, 0)
		FROM pokemon_locations pl
		JOIN pokemons p ON pl.pokemon_id = p.id
		WHERE pl.location_id = ?
	`, locationID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	type candidate struct {
		id                int
		name              string
		imageURL          string
		primaryTypeID     int
		secondaryTypeID   sql.NullInt64
		baseHP, baseAtk, baseDef, baseSpd, baseSpAtk, baseSpDef int
		weight            float64
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.name, &c.imageURL, &c.primaryTypeID,
			&c.secondaryTypeID, &c.baseHP, &c.baseAtk, &c.baseDef,
			&c.baseSpd, &c.baseSpAtk, &c.baseSpDef, &c.weight); err != nil {
			continue
		}
		candidates = append(candidates, c)
	}
	if len(candidates) == 0 {
		return nil
	}

	chosen := candidates[rand.Intn(len(candidates))]

	level := 1 + rand.Intn(20) // 1–20

	ivHP := rand.Intn(6)
	ivAtk := rand.Intn(6)
	ivDef := rand.Intn(6)
	ivSpd := rand.Intn(6)
	ivSpAtk := rand.Intn(6)
	ivSpDef := rand.Intn(6)

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

	maxHP := ((2*chosen.baseHP + ivHP) * level / 100) + level + 10
	attack := stat(chosen.baseAtk, ivAtk, natureInc, natureDec, "attack")
	defense := stat(chosen.baseDef, ivDef, natureInc, natureDec, "defense")
	speed := stat(chosen.baseSpd, ivSpd, natureInc, natureDec, "speed")
	specialAttack := stat(chosen.baseSpAtk, ivSpAtk, natureInc, natureDec, "special_attack")
	specialDefense := stat(chosen.baseSpDef, ivSpDef, natureInc, natureDec, "special_defense")

	moves := generateWildMoves(chosen.id, level)

	var secType *int
	if chosen.secondaryTypeID.Valid {
		st := int(chosen.secondaryTypeID.Int64)
		secType = &st
	}

	return &WildPokemonGenerate{
		PokemonID:       chosen.id,
		Name:            chosen.name,
		Level:           level,
		ImageURL:        chosen.imageURL,
		PrimaryTypeID:   chosen.primaryTypeID,
		SecondaryTypeID: secType,
		MaxHP:           maxHP,
		Attack:          attack,
		Defense:         defense,
		Speed:           speed,
		SpecialAttack:   specialAttack,
		SpecialDefense:  specialDefense,
		Weight:          chosen.weight,
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
		       COALESCE(a.disabled, 0), COALESCE(a.heal_percent, 0), COALESCE(a.contact, 0)
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
