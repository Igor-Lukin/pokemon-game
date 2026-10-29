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
	seedEvolutionItems()
	seedHappinessBerry()
	seedHeldItems()
	seedBattleItems()
	SeedEastJohtoFiveRoads()
	BackfillLegendaryFlags()
	SeedPokemonForms()
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

		// Боевые/альтернативные формы (HP-порог или использованный ход).
		// trigger_key = abilities.effect_key; trigger_mode = 'hp' | 'move'.
		`CREATE TABLE IF NOT EXISTS pokemon_forms (
            pokemon_id INTEGER NOT NULL,
            form_key TEXT NOT NULL,
            trigger_key TEXT NOT NULL,
            form_name TEXT NOT NULL,
            form_label TEXT,
            image_url TEXT,
            primary_type_id INTEGER,
            secondary_type_id INTEGER,
            hp_threshold_percent INTEGER NOT NULL DEFAULT 50,
            hp_mode INTEGER NOT NULL DEFAULT 1,
            min_level INTEGER NOT NULL DEFAULT 0,
            attack_mult REAL NOT NULL DEFAULT 1,
            defense_mult REAL NOT NULL DEFAULT 1,
            speed_mult REAL NOT NULL DEFAULT 1,
            special_attack_mult REAL NOT NULL DEFAULT 1,
            special_defense_mult REAL NOT NULL DEFAULT 1,
            trigger_mode TEXT NOT NULL DEFAULT 'hp',
            trigger_move_id INTEGER NOT NULL DEFAULT 0,
            trigger_move_kind TEXT NOT NULL DEFAULT '',
            is_base_form INTEGER NOT NULL DEFAULT 0,
            form_order INTEGER NOT NULL DEFAULT 0,
            form_description TEXT,
            PRIMARY KEY(pokemon_id, form_key)
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
            min_level INTEGER DEFAULT 1,
            max_level INTEGER DEFAULT 20,
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
            is_starter INTEGER DEFAULT 0,     -- 1 = стартовый покемон (первым в бой)
            status_id INTEGER,
            nature_id INTEGER,
            gender TEXT,            -- пол экземпляра: 'M'/'F'/'N' (см. RollGender)
            attractiveness TEXT,    -- буква привлекательности: 'A'/'B'/'C' (см. RollAttractiveness)
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
            source TEXT DEFAULT 'level',        -- 'level' | 'tm' | 'hm'
            source_item_id INTEGER,              -- TM/HM item, которым открыта атака
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
			req_weather INTEGER NOT NULL DEFAULT 0,
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
			attack_id INTEGER,                  -- TM/HM: какую атаку открывает предмет
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

		// ===== КАРТОЧКА ТРЕНЕРА (поток [7]) =====
		// Справочник должностей игрока (отображаемая роль, отдельно от is_admin-прав).
		`CREATE TABLE IF NOT EXISTS player_roles (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL UNIQUE
		);`,
		// Друзья (симметрия — двумя строками при наполнении; показ только чтение).
		`CREATE TABLE IF NOT EXISTS user_friends (
			user_id INTEGER NOT NULL,
			friend_id INTEGER NOT NULL,
			PRIMARY KEY(user_id, friend_id),
			FOREIGN KEY(user_id) REFERENCES users(id),
			FOREIGN KEY(friend_id) REFERENCES users(id)
		);`,
		// Желаемые покемоны (только показ).
		`CREATE TABLE IF NOT EXISTS user_wishlist (
			user_id INTEGER NOT NULL,
			pokemon_id INTEGER NOT NULL,
			PRIMARY KEY(user_id, pokemon_id),
			FOREIGN KEY(user_id) REFERENCES users(id),
			FOREIGN KEY(pokemon_id) REFERENCES pokemons(id)
		);`,

		// ===== ДИАЛОГИ NPC (поток A) =====
		// Узлы диалога — реплики NPC. Ровно один is_start=1 на NPC.
		`CREATE TABLE IF NOT EXISTS dialogue_nodes (
			id INTEGER PRIMARY KEY,
			npc_id INTEGER NOT NULL,
			node_key TEXT,               -- удобный ключ узла в рамках NPC (авторинг/квесты)
			speaker_text TEXT NOT NULL,  -- что говорит NPC
			is_start INTEGER DEFAULT 0,  -- 1 = стартовый узел NPC (ровно один)
			FOREIGN KEY(npc_id) REFERENCES npc(id)
		);`,
		// Опции (ответы игрока) — рёбра графа диалога.
		`CREATE TABLE IF NOT EXISTS dialogue_options (
			id INTEGER PRIMARY KEY,
			node_id INTEGER NOT NULL,    -- к какому узлу относится ответ
			sort_order INTEGER DEFAULT 0,
			option_text TEXT NOT NULL,   -- текст кнопки-ответа
			next_node_id INTEGER,        -- куда ведёт (NULL = конец после действия)
			action_type TEXT,            -- NULL | 'battle' | 'close' (позже: give_item/set_flag/start_quest)
			action_param TEXT,           -- параметр действия (на будущее)
			condition_flag TEXT,         -- НА БУДУЩЕЕ (квесты): условие показа; сейчас NULL/игнор
			FOREIGN KEY(node_id) REFERENCES dialogue_nodes(id)
		);`,
		// ===== ТРЕНЕРСКИЙ БОЙ (поток B) =====
		// Фиксированный ростер команды NPC (team_mode='fixed'). Для 'random'
		// команда генерируется на лету и тут НЕ хранится.
		`CREATE TABLE IF NOT EXISTS npc_pokemons (
			id INTEGER PRIMARY KEY,
			npc_id INTEGER NOT NULL,
			pokemon_id INTEGER NOT NULL,
			level INTEGER NOT NULL DEFAULT 5,
			position INTEGER DEFAULT 0,        -- порядок вывода в бою
			FOREIGN KEY(npc_id) REFERENCES npc(id),
			FOREIGN KEY(pokemon_id) REFERENCES pokemons(id)
		);`,
		// AI-команды тренера (ai_level >= 1): пул сохранённых составов.
		// team_json — снимок полного покемона (вид, ходы, способность, характер,
		// IV/EV, предмет), чтобы бой воспроизводился без пересоздания.
		`CREATE TABLE IF NOT EXISTS npc_ai_teams (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			npc_id INTEGER NOT NULL,
			ai_level INTEGER NOT NULL DEFAULT 1,
			level INTEGER NOT NULL,
			team_json TEXT NOT NULL,
			score REAL DEFAULT 0,
			source TEXT DEFAULT 'draft',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(npc_id) REFERENCES npc(id)
		);`,
		// ===== РАЗВЕДЕНИЕ (поток [13]) =====
		// Яйцо, полученное сведе́нием пары в питомнике. Вид (species_id) —
		// БАЗОВАЯ форма линии; IV уже посчитаны при создании (макс родителей +30%).
		// created_at/hatch_at хранятся как UTC-строки 'YYYY-MM-DD HH:MM:SS'
		// (тот же формат, что user_pokemons.created_at); готовность = now>=hatch_at.
		`CREATE TABLE IF NOT EXISTS eggs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			species_id INTEGER NOT NULL,        -- базовая форма, которая вылупится
			iv_hp INTEGER, iv_attack INTEGER, iv_defense INTEGER,
			iv_speed INTEGER, iv_special_attack INTEGER, iv_special_defense INTEGER,
			parent1_id INTEGER, parent2_id INTEGER,
			created_at TEXT NOT NULL,
			hatch_at   TEXT NOT NULL,
			FOREIGN KEY(user_id) REFERENCES users(id),
			FOREIGN KEY(species_id) REFERENCES pokemons(id)
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
	DB.Exec("ALTER TABLE pokemon_locations ADD COLUMN min_level INTEGER DEFAULT 1")
	DB.Exec("ALTER TABLE pokemon_locations ADD COLUMN max_level INTEGER DEFAULT 20")
	DB.Exec("ALTER TABLE user_pokemon_learnable_attacks ADD COLUMN current_pp INTEGER DEFAULT 0")
	DB.Exec("ALTER TABLE user_pokemon_learnable_attacks ADD COLUMN source TEXT DEFAULT 'level'")
	DB.Exec("ALTER TABLE user_pokemon_learnable_attacks ADD COLUMN source_item_id INTEGER")
	DB.Exec("ALTER TABLE items ADD COLUMN attack_id INTEGER")
	DB.Exec("ALTER TABLE user_pokemons ADD COLUMN happiness INTEGER DEFAULT 0")
	DB.Exec("ALTER TABLE user_pokemons ADD COLUMN gender TEXT")                  // пол экземпляра 'M'/'F'/'N'
	DB.Exec("ALTER TABLE user_pokemons ADD COLUMN attractiveness TEXT")          // буква привлекательности 'A'/'B'/'C'
	DB.Exec("ALTER TABLE user_pokemons ADD COLUMN is_starter INTEGER DEFAULT 0") // 1 = стартовый (первым в бой)

	// Карточка тренера (поток [7]): поля профиля на users + флаг награды на items.
	DB.Exec("ALTER TABLE users ADD COLUMN status_text TEXT")           // статус игрока (сам пишет)
	DB.Exec("ALTER TABLE users ADD COLUMN role_id INTEGER")            // должность (FK player_roles), nullable
	DB.Exec("ALTER TABLE users ADD COLUMN last_seen TIMESTAMP")        // для онлайн/оффлайн
	DB.Exec("ALTER TABLE users ADD COLUMN avatar_data TEXT")           // задел под аватар (пока не используем)
	DB.Exec("ALTER TABLE items ADD COLUMN is_award INTEGER DEFAULT 0") // 1 = награда (показ в карточке, не в сумке)

	// Диалоги NPC (поток A): поля команды NPC для боя (используются потоком B).
	// В потоке A только редактируются в админке; команда НЕ генерируется.
	DB.Exec("ALTER TABLE npc ADD COLUMN team_mode TEXT DEFAULT 'random'") // 'random'|'fixed'
	DB.Exec("ALTER TABLE npc ADD COLUMN team_size INTEGER DEFAULT 6")     // 1..6
	DB.Exec("ALTER TABLE npc ADD COLUMN min_level INTEGER DEFAULT 5")
	DB.Exec("ALTER TABLE npc ADD COLUMN max_level INTEGER DEFAULT 10")
	// AI-уровень тренера: 0 = старый smart-AI, 1/2 = новый бот, 3/4 = задел.
	DB.Exec("ALTER TABLE npc ADD COLUMN ai_level INTEGER DEFAULT 0")
	// Признак легендарки/мифика для ограничений драфта (бэкфилл — legendaries.go).
	DB.Exec("ALTER TABLE pokemons ADD COLUMN is_legendary INTEGER DEFAULT 0")
	DB.Exec("ALTER TABLE attacks ADD COLUMN never_misses INTEGER DEFAULT 0")
	DB.Exec("ALTER TABLE attacks ADD COLUMN recoil INTEGER DEFAULT 0")
	DB.Exec("ALTER TABLE attacks ADD COLUMN drain INTEGER DEFAULT 0")
	DB.Exec("ALTER TABLE attacks ADD COLUMN crit_rate INTEGER DEFAULT 0")
	DB.Exec("ALTER TABLE attacks ADD COLUMN hits_min INTEGER DEFAULT 1")
	DB.Exec("ALTER TABLE attacks ADD COLUMN hits_max INTEGER DEFAULT 1")
	DB.Exec("ALTER TABLE attacks ADD COLUMN heal_percent INTEGER DEFAULT 0")

	// Статус "Проклят" (volatile, id 12) — накладывается атакой "Проклятье".
	DB.Exec("INSERT OR IGNORE INTO statuses (id, name, description) VALUES (12, 'Проклят', 'Теряет 1/4 макс. HP каждый раунд')")
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
	DB.Exec("DELETE FROM statuses WHERE id = 9")  // Путы
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

	// Общая категория TM/HM (тип предмета различается effect_type: tm/hm)
	DB.Exec("INSERT OR IGNORE INTO item_categories (id, name) VALUES (10, 'TM/HM')")

	// Категория "candy" и конфеты. effect_value = сколько EV даётся за уровень.
	DB.Exec("INSERT OR IGNORE INTO item_categories (name) VALUES ('candy')")
	DB.Exec(`
		INSERT OR IGNORE INTO items (id, name, description, category_id, price, effect_type, effect_value, image_url)
		VALUES (?, ?, ?, (SELECT id FROM item_categories WHERE name = 'candy'), ?, ?, ?, ?)
	`, 100, "Леденец", "Повышает уровень покемона на 1 и даёт +2 EV", 0, "candy", 2, "/static/images/items/41.png")
	DB.Exec(`
		INSERT OR IGNORE INTO items (id, name, description, category_id, price, effect_type, effect_value, image_url)
		VALUES (?, ?, ?, (SELECT id FROM item_categories WHERE name = 'candy'), ?, ?, ?, ?)
	`, 167, "Жёлтая конфета", "Повышает уровень покемона на 1 и даёт +3 EV", 0, "candy", 3, "/static/images/items/yellow_candy.png")
	// Идемпотентно приводим effect_value Леденца к смыслу «EV за уровень»
	// (исторически там стояла 1, а +2 было захардкожено в useCandy).
	DB.Exec("UPDATE items SET effect_value = 2 WHERE id = 100 AND effect_type = 'candy' AND effect_value = 1")

	// Способности (поток [9] — КАРКАС). Схема + миграция слотов + бэкфилл экземпляров.
	// ALTER fire-and-ignore (конвенция проекта: дубль колонки игнорируется).
	DB.Exec("ALTER TABLE abilities ADD COLUMN effect_key TEXT")          // машиночит. ключ эффекта (заполняется позже)
	DB.Exec("ALTER TABLE pokemon_abilities ADD COLUMN slot INTEGER")     // 1=базовая X, 2=базовая Y, 3=скрытая
	DB.Exec("ALTER TABLE user_pokemons ADD COLUMN ability_id INTEGER")   // закреплённая способность экземпляра
	DB.Exec("ALTER TABLE user_pokemons ADD COLUMN ability_slot INTEGER") // её слот (для ремапа при эволюции)
	// [forms] Расширение pokemon_forms: переключение по ходу + подписи/порядок.
	DB.Exec("ALTER TABLE pokemon_forms ADD COLUMN form_label TEXT")
	DB.Exec("ALTER TABLE pokemon_forms ADD COLUMN trigger_mode TEXT DEFAULT 'hp'")
	DB.Exec("ALTER TABLE pokemon_forms ADD COLUMN trigger_move_id INTEGER DEFAULT 0")
	DB.Exec("ALTER TABLE pokemon_forms ADD COLUMN trigger_move_kind TEXT DEFAULT ''")
	DB.Exec("ALTER TABLE pokemon_forms ADD COLUMN is_base_form INTEGER DEFAULT 0")
	DB.Exec("ALTER TABLE pokemon_forms ADD COLUMN form_order INTEGER DEFAULT 0")
	DB.Exec("ALTER TABLE pokemon_forms ADD COLUMN form_description TEXT")
	migrateAbilitySlots()       // проставить slot существующим pokemon_abilities (идемпотентно)
	backfillInstanceAbilities() // назначить способность экземплярам без неё (идемпотентно)
	migrateAbilityBatch5Keys()  // [batch5] effect_key сложных способностей (идемпотентно)
	migrateAbilityBatch6Keys()  // [batch6] effect_key топ-20 по носителям (идемпотентно)
	migrateAbilityBatch7Keys()  // [batch7] effect_key расширенного набора (идемпотентно)

	// Условия эволюции (день/ночь, сравнение стат и т.п.). Пустое значение —
	// условие не задано; для level-эволюций это норма (только уровень).
	DB.Exec("ALTER TABLE pokemons ADD COLUMN evolution_condition TEXT")
	DB.Exec("ALTER TABLE pokemons ADD COLUMN evolution_priority INTEGER DEFAULT 0")
	migrateEvolutionConditions()
	migrateEvolutionParents()
}

// migrateEvolutionConditions заполняет условия для развилок, которые раньше
// были неразличимы (Иви→Эспеон/Умбреон, Тайрог→Хитмон*). Идемпотентно: трогает
// строку только пока condition пустой, чтобы не затирать правки из админки.
func migrateEvolutionConditions() {
	queries := []string{
		// Иви: friendship-ветки различаются временем суток.
		`UPDATE pokemons
		 SET evolves_from_id = 133, evolution_method_id = 4, evolution_level = NULL,
		     evolution_condition = 'time_day', evolution_priority = 0
		 WHERE id = 196 AND (evolution_condition IS NULL OR evolution_condition = '')`,
		`UPDATE pokemons
		 SET evolves_from_id = 133, evolution_method_id = 4, evolution_level = NULL,
		     evolution_condition = 'time_night', evolution_priority = 0
		 WHERE id = 197 AND (evolution_condition IS NULL OR evolution_condition = '')`,
		// Сильвеон — приоритетнее день/ночь, если Иви знает Fairy-атаку.
		`UPDATE pokemons
		 SET evolves_from_id = 133, evolution_method_id = 4, evolution_level = NULL,
		     evolution_condition = 'fairy_move', evolution_priority = 10
		 WHERE id = 700 AND (evolution_condition IS NULL OR evolution_condition = '')`,

		// Тайрог: три ветки по сравнению Atk/Def на момент эволюции.
		`UPDATE pokemons
		 SET evolves_from_id = 236, evolution_method_id = 1, evolution_level = 20,
		     evolution_condition = 'atk_gt_def', evolution_priority = 0
		 WHERE id = 106 AND (evolution_condition IS NULL OR evolution_condition = '')`,
		`UPDATE pokemons
		 SET evolves_from_id = 236, evolution_method_id = 1, evolution_level = 20,
		     evolution_condition = 'atk_lt_def', evolution_priority = 0
		 WHERE id = 107 AND (evolution_condition IS NULL OR evolution_condition = '')`,
		`UPDATE pokemons
		 SET evolves_from_id = 236, evolution_method_id = 1, evolution_level = 20,
		     evolution_condition = 'atk_eq_def', evolution_priority = 0
		 WHERE id = 237 AND (evolution_condition IS NULL OR evolution_condition = '')`,
	}
	for _, q := range queries {
		DB.Exec(q)
	}
}

// migrateEvolutionParents чинит ошибочные родительские связи, оставшиеся от
// линейного импорта: все формы Иви должны эволюционировать напрямую из Иви (133),
// а не Вапореон → Джолтеон → Флареон. Идемпотентно.
func migrateEvolutionParents() {
	DB.Exec(`
		UPDATE pokemons
		SET evolves_from_id = 133
		WHERE id IN (135, 136) AND (evolves_from_id IS NULL OR evolves_from_id <> 133)
	`)
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

	// Методы эволюции. «Обмен» заменён на «Разведение»: торговли в игре нет,
	// покемон эволюционирует при спаривании (BreedHandler). Переименовываем
	// существующую строку id=2 ДО вставки, чтобы UNIQUE(name) не создал дубль.
	DB.Exec("UPDATE evolution_methods SET name = 'breeding' WHERE id = 2 AND name = 'trade'")
	evoMethods := []string{"level", "breeding", "stone", "friendship", "item"}
	for _, m := range evoMethods {
		DB.Exec("INSERT OR IGNORE INTO evolution_methods (name) VALUES (?)", m)
	}
	// gen1-«обменные» (метод был NULL): Алаказам/Мачамп/Голем/Генгар.
	DB.Exec("UPDATE pokemons SET evolution_method_id = 2 WHERE id IN (65, 68, 76, 94) AND evolution_method_id IS NULL")

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

	// Должности игрока (карточка тренера). is_admin остаётся флагом ПРАВ отдельно.
	playerRoles := []struct {
		id   int
		name string
	}{
		{1, "Тренер"}, // дефолт (можно NULL role_id)
		{2, "Админ"},
		{3, "Гим лидер"},
		{4, "Полицейский"},
		{5, "Наставник"},
	}
	for _, pr := range playerRoles {
		DB.Exec("INSERT OR IGNORE INTO player_roles (id, name) VALUES (?, ?)", pr.id, pr.name)
	}

	// Регион создаётся в SeedEastJohtoFiveRoads (id=3).

	// Погода
	weatherTypes := []string{"Обычная", "Солнечно", "Дождь", "Град", "Песчаная буря"}
	for _, w := range weatherTypes {
		DB.Exec("INSERT OR IGNORE INTO weather (name) VALUES (?)", w)
	}

	// Установить случайную погоду для каждого региона при первом запуске
	DB.Exec("UPDATE regions SET current_weather_id = (ABS(RANDOM()) % 5) + 1")
}

// seedEvolutionItems идемпотентно добавляет предметы эволюции и привязывает
// их к видам через pokemons.evolution_item_id. Привязка ставится только если поле
// ещё пустое — ручные правки админа не затираются повторным запуском.
//
// Каноничные камни (10): Fire, Water, Thunder, Leaf, Moon, Sun, Shiny, Dusk,
// Dawn, Ice. King's Rock / Metal Coat / Dragon Scale / Up-Grade оставлены как
// trade-предметы (method=2). Oval Stone (Хаппини → Ченси) — предмет с той же
// механикой «применить из инвентаря». Региональные формы (Alolan Sandslash/
// Ninetales, Galarian Darmanitan, Hisuian Electrode) в БД не заведены
// отдельными видами, поэтому их каменные ветки сюда не включены.
// seedHappinessBerry идемпотентно добавляет «Сладкую ягоду» — расходуемый
// предмет, поднимающий счастье покемона на 5 (максимум 255). При достижении
// 255 friendship-эволюция срабатывает сразу (см. api.useHappiness).
func seedHappinessBerry() {
	DB.Exec(`
		INSERT OR IGNORE INTO items
			(id, name, description, category_id, price, effect_type, effect_value, image_url)
		VALUES (166, 'Сладкая ягода',
			'Увеличивает счастье покемона на 5. Максимум — 255.',
			6, 0, 'happiness', 5, '/static/images/items/sweet_berry.png')
	`)
}

// seedHeldItems идемпотентно добавляет надеваемые предметы с боевыми эффектами.
// Зажигалка (burn_holder): в конце хода поджигает покемона, на котором надета.
func seedHeldItems() {
	DB.Exec("INSERT OR IGNORE INTO item_categories (id, name) VALUES (9, 'held')")
	DB.Exec(`
		INSERT OR IGNORE INTO items
			(id, name, description, category_id, price, effect_type, effect_value, image_url)
		VALUES (168, 'Зажигалка',
			'Надеваемый предмет. В конце хода поджигает покемона, на котором надет, вызывая ожог.',
			9, 0, 'burn_holder', 0, '/static/images/items/lighter.svg')
	`)
}

// seedBattleItems идемпотентно добавляет расходуемые предметы, которые можно
// применять в бою (лечение HP и снятие статусов). effect_type:
//
//	battle_heal_hp      — вылечить effect_value HP;
//	battle_full_restore — полностью вылечить и срезать бусты статов выше +2;
//	battle_cure_status  — снять статус effect_value (13 = замешательство,
//	                      6 = любое отравление: 6 и 7).
func seedBattleItems() {
	DB.Exec("INSERT OR IGNORE INTO item_categories (id, name) VALUES (2, 'potion')")
	DB.Exec("INSERT OR IGNORE INTO item_categories (id, name) VALUES (3, 'status_healer')")
	items := []struct {
		id          int
		name        string
		description string
		categoryID  int
		effectType  string
		effectValue int
		imageURL    string
	}{
		{169, "Стимпак", "Боевой предмет. Полностью восстанавливает HP активного покемона, но все его характеристики выше +2 сбрасываются до +2.", 2, "battle_full_restore", 0, "/static/images/items/stimpak.svg"},
		{170, "Стимулятор", "Боевой предмет. Восстанавливает 10 HP активному покемону.", 2, "battle_heal_hp", 10, "/static/images/items/stimulator.svg"},
		{171, "Лечебная трава", "Боевой предмет. Восстанавливает 5 HP активному покемону.", 2, "battle_heal_hp", 5, "/static/images/items/healing_herb.svg"},
		{172, "Суперстимулятор", "Боевой предмет. Восстанавливает 100 HP активному покемону.", 2, "battle_heal_hp", 100, "/static/images/items/super_stimulator.svg"},
		{173, "Антидот", "Боевой предмет. Снимает отравление (обычное и сильное) с активного покемона.", 3, "battle_cure_status", 6, "/static/images/items/antidote.svg"},
		{174, "Антипаралитик", "Боевой предмет. Снимает паралич с активного покемона.", 3, "battle_cure_status", 2, "/static/images/items/antiparalytic.svg"},
		{175, "Антизаморозка", "Боевой предмет. Снимает заморозку с активного покемона.", 3, "battle_cure_status", 4, "/static/images/items/antifreeze.svg"},
		{176, "Пробуждение", "Боевой предмет. Снимает сон с активного покемона.", 3, "battle_cure_status", 5, "/static/images/items/awakening.svg"},
		{177, "Ожоговый бальзам", "Боевой предмет. Снимает ожог с активного покемона.", 3, "battle_cure_status", 3, "/static/images/items/burn_balm.svg"},
		{178, "Успокоительное", "Боевой предмет. Снимает замешательство с активного покемона.", 3, "battle_cure_status", 13, "/static/images/items/calm_balm.svg"},
	}
	for _, it := range items {
		DB.Exec(`
			INSERT OR IGNORE INTO items
				(id, name, description, category_id, price, effect_type, effect_value, image_url)
			VALUES (?, ?, ?, ?, 0, ?, ?, ?)
		`, it.id, it.name, it.description, it.categoryID, it.effectType, it.effectValue, it.imageURL)
	}
}

func seedEvolutionItems() {
	DB.Exec("INSERT OR IGNORE INTO item_categories (id, name) VALUES (5, 'evolution_stone')")

	items := []struct {
		id          int
		name        string
		description string
		imageURL    string
	}{
		{151, "Солнечный камень", "Эволюционный предмет: Глум → Беллоссом, Санкерн → Санфлора, Коттони → Вимсикотт, Петилил → Лиллигант, Хелиоптайл → Хелиолиск.", "/static/images/items/sun_stone.png"},
		{152, "Королевский камень", "Эволюционный предмет: Поливирл → Политоуд, Слоупок → Слоукинг.", "/static/images/items/kings_rock.png"},
		{153, "Металлическая оболочка", "Эволюционный предмет: Оникс → Стиликс, Сайзер → Сайзор.", "/static/images/items/metal_coat.png"},
		{154, "Драконья чешуя", "Эволюционный предмет: Сидра → Кингдра.", "/static/images/items/dragon_scale.png"},
		{155, "Обновление уровень 1", "Эволюционный предмет: Поригон → Поригон2.", "/static/images/items/up_grade.png"},
		{156, "Огненный камень", "Эволюционный предмет: Вульпикс → Нинетейлс, Гроулит → Арканин, Иви → Флареон, Пансир → Симисир, Капсакид → Сковиллен.", "/static/images/items/fire_stone.png"},
		{157, "Водный камень", "Эволюционный предмет: Поливирл → Поливрат, Шеллдер → Клойстер, Старью → Старми, Иви → Вапореон, Ломбре → Лудиколо, Панпур → Симипур.", "/static/images/items/water_stone.png"},
		{158, "Громовой камень", "Эволюционный предмет: Пикачу → Райчу, Иви → Джолтеон, Магнетон → Магнезон, Ноузпасс → Пробопасс, Илектрайк → Электросс, Чарджабаг → Викаволт, Тадбулб → Беллиболт.", "/static/images/items/thunder_stone.png"},
		{159, "Листовой камень", "Эволюционный предмет: Глум → Вилеплум, Випинбелл → Виктрибел, Эгзекьют → Экзеггутор, Назлиф → Шифтри, Иви → Лифеон, Пансейдж → Симисейдж.", "/static/images/items/leaf_stone.png"},
		{160, "Лунный камень", "Эволюционный предмет: Нидорина → Нидоуквин, Нидорино → Нидокинг, Клефейри → Клефабл, Джигглипафф → Вигглитафф, Скитти → Делкатти, Мунна → Мушарна.", "/static/images/items/moon_stone.png"},
		{161, "Блестящий камень", "Эволюционный предмет: Розелия → Розрейд, Тогетик → Тогекисс, Минччино → Чинччино, Флоэтт → Флоржес.", "/static/images/items/shiny_stone.png"},
		{162, "Тёмный камень", "Эволюционный предмет: Мисдривус → Мисмагиус, Муркроу → Хончкроу, Лампент → Шанделюр, Дублейд → Иджислаш.", "/static/images/items/dusk_stone.png"},
		{163, "Рассветный камень", "Эволюционный предмет: Кирлия → Галлейд, Снорант → Фросласс.", "/static/images/items/dawn_stone.png"},
		{164, "Ледяной камень", "Эволюционный предмет: Иви → Глейсеон, Крабролер → Крабоминабл, Сетоддл → Сетайтан.", "/static/images/items/ice_stone.png"},
		{165, "Овальный камень", "Эволюционный предмет: Хаппини → Ченси.", "/static/images/items/oval_stone.png"},
	}
	for _, it := range items {
		DB.Exec(`
			INSERT OR IGNORE INTO items
				(id, name, description, category_id, price, effect_type, effect_value, image_url)
			VALUES (?, ?, ?, 5, 0, 'evolve', 0, ?)
		`, it.id, it.name, it.description, it.imageURL)
	}

	// method: 3=stone, 2=trade. Для новых связей метод ставится явно (3): часть
	// строк в БД была method=1/null, и старый COALESCE оставлял их без item-эволюции.
	// Привязка по-прежнему только для пустого evolution_item_id.
	links := []struct {
		pokemonID, itemID, methodID int
	}{
		{182, 151, 3}, {192, 151, 3}, {547, 151, 3}, {549, 151, 3}, {695, 151, 3},
		{186, 152, 2}, {199, 152, 2},
		{208, 153, 2}, {212, 153, 2},
		{230, 154, 2},
		{233, 155, 2},
		{38, 156, 3}, {59, 156, 3}, {136, 156, 3}, {514, 156, 3}, {952, 156, 3},
		{62, 157, 3}, {91, 157, 3}, {121, 157, 3}, {134, 157, 3}, {272, 157, 3}, {516, 157, 3},
		{26, 158, 3}, {135, 158, 3}, {462, 158, 3}, {476, 158, 3}, {604, 158, 3}, {738, 158, 3}, {939, 158, 3},
		{45, 159, 3}, {71, 159, 3}, {103, 159, 3}, {275, 159, 3}, {470, 159, 3}, {512, 159, 3},
		{31, 160, 3}, {34, 160, 3}, {36, 160, 3}, {40, 160, 3}, {301, 160, 3}, {518, 160, 3},
		{407, 161, 3}, {468, 161, 3}, {573, 161, 3}, {671, 161, 3},
		{429, 162, 3}, {430, 162, 3}, {609, 162, 3}, {681, 162, 3},
		{475, 163, 3}, {478, 163, 3},
		{471, 164, 3}, {740, 164, 3}, {975, 164, 3},
		{113, 165, 3},
	}
	for _, l := range links {
		DB.Exec(`
			UPDATE pokemons
			SET evolution_item_id = ?,
			    evolution_method_id = ?
			WHERE id = ? AND evolution_item_id IS NULL
		`, l.itemID, l.methodID, l.pokemonID)
	}
}
