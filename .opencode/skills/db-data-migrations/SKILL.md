---
name: db-data-migrations
description: Use when editing pokemon-game data or schema - SQLite, database/pokemon.db, Python sqlite3 scripts, migrations in database.go, imports of Pokemon/moves/abilities/items, backups, integrity checks, orphan/duplicate checks.
---

# БД и миграции pokemon-game

Данные — главный источник правды: виды, атаки, способности, лёрнсеты, локации, NPC и предметы лежат в SQLite, а не в Go-коде. Движок должен работать с любым видом без правок.

## Расположение и безопасность

- Активная БД: `database/pokemon.db` (в git **не** хранится).
- Локальные бэкапы: `database/backups/` (ignored). Не удалять их через git-команды.
- Перед любой миграцией/массовой правкой:

```powershell
Copy-Item database/pokemon.db "database/backups/pokemon.db.bak-before-<тема>" -Force
python -c "import sqlite3; c=sqlite3.connect(r'database/pokemon.db'); print(c.execute('PRAGMA integrity_check').fetchone()); c.close()"
```

- Массовые записи делать при остановленном сервере (избежать `database is locked`).
- Все изменения — в транзакции: `BEGIN` … `COMMIT`, при ошибке `ROLLBACK`.
- Python на машине есть; `sqlite3` CLI может отсутствовать. PowerShell 5.1: не забывать `-Encoding UTF8` при чтении файлов.

## Схема (срез 2026-10-23, после gen9)

Ключевые таблицы:

| Таблица | Назначение |
|---|---|
| `pokemons` | виды #1–1025, статы, типы, эволюции, `category_power` (=base_exp), `catch_rate`, image_url |
| `attacks` | 791 атака (max id 793): `name`, `eng_name`, `type_id`, `category`, `power`, `accuracy`, эффекты |
| `pokemon_attacks` | лёрнсеты (54k+ строк): вид ↔ атака, level/tm/egg/tutor |
| `abilities` | 303 способности, `effect_key`, `description` |
| `pokemon_abilities` | способности видов: `slot 1/2/3` (3 ⇔ hidden) |
| `user_pokemons` | экземпляры: level, exp, IV/EV, `gender`, `attractiveness`, `ability_id/ability_slot`, `held_item_id`, позиция |
| `user_pokemon_learnable_attacks` | выученные/доступные атаки экземпляра |
| `user_items` / `items` | инвентарь и каталог предметов; `items.effect_type`, `effect_value`, `price`, `image_url` |
| `users` | аккаунты, `location_id`, `last_seen`, профиль, роль |
| `locations` / `location_exits` / `pokemon_locations` | карта и зоны ловли |
| `npc` / `npc_pokemons` | NPC, режим команды, фикс-ростер |
| `dialogue_nodes` / `dialogue_options` | граф диалогов, ровно один `is_start` на NPC |
| `eggs` | яйца разведения |
| `player_roles`, `user_friends`, `user_wishlist` | карточка тренера |
| `statuses`, `weather`, `natures`, `types`, `type_effectiveness` | справочники боя |

## Как делать миграции в коде

`server/db/database.go` → `InitDB` вызывает `createTables()` и `insertInitialData()` при каждом старте. Миграции должны быть **идемпотентными**:

```go
// создание
`CREATE TABLE IF NOT EXISTS foo (...);`

// добавление колонки: ошибку "duplicate column" игнорировать
DB.Exec(`ALTER TABLE foo ADD COLUMN bar INTEGER DEFAULT 0`)

// бэкфилл — можно запускать повторно
DB.Exec(`UPDATE foo SET bar = ... WHERE bar IS NULL`)
```

Сложные бэкфиллы (например слоты способностей) — в `server/db/abilities.go` (`migrateAbilitySlots`, `backfillInstanceAbilities`), вызываются из `InitDB`. После миграции проверить `PRAGMA integrity_check` и ключевые `SELECT COUNT(*)`.

## Рецепты проверки данных

```python
import sqlite3
c = sqlite3.connect(r'database/pokemon.db')

# сироты лёрнсетов
print(c.execute("""
  select count(*) from pokemon_attacks pa
  left join pokemons p on p.id = pa.pokemon_id
  left join attacks a on a.id = pa.attack_id
  where p.id is null or a.id is null
""").fetchone())

# дубли слотов способностей
print(c.execute("""
  select pokemon_id, slot, count(*) from pokemon_abilities
  group by pokemon_id, slot having count(*) > 1
""").fetchall())

# покрытие effect_key
print(c.execute("select count(distinct effect_key) from abilities where effect_key <> ''").fetchone())

# экземпляры без способности
print(c.execute("select count(*) from user_pokemons where ability_id is null").fetchone())
```

Проверять поколения: `select min(id), max(id), count(*) from pokemons` (срез 2026-10-23: 1–1025 / 1025).

## Грабли (уже случались)

- **NULL `description` у вида ломает покедекс**: `pokedex.go` сканирует в `string`; при вставке gen-N всегда заполнять непустой `description`, иначе `/full` → 404, `/search` молча пропускает.
- **`category_power` = base_experience**: иначе `awardExp` даёт 0 за gen-N.
- **`evolves_from_id`**: 0 у базовых форм; спуск до базовой формы используется в разведении (`baseFormOf`). Не плодить циклы.
- **Способности**: `slot=3` обязан совпадать с `is_hidden=1`; PK `(pokemon_id, ability_id)` схлопывает дубли (Metapod/Kakuna уже ловили).
- **Покемоны gen2–gen6**: `image_url` ведёт на `/static/images/pokemons/normal/NNN.gif`, клиент заменяет `/normal/` → `/mini/`. Обе папки заполнены (фактически одинаковыми файлами — известное дублирование). Для нового поколения см. skill `adding-pokemon-generations`.
- **Русские имена видов gen4** берутся с pokemon.fandom.com/ru (список «Поколение IV»); имена совпадают с конвенцией gen1–3 (Тортвиг, Гарчомп и т.п.).
- **Половые доли**: `male_percentage` + `female_percentage`; обе 0 → пол `N`.
- **Разведение**: `BreedCompat` = один вид + одна буква + (M/F или N/N с Амуритом id=150 на обоих); Амурит не расходуется.
- **Предметы эволюции**: у части gen2-эволюций нет `item_id` в `items` (Sun Stone, King's Rock, Metal Coat, Dragon Scale, Up-Grade) — условие задано методом без предмета.
- **Спрайты**: не переименовывать формат `NNN.gif` и пути `normal/` / `mini/` — клиент строит путь заменой строки.

## Источники данных

- gen1/gen2 статы, типы, эволюции, лёрнсеты — PokeAPI (канон).
- Способности, русские описания, часть предметов — wiki.league17.ru (MediaWiki API, сырой wikitext). Русские имена league17 не совпадают с каноном: `effect_key` маппить по описанию, не по имени.
- Перед импортом сохранять скрипт/дампы вне репозитория (scratchpad), в репозиторий — только итоговые данные.
