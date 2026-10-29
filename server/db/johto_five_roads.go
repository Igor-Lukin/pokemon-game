package db

import "log"

// SeedEastJohtoFiveRoads идемпотентно создаёт регион 3 «Восточный Джотто —
// Пять дорог»: 41 локация, 42 двусторонние связи и 114 плейсхолдер-спавнов
// на 22 локациях. Регионы 1 (тест) и 2 (канон-ядро) не трогаются.
func SeedEastJohtoFiveRoads() {
	// Сидим только один раз: если регион 3 уже есть и в нём есть локации,
	// не перезаписываем правки из админки.
	var regionCount, locationCount int
	DB.QueryRow("SELECT COUNT(*) FROM regions WHERE id = 3").Scan(&regionCount)
	if regionCount > 0 {
		DB.QueryRow("SELECT COUNT(*) FROM locations WHERE region_id = 3").Scan(&locationCount)
		if locationCount > 0 {
			return
		}
	}

	tx, err := DB.Begin()
	if err != nil {
		log.Printf("five roads seed: begin: %v", err)
		return
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`INSERT OR IGNORE INTO regions (id, name, current_weather_id) VALUES (3, 'Восточный Джотто', 1)`); err != nil {
		log.Printf("five roads seed region: %v", err)
		return
	}

	for _, l := range fiveRoadsLocationSeeds {
		if _, err := tx.Exec(`
			INSERT OR IGNORE INTO locations
				(id, name, description, has_nursery, has_healing, region_id)
			VALUES (?, ?, ?, ?, ?, 3)
		`, l.ID, l.Name, l.Description, l.HasNursery, l.HasHealing); err != nil {
			log.Printf("five roads seed location %d: %v", l.ID, err)
			return
		}
	}

	addExit := func(from, to int) bool {
		_, err := tx.Exec(`
			INSERT INTO location_exits (from_location, to_location)
			SELECT ?, ?
			WHERE NOT EXISTS (
				SELECT 1 FROM location_exits
				WHERE from_location = ? AND to_location = ?
			)
		`, from, to, from, to)
		if err != nil {
			log.Printf("five roads seed exit %d->%d: %v", from, to, err)
			return false
		}
		return true
	}
	for _, e := range fiveRoadsExitSeeds {
		if !addExit(e.From, e.To) || !addExit(e.To, e.From) {
			return
		}
	}

	for _, s := range fiveRoadsSpawnSeeds {
		if _, err := tx.Exec(`
			INSERT OR IGNORE INTO pokemon_locations
				(pokemon_id, location_id, rarity, min_level, max_level)
			VALUES (?, ?, ?, ?, ?)
		`, s.PokemonID, s.LocationID, s.Rarity, s.MinLevel, s.MaxLevel); err != nil {
			log.Printf("five roads seed spawn %d@%d: %v", s.PokemonID, s.LocationID, err)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		log.Printf("five roads seed commit: %v", err)
	}
}
