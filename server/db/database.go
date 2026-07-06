package db

import (
	"database/sql"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

func InitDB() {
	var err error
	DB, err = sql.Open("sqlite3", "./database/pokemon.db")
	if err != nil {
		log.Fatal(err)
	}
	createTables()
	insertInitialData()
}

func CloseDB() {
	DB.Close()
}

func createTables() {
	queries := []string{
		// ===== СУЩЕСТВУЮЩИЕ ТАБЛИЦЫ =====
		`CREATE TABLE IF NOT EXISTS locations (
            id INTEGER PRIMARY KEY,
            name TEXT NOT NULL,
            description TEXT NOT NULL,
            region TEXT,
            has_nursery INTEGER DEFAULT 0,
            has_healing INTEGER DEFAULT 0
        );`,
		`CREATE TABLE IF NOT EXISTS location_exits (
            from_location INTEGER,
            to_location INTEGER,
            FOREIGN KEY(from_location) REFERENCES locations(id),
            FOREIGN KEY(to_location) REFERENCES locations(id)
        );`,
		`CREATE TABLE IF NOT EXISTS npc (
            id INTEGER PRIMARY KEY,
            name TEXT NOT NULL,
            location_id INTEGER,
            FOREIGN KEY(location_id) REFERENCES locations(id)
        );`,
		`CREATE TABLE IF NOT EXISTS users (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            username TEXT NOT NULL UNIQUE,
            email TEXT NOT NULL UNIQUE,
            password TEXT NOT NULL,
            location_id INTEGER DEFAULT 1,
            is_admin INTEGER DEFAULT 0,
            FOREIGN KEY(location_id) REFERENCES locations(id)
        );`,

		// ===== НОВЫЕ ТАБЛИЦЫ ДЛЯ ПОКЕМОНОВ =====
		`CREATE TABLE IF NOT EXISTS types (
            id INTEGER PRIMARY KEY,
            name TEXT NOT NULL UNIQUE
        );`,
		`CREATE TABLE IF NOT EXISTS attack_categories (
            id INTEGER PRIMARY KEY,
            name TEXT NOT NULL UNIQUE   -- 'physical', 'special', 'status'
        );`,
		`CREATE TABLE IF NOT EXISTS attack_targets (
            id INTEGER PRIMARY KEY,
            name TEXT NOT NULL UNIQUE   -- 'enemy', 'self', 'ally'
        );`,
		`CREATE TABLE IF NOT EXISTS evolution_methods (
            id INTEGER PRIMARY KEY,
            name TEXT NOT NULL UNIQUE   -- 'level', 'trade', 'stone', 'friendship', 'item'
        );`,
		`CREATE TABLE IF NOT EXISTS type_effectiveness (
            attacking_type_id INTEGER NOT NULL,
            defending_type_id INTEGER NOT NULL,
            multiplier REAL NOT NULL DEFAULT 1,
            PRIMARY KEY(attacking_type_id, defending_type_id),
            FOREIGN KEY(attacking_type_id) REFERENCES types(id),
            FOREIGN KEY(defending_type_id) REFERENCES types(id)
        );`,
		`CREATE TABLE IF NOT EXISTS attack_learn_methods (
            id INTEGER PRIMARY KEY,
            name TEXT NOT NULL UNIQUE   -- 'level-up', 'tm', 'egg', 'tutor'
        );`,

		`CREATE TABLE IF NOT EXISTS pokemons (
            id INTEGER PRIMARY KEY,
            name TEXT NOT NULL UNIQUE,
            image_url TEXT,
            description TEXT,
            height REAL,
            weight REAL,
            base_hp INTEGER,
            base_attack INTEGER,
            base_defense INTEGER,
            base_speed INTEGER,
            base_special_attack INTEGER,
            base_special_defense INTEGER,
            category_power INTEGER,
            experience_group INTEGER,
            rarity INTEGER,
            female_percentage INTEGER,
            male_percentage INTEGER,
            primary_type_id INTEGER,
            secondary_type_id INTEGER,
            evolves_from_id INTEGER,
            evolution_method_id INTEGER,
            evolution_level INTEGER,
            evolution_item_id INTEGER,
            FOREIGN KEY(primary_type_id) REFERENCES types(id),
            FOREIGN KEY(secondary_type_id) REFERENCES types(id),
            FOREIGN KEY(evolves_from_id) REFERENCES pokemons(id),
            FOREIGN KEY(evolution_method_id) REFERENCES evolution_methods(id)
        );`,

		`CREATE TABLE IF NOT EXISTS abilities (
            id INTEGER PRIMARY KEY,
            name TEXT NOT NULL UNIQUE,
            description TEXT
        );`,

		// Таблица статусов
		`CREATE TABLE IF NOT EXISTS statuses (
            id INTEGER PRIMARY KEY,
            name TEXT NOT NULL UNIQUE,
            description TEXT
        );`,

		// Таблица характеров
		`CREATE TABLE IF NOT EXISTS natures (
            id INTEGER PRIMARY KEY,
            name TEXT NOT NULL UNIQUE,
            increased_stat TEXT,   -- может быть NULL для нейтральных характеров
            decreased_stat TEXT
        );`,

		`CREATE TABLE IF NOT EXISTS pokemon_abilities (
            pokemon_id INTEGER,
            ability_id INTEGER,
            is_hidden BOOLEAN DEFAULT 0,
            PRIMARY KEY(pokemon_id, ability_id),
            FOREIGN KEY(pokemon_id) REFERENCES pokemons(id),
            FOREIGN KEY(ability_id) REFERENCES abilities(id)
        );`,

		`CREATE TABLE IF NOT EXISTS attacks (
            id INTEGER PRIMARY KEY,
            name TEXT NOT NULL UNIQUE,
            type_id INTEGER,
            power INTEGER,
            accuracy INTEGER,
            pp INTEGER,
            category_id INTEGER,
            target_id INTEGER,
            priority INTEGER DEFAULT 0,
            description TEXT,
            tm_number INTEGER,
            eng_name TEXT,
            contact BOOLEAN DEFAULT 0,
            sound BOOLEAN DEFAULT 0,
            effect_status_id INTEGER REFERENCES statuses(id),
            effect_chance INTEGER DEFAULT 0,
            FOREIGN KEY(type_id) REFERENCES types(id),
            FOREIGN KEY(category_id) REFERENCES attack_categories(id),
            FOREIGN KEY(target_id) REFERENCES attack_targets(id)
        );`,

		`CREATE TABLE IF NOT EXISTS attack_effects (
            id INTEGER PRIMARY KEY,
            attack_id INTEGER NOT NULL,
            effect_description TEXT,
            chance INTEGER,
            FOREIGN KEY(attack_id) REFERENCES attacks(id) ON DELETE CASCADE
        );`,

		`CREATE TABLE IF NOT EXISTS pokemon_attacks (
            pokemon_id INTEGER,
            attack_id INTEGER,
            learn_method_id INTEGER,
            level_learned_at INTEGER,
            PRIMARY KEY(pokemon_id, attack_id, learn_method_id),
            FOREIGN KEY(pokemon_id) REFERENCES pokemons(id),
            FOREIGN KEY(attack_id) REFERENCES attacks(id),
            FOREIGN KEY(learn_method_id) REFERENCES attack_learn_methods(id)
        );`,

		`CREATE TABLE IF NOT EXISTS pokemon_locations (
            pokemon_id INTEGER,
            location_id INTEGER,
            rarity TEXT,
            PRIMARY KEY(pokemon_id, location_id),
            FOREIGN KEY(pokemon_id) REFERENCES pokemons(id),
            FOREIGN KEY(location_id) REFERENCES locations(id)
        );`,

		`CREATE TABLE IF NOT EXISTS user_pokemons (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            user_id INTEGER NOT NULL,
            pokemon_id INTEGER NOT NULL,
            nickname TEXT,
            level INTEGER DEFAULT 1,
            experience INTEGER DEFAULT 0,
            current_hp INTEGER,
            max_hp INTEGER,
            attack INTEGER,
            defense INTEGER,
            speed INTEGER,
            special_attack INTEGER,
            special_defense INTEGER,
            iv_hp INTEGER DEFAULT 0,
            iv_attack INTEGER DEFAULT 0,
            iv_defense INTEGER DEFAULT 0,
            iv_speed INTEGER DEFAULT 0,
            iv_special_attack INTEGER DEFAULT 0,
            iv_special_defense INTEGER DEFAULT 0,
            ev_hp INTEGER DEFAULT 0,
            ev_attack INTEGER DEFAULT 0,
            ev_defense INTEGER DEFAULT 0,
            ev_speed INTEGER DEFAULT 0,
            ev_special_attack INTEGER DEFAULT 0,
            ev_special_defense INTEGER DEFAULT 0,
            available_ev INTEGER DEFAULT 0,   -- нераспределённые очки EV
            position INTEGER DEFAULT 0,
            status_id INTEGER,
            nature_id INTEGER,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            FOREIGN KEY(user_id) REFERENCES users(id),
            FOREIGN KEY(pokemon_id) REFERENCES pokemons(id),
            FOREIGN KEY(status_id) REFERENCES statuses(id),
            FOREIGN KEY(nature_id) REFERENCES natures(id)
	);`,
        `CREATE TABLE IF NOT EXISTS user_pokemon_learnable_attacks (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            user_pokemon_id INTEGER NOT NULL,
            attack_id INTEGER NOT NULL,
            level_available INTEGER NOT NULL,
            is_learned BOOLEAN DEFAULT 0,
            position INTEGER,
            current_pp INTEGER DEFAULT 0,
            UNIQUE(user_pokemon_id, attack_id),
            FOREIGN KEY(user_pokemon_id) REFERENCES user_pokemons(id) ON DELETE CASCADE,
            FOREIGN KEY(attack_id) REFERENCES attacks(id)
        );`,

		// ===== Регионы =====
		`CREATE TABLE IF NOT EXISTS regions (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL UNIQUE
		);`,

		// ===== Погода =====
		`CREATE TABLE IF NOT EXISTS weather (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL UNIQUE
		);`,

		// ===== Изменения статов атак =====
		`CREATE TABLE IF NOT EXISTS attack_stat_changes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			attack_id INTEGER NOT NULL,
			stat_id INTEGER NOT NULL,
			stages INTEGER NOT NULL,
			target INTEGER NOT NULL DEFAULT 0,
			chance INTEGER NOT NULL DEFAULT 100,
			FOREIGN KEY(attack_id) REFERENCES attacks(id) ON DELETE CASCADE
		);`,

		// ===== ИНВЕНТАРЬ =====
		`CREATE TABLE IF NOT EXISTS item_categories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE
		);`,
		`CREATE TABLE IF NOT EXISTS items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			description TEXT,
			category_id INTEGER NOT NULL,
			price INTEGER DEFAULT 0,
			effect_type TEXT,
			effect_value INTEGER,
			image_url TEXT,
			FOREIGN KEY(category_id) REFERENCES item_categories(id)
		);`,
		`CREATE TABLE IF NOT EXISTS user_items (
			user_id INTEGER NOT NULL,
			item_id INTEGER NOT NULL,
			quantity INTEGER DEFAULT 0,
			PRIMARY KEY(user_id, item_id),
			FOREIGN KEY(user_id) REFERENCES users(id),
			FOREIGN KEY(item_id) REFERENCES items(id)
		);`,
	}

	for _, q := range queries {
		if _, err := DB.Exec(q); err != nil {
			log.Fatal("Error creating table:", err, "\nQuery:", q)
		}
	}

	// Миграции для существующих таблиц
	DB.Exec("ALTER TABLE locations ADD COLUMN has_nursery INTEGER DEFAULT 0")
	DB.Exec("ALTER TABLE locations ADD COLUMN has_healing INTEGER DEFAULT 0")
	DB.Exec("ALTER TABLE locations ADD COLUMN region_id INTEGER DEFAULT 1 REFERENCES regions(id)")
	DB.Exec("ALTER TABLE user_pokemon_learnable_attacks ADD COLUMN current_pp INTEGER DEFAULT 0")
	DB.Exec("ALTER TABLE attacks ADD COLUMN never_misses INTEGER DEFAULT 0")
	DB.Exec("ALTER TABLE attacks ADD COLUMN recoil INTEGER DEFAULT 0")
	DB.Exec("ALTER TABLE attacks ADD COLUMN drain INTEGER DEFAULT 0")
	DB.Exec("ALTER TABLE attacks ADD COLUMN crit_rate INTEGER DEFAULT 0")
	DB.Exec("ALTER TABLE attacks ADD COLUMN hits_min INTEGER DEFAULT 1")
	DB.Exec("ALTER TABLE attacks ADD COLUMN hits_max INTEGER DEFAULT 1")
	DB.Exec("ALTER TABLE regions ADD COLUMN current_weather_id INTEGER DEFAULT 1 REFERENCES weather(id)")

	// Copy recoil → drain with negated values (recoil was positive, drain negative = отдача)
	DB.Exec("UPDATE attacks SET drain = -recoil WHERE recoil > 0")

	// Drop old recoil column
	DB.Exec("ALTER TABLE attacks DROP COLUMN recoil")

	// Set current_pp for existing rows to attacks.pp
	DB.Exec(`
		UPDATE user_pokemon_learnable_attacks
		SET current_pp = (SELECT pp FROM attacks WHERE attacks.id = user_pokemon_learnable_attacks.attack_id)
		WHERE current_pp = 0
	`)

	// Удаляем устаревшие записи
	DB.Exec("DELETE FROM statuses WHERE id = 9") // Путы
	DB.Exec("DELETE FROM statuses WHERE id = 12") // дубль Испуг

	// Новые эффекты атак (погода)
	for _, s := range []struct {
		id   int
		name string
		desc string
	}{
		{14, "Солнечная погода", "Меняет погоду на солнечную"},
		{15, "Дождь", "Меняет погоду на дождливую"},
		{16, "Град", "Меняет погоду на град"},
		{17, "Песчаная буря", "Меняет погоду на песчаную бурю"},
	} {
		DB.Exec("INSERT OR IGNORE INTO statuses (id, name, description) VALUES (?, ?, ?)", s.id, s.name, s.desc)
	}

	// Категория "candy" и предмет "Леденец"
	DB.Exec("INSERT OR IGNORE INTO item_categories (name) VALUES ('candy')")
	DB.Exec(`
		INSERT OR IGNORE INTO items (id, name, description, category_id, price, effect_type, effect_value, image_url)
		VALUES (?, ?, ?, (SELECT id FROM item_categories WHERE name = 'candy'), ?, ?, ?, ?)
	`, 100, "Леденец", "Повышает уровень покемона на 1 и даёт +2 EV", 0, "candy", 1, "/static/images/items/41.png")
}

// Начальные данные для справочных таблиц
func insertInitialData() {
	// Типы покемонов (базовые)
	types := []string{"Normal", "Fire", "Water", "Electric", "Grass", "Ice", "Fighting", "Poison", "Ground", "Flying", "Psychic", "Bug", "Rock", "Ghost", "Dragon", "Dark", "Steel", "Fairy"}
	for _, t := range types {
		DB.Exec("INSERT OR IGNORE INTO types (name) VALUES (?)", t)
	}

	// Категории атак
	categories := []string{"physical", "special", "status"}
	for _, c := range categories {
		DB.Exec("INSERT OR IGNORE INTO attack_categories (name) VALUES (?)", c)
	}

	// Цели атак
	targets := []string{"enemy", "self", "ally"}
	for _, t := range targets {
		DB.Exec("INSERT OR IGNORE INTO attack_targets (name) VALUES (?)", t)
	}

	// Методы эволюции
	evoMethods := []string{"level", "trade", "stone", "friendship", "item"}
	for _, m := range evoMethods {
		DB.Exec("INSERT OR IGNORE INTO evolution_methods (name) VALUES (?)", m)
	}

	// Методы изучения атак
	learnMethods := []string{"level-up", "tm", "egg", "tutor"}
	for _, m := range learnMethods {
		DB.Exec("INSERT OR IGNORE INTO attack_learn_methods (name) VALUES (?)", m)
	}

	// Статусы покемонов
	statuses := []struct {
		id   int
		name string
		desc string
	}{
		{1, "Здоров", "Нет статуса"},
		{2, "Паралич", "Скорость снижена, 25% пропуск хода"},
		{3, "Ожог", "Атака снижена, урон каждый ход"},
		{4, "Заморозка", "Невозможно ходить"},
		{5, "Сон", "Невозможно ходить 1-3 хода"},
		{6, "Отравление", "Урон каждый ход"},
		{7, "Сильное отравление", "Урон растёт каждый ход"},
		{8, "Испуг", "Если покемон напуган, атака прерывается."},
		{10, "Семена-пиявки", "Отнимают 1/16 от максимальных HP противника и присваивают их пользователю."},
		{11, "Ловушка", "Запрещает покемону противника покидать битву, наносит 1/16 урона от максимального здоровья."},
		{13, "Замешательство", "Покемон атакует сам себя"},
		{14, "Солнечная погода", "Меняет погоду на солнечную"},
		{15, "Дождь", "Меняет погоду на дождливую"},
		{16, "Град", "Меняет погоду на град"},
		{17, "Песчаная буря", "Меняет погоду на песчаную бурю"},
	}
	for _, s := range statuses {
		DB.Exec("INSERT OR IGNORE INTO statuses (id, name, description) VALUES (?, ?, ?)", s.id, s.name, s.desc)
	}

	// Если таблица locations пуста, добавим стартовую локацию (лес)
	var count int
	DB.QueryRow("SELECT COUNT(*) FROM locations").Scan(&count)
	if count == 0 {
		DB.Exec("INSERT INTO locations (id, name, description, has_nursery, has_healing) VALUES (1, 'Лес Пикачу', 'Густой лес с высокими деревьями...', 0, 0)")
		DB.Exec("INSERT INTO locations (id, name, description, has_nursery, has_healing) VALUES (2, 'Дорога к городу', 'Пыльная дорога, ведущая к городу', 0, 0)")
		DB.Exec("INSERT INTO locations (id, name, description, has_nursery, has_healing) VALUES (3, 'Пещера Зубата', 'Тёмная пещера, где обитают летучие мыши', 0, 0)")
		DB.Exec("INSERT INTO location_exits (from_location, to_location) VALUES (1,2), (1,3), (2,1), (3,1)")
	}

	natures := []struct {
		id   int
		name string
		inc  string
		dec  string
	}{
		{1, "Веселый", "speed", "special_attack"},
		{2, "Выносливый", "", ""},
		{3, "Застенчивый", "", ""},
		{4, "Кроткий", "", ""},
		{5, "Мирный", "special_defense", "attack"},
		{6, "Мягкий", "special_attack", "defense"},
		{7, "Наглый", "defense", "attack"},
		{8, "Наивный", "speed", "special_defense"},
		{9, "Нахальный", "special_defense", "speed"},
		{10, "Нежный", "special_defense", "defense"},
		{11, "Непослушный", "attack", "special_defense"},
		{12, "Непреклонный", "attack", "special_attack"},
		{14, "Одинокий", "attack", "defense"},
		{15, "Озорной", "defense", "special_attack"},
		{16, "Осторожный", "special_defense", "special_attack"},
		{17, "Поспешный", "speed", "defense"},
		{19, "Распущенный", "defense", "special_defense"},
		{20, "Робкий", "speed", "attack"},
		{22, "Скромный", "special_attack", "attack"},
		{23, "Смелый", "attack", "speed"},
		{24, "Спокойный", "defense", "speed"},
		{25, "Стремительный", "special_attack", "special_defense"},
		{26, "Тихий", "special_attack", "speed"},
	}
	for _, n := range natures {
		var incPtr, decPtr interface{}
		if n.inc == "" {
			incPtr = nil
		} else {
			incPtr = n.inc
		}
		if n.dec == "" {
			decPtr = nil
		} else {
			decPtr = n.dec
		}
		DB.Exec("INSERT OR IGNORE INTO natures (id, name, increased_stat, decreased_stat) VALUES (?, ?, ?, ?)", n.id, n.name, incPtr, decPtr)
	}

	// Эффективность типов (attacking_type_id, defending_type_id, multiplier)
	type typeEff struct {
		atk, def int
		mult     float64
	}
	effs := []typeEff{
		// Normal
		{1, 13, 0.5}, {1, 14, 0}, {1, 17, 0.5},
		// Fire
		{2, 2, 0.5}, {2, 3, 0.5}, {2, 5, 2}, {2, 6, 2}, {2, 12, 2}, {2, 13, 0.5}, {2, 15, 0.5}, {2, 17, 2},
		// Water
		{3, 2, 2}, {3, 3, 0.5}, {3, 5, 0.5}, {3, 9, 2}, {3, 13, 2}, {3, 15, 0.5},
		// Electric
		{4, 3, 2}, {4, 4, 0.5}, {4, 5, 0.5}, {4, 9, 0}, {4, 10, 2}, {4, 15, 0.5},
		// Grass
		{5, 2, 0.5}, {5, 3, 2}, {5, 4, 1}, {5, 5, 0.5}, {5, 7, 0.5}, {5, 8, 0.5}, {5, 9, 2}, {5, 10, 0.5}, {5, 12, 0.5}, {5, 13, 2}, {5, 15, 0.5}, {5, 17, 0.5},
		// Ice
		{6, 2, 0.5}, {6, 3, 0.5}, {6, 5, 2}, {6, 6, 0.5}, {6, 9, 2}, {6, 10, 2}, {6, 15, 2}, {6, 17, 0.5},
		// Fighting
		{7, 1, 2}, {7, 6, 2}, {7, 8, 0.5}, {7, 10, 0.5}, {7, 11, 0.5}, {7, 13, 2}, {7, 14, 0}, {7, 16, 2}, {7, 17, 2}, {7, 18, 0.5},
		// Poison
		{8, 5, 2}, {8, 7, 1}, {8, 8, 0.5}, {8, 9, 0.5}, {8, 11, 1}, {8, 13, 0.5}, {8, 14, 0.5}, {8, 17, 0}, {8, 18, 2},
		// Ground
		{9, 2, 2}, {9, 4, 2}, {9, 5, 0.5}, {9, 8, 2}, {9, 10, 0}, {9, 12, 0.5}, {9, 13, 2}, {9, 17, 2},
		// Flying
		{10, 5, 2}, {10, 7, 2}, {10, 10, 1}, {10, 12, 2}, {10, 13, 0.5}, {10, 17, 0.5},
		// Psychic
		{11, 7, 2}, {11, 8, 2}, {11, 11, 0.5}, {11, 17, 0.5}, {11, 16, 0},
		// Bug
		{12, 2, 0.5}, {12, 5, 2}, {12, 7, 0.5}, {12, 8, 0.5}, {12, 10, 0.5}, {12, 11, 2}, {12, 14, 0.5}, {12, 16, 2}, {12, 17, 0.5}, {12, 18, 0.5},
		// Rock
		{13, 2, 2}, {13, 5, 1}, {13, 6, 2}, {13, 7, 0.5}, {13, 9, 0.5}, {13, 10, 2}, {13, 12, 2}, {13, 17, 0.5},
		// Ghost
		{14, 1, 0}, {14, 11, 2}, {14, 14, 2}, {14, 16, 0.5}, {14, 18, 1},
		// Dragon
		{15, 15, 2}, {15, 17, 0.5}, {15, 18, 0},
		// Dark
		{16, 7, 0.5}, {16, 11, 2}, {16, 14, 2}, {16, 16, 0.5}, {16, 18, 0.5},
		// Steel
		{17, 2, 0.5}, {17, 3, 0.5}, {17, 4, 0.5}, {17, 6, 2}, {17, 13, 2}, {17, 17, 0.5}, {17, 18, 2},
		// Fairy
		{18, 2, 0.5}, {18, 7, 2}, {18, 8, 0.5}, {18, 15, 2}, {18, 16, 2}, {18, 17, 0.5},
	}
	for _, e := range effs {
		DB.Exec("INSERT OR IGNORE INTO type_effectiveness (attacking_type_id, defending_type_id, multiplier) VALUES (?, ?, ?)", e.atk, e.def, e.mult)
	}

	// Регионы
	DB.Exec("INSERT OR IGNORE INTO regions (id, name) VALUES (1, 'Джотто')")

	// Назначить все существующие локации региону Джотто
	DB.Exec("UPDATE locations SET region_id = 1 WHERE region_id IS NULL OR region_id = 1")

	// Погода
	weatherTypes := []string{"Обычная", "Солнечно", "Дождь", "Град", "Песчаная буря"}
	for _, w := range weatherTypes {
		DB.Exec("INSERT OR IGNORE INTO weather (name) VALUES (?)", w)
	}

	// Установить случайную погоду для каждого региона при первом запуске
	DB.Exec("UPDATE regions SET current_weather_id = (ABS(RANDOM()) % 5) + 1")
}
