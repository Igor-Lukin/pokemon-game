---
name: pokemon-game-core
description: Use ONLY inside the pokemon-game repository. Project map, build/run commands, HANDOFF/TEST workflow, data-safety rules, single-source-of-truth conventions and definition of done. Trigger on pokemon-game, server/main.go, database/pokemon.db, HANDOFF_*.txt, battle.go, game.js, CreateUserPokemon.
---

# pokemon-game: ядро проекта

Веб-игра по покемонам: Go + SQLite + серверный HTML + админка. Бой пошаговый, серверный и stateful. Клиент — `server/templates/game.html` + `client/static/js/game.js`.

## Быстрый старт

Работать и запускать **только из корня репозитория** (`D:\pokemon-game`): БД открывается как `./database/pokemon.db`.

```powershell
# проверить компиляцию и статический анализ
go build ./...
go vet ./...

# проверка JS (node установлен)
node --check client/static/js/game.js
node --check client/static/js/pokedex.js

# запуск сервера — только через detached-скрипт (skill `start-pokemon-server`):
# go run ./server из Bash-вызова держит сессию и выглядит как зависание.
& .\.opencode\skills\start-pokemon-server\start-server.ps1
# остановка:
& .\.opencode\skills\start-pokemon-server\start-server.ps1 -Stop
# http://localhost:8080
```

Служебные команды/утилиты: `cmd/give_all` (массовая выдача), `database/backups/` (локальные бэкапы БД, в git не попадают).

## Карта файлов

| Что | Где |
|---|---|
| Точка входа | `server/main.go` |
| Инициализация БД, миграции `CREATE/ALTER` | `server/db/database.go` (`InitDB`, `createTables`, `insertInitialData`) |
| Роуты | `server/routes/routes.go` |
| Сессии (cookie, `SESSION_KEY`) | `server/session/store.go` |
| Бой (дикий + тренер) | `server/handlers/api/battle.go`, `wild.go`, `catch.go`, `npc_battle.go`, `npc_smart_team.go` |
| PvP | `server/handlers/api/pvp.go` (+ `online.go` для вызовов) |
| Способности | `server/db/abilities.go`, `server/handlers/api/abilities_battle.go` |
| Создание экземпляра покемона | `server/utils/create_pokemon.go` → `CreateUserPokemon` |
| Пол/привлекательность | `server/db/gender.go` (`RollGender`, `RollAttractiveness`, `BreedCompat`) |
| Разведение | `server/handlers/api/breeding.go` |
| Предметы / held / болы | `server/db/items.go`, `server/handlers/api/held_items.go`, `inventory.go`, `catch.go` |
| Покедекс | `server/handlers/api/pokedex.go`, `client/static/js/pokedex.js` |
| Профиль/карточка тренера | `server/db/profile.go`, `server/handlers/api/profile.go` |
| NPC + диалоги | `server/handlers/admin/npc*.go`, `dialogue.go`, `server/handlers/api/dialogue.go` |
| Фронт игры | `client/static/js/game.js`, `client/static/css/game.css`, `server/templates/game.html`, `client/static/templates/battle_card.html` |
| Админка | `server/handlers/admin/*`, `server/templates/admin/*` |

## Документация и рабочий процесс

Перед крупной задачей читать в таком порядке:

1. `AI_MEMORY.md` — институциональная память, архитектурные грабли.
2. `HANDOFF_STATUS.txt` — консолидированный статус: готово / написано но не проверено / очередь.
3. `HANDOFF_main.txt` — журнал решений владельца.
4. `HANDOFF_<тема>.txt` — ТЗ на фичу, `TEST_<тема>.txt` — отчёт о реализации.

Формат работы проекта: владелец пишет ТЗ (`HANDOFF_<тема>.txt`) → агент реализует → отчёт `TEST_<тема>.txt` → владелец делает живой прогон. «Готово» = живой прогон, а не только зелёный build/vet. Git-прод-пайплайна нет, но текущее состояние всё равно надо сохранять коммитами.

Добавление нового поколения покемонов — skill `adding-pokemon-generations`; задачи по поколениям — `HANDOFF_gen4.txt`…`HANDOFF_gen9.txt`.

## Железные конвенции

- **Единственная точка создания экземпляра** — `utils.CreateUserPokemon` (ловля, выдача, вылупление, админка). Она роллит пол, привлекательность, способность. Не дублировать эту логику.
- **Пол/буква** — только через `db.RollGender` / `db.RollAttractiveness`; совместимость разведения — `db.BreedCompat`.
- **Данные — в БД, не в Go.** Виды, атаки, способности, лёрнсеты правятся SQL/python. Движок не должен знать конкретные виды.
- **Не плодить второй боевой движок.** Новые эффекты встраивать в существующие примитивы (`executeDamagingMove`, `applyEffectByID`, `applyStatChanges`, `sideFor` и т.д.). Подробности — skill `battle-engine`.
- **Опыт**: `category_power` у вида = `base_experience`; `awardExp` работает только для `BattleType == "wild"`.
- **IV** — шкала `0..31` (дикие, NPC, разведение).
- **Способность экземпляра** хранится в `user_pokemons.ability_id/ability_slot`; при эволюции ремап — `db.RemapAbilityOnEvolve` (вызывается из `evolution.go` и `levelup.go`).
- **Слоты команды**: `position 1..6`, `0/NULL` — питомник.
- **Не хардкодить 151**: лимит поколения берётся из БД/`/api/pokedex/list` (`maxPokemonId`). Сервер по номеру вида не ограничен.
- **Клиент без сборщика**: ванильный JS, Ctrl+F5 после правок.
- **Кодировка**: все `*.txt/*.md` — UTF-8 без BOM. В PowerShell 5.1 `Get-Content` по умолчанию читает ANSI и показывает кракозябры — читать через инструмент Read или `Get-Content -Encoding UTF8`. Не пересохранять документы в другой кодировке.

## Безопасность данных

- Активная БД: `database/pokemon.db` (в git игнорируется).
- Перед миграцией/массовой правкой: копия в `database/backups/` (тоже ignored).
- После правок: `python -c "import sqlite3; ... PRAGMA integrity_check"`.
- Миграции в `InitDB` должны быть идемпотентными: `CREATE TABLE IF NOT EXISTS` + fire-and-forget `ALTER` + бэкфилл, который можно запускать повторно.
- Сырые SQL-скрипты и импорт данных — skill `db-data-migrations`.

## Текущий статус (срез 2026-10-23)

Последняя подтверждённая работа: gen7 (#722–809) — данные, лёрнсеты, способности (base+hidden), эволюции, 25 новых атак данными, спрайты (`TEST_gen7.txt`); ранее gen6 (`TEST_gen6.txt`), gen5 (`TEST_gen5.txt`), gen4 (`TEST_gen4.txt`), gen3 (`TEST_gen3.txt`), gen2-атаки (`TEST_gen2_moves.txt`). Последний коммит-чекпоинт: `01e27f4`.

Не проверено живьём (хвосты B из `HANDOFF_STATUS.txt`):

- эффекты способностей, батчи 1–4 (48 `effect_key`);
- PvP-бой двумя игроками (в т.ч. `handlePhazeMove`);
- три UI-фикса из `TEST_ui_fixes2.txt`.

Очередь: механики gen3/gen4/gen5/gen6 (`MISSING_MOVES_gen3.txt`…`gen6.txt`) → поколения gen7–gen9 по `HANDOFF_gen7.txt`…`HANDOFF_gen9.txt` (skill `adding-pokemon-generations`) → квесты или экономика (`DESIGN_economy.txt`) → кланы/аватар. Известный баг: чат в `game.js` коннектится к `ws://localhost:8080/chat`, серверного обработчика нет.

## Definition of done

1. `go build ./...` и `go vet ./...` зелёные.
2. `node --check` для изменённых JS.
3. Для данных — `PRAGMA integrity_check` + проверка ключевых выборок.
4. Живой прогон затронутого сценария (wild / trainer / PvP — что менялось).
5. Регрессия соседнего режима боя, если трогался общий движок.
6. Запись в `TEST_<тема>.txt`; при новых архитектурных решениях — обновить `AI_MEMORY.md` / `HANDOFF_STATUS.txt`.
