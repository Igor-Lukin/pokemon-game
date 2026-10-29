---
name: live-e2e-test
description: Use when verifying pokemon-game by running it - starting the Go server with SESSION_KEY, smoke tests, wild/trainer/PvP playtests with two users, DB before/after checks, logs, cleanup. Trigger on live run, playtest, go run ./server, localhost:8080, PvP test, regression.
---

# Живой прогон pokemon-game

Build/vet — не финальная проверка. DoD проекта — живой прогон затронутого сценария. Go на этой машине есть (в отличие от исторического RDP-окружения).

## Preflight

```powershell
cd D:\pokemon-game          # обязательно корень: БД = ./database/pokemon.db
git status --short
go build ./...
go vet ./...
node --check client/static/js/game.js
node --check client/static/js/pokedex.js

# бэкап перед прогоном, если тест пишет в БД
Copy-Item database/pokemon.db "database/backups/pokemon.db.bak-before-test" -Force
python -c "import sqlite3; c=sqlite3.connect(r'database/pokemon.db'); print(c.execute('PRAGMA integrity_check').fetchone()); c.close()"
```

## Запуск

Только через skill `start-pokemon-server` (detached, не вешает сессию):

```powershell
& .\.opencode\skills\start-pokemon-server\start-server.ps1
# server up pid=... http://localhost:8080
```

Остановка после теста:

```powershell
& .\.opencode\skills\start-pokemon-server\start-server.ps1 -Stop
```

Не использовать `go run ./server` в Bash-вызове: процесс держит сессию и
выглядит как зависание. Без `SESSION_KEY` сервер сразу падает
(`session.Init` → `log.Fatal`); скрипт выставляет его сам. После остановки
проверить, не остался ли `database/pokemon.db-journal`.

Smoke:
```powershell
Invoke-WebRequest -UseBasicParsing http://localhost:8080/ | Select-Object StatusCode
```

## Тестовые данные

- В БД 4 пользователя, 515 экземпляров, 251 вид. Пароли не печатать и не менять без нужды.
- Регистрация нового тестового игрока: `/register`. Для PvP нужно **два** аккаунта в **одной** локации (`users.location_id`).
- NPC для быстрого тренерского боя: Ричи (id 1, random 3 / 8–12); Тренер Ли (id 2, fixed ростер) — из `TEST_npc_battle.txt`.
- Онлайн считается по `users.last_seen < 300с`, запись троттлится 60с; PvP-вызов живёт 45с, сессия без активности — 3 мин → ничья.
- Для проверки способностей: `user_pokemons.ability_id/ability_slot` должны быть заполнены; `abilities.effect_key` — непустой (иначе способность инертна). Можно выдать/поменять через админку.

## Матрица прогона

**Дикий бой**: старт → атака → смена → ловля (только wild) → победа (опыт/эволюция) → поражение. Проверить расход бола и ход дикого при провале ловли.

**Тренерский бой**: random и fixed ростер; ИИ-ход, ИИ-замена после обморока, победа только по всей команде; опыт/ловля отключены; после боя HP/PP сохранились.

**PvP** (два браузера/инкогнито):
1. Оба в одной локации, панель онлайна видит друг друга.
2. Вызов → принять/отклонить; повторный вызов не создаётся.
3. Выбор лида вслепую → `battling` → ход/смена/замена после обморока → победа.
4. Урон сохраняется в реальные `user_pokemons` обеих сторон.
5. Отдельно проверить ходы со сменой внутри хода и `handlePhazeMove` (Рёв/Вихрь) — известный риск.

**Способности** (хвост B): blaze/torrent/overgrow/swarm при HP<1/3, huge/pure_power, thick_fat/fur_coat, иммунитеты к статусам, погодники, поглощение по типу, ответ на контакт, точность/криты. Проверять у игрока и у врага.

**UI-фиксы 2**: «Покедекс» из меню покемона открывает нужный вид; клик по способности показывает описание; ввод количества конфет полем (`+/-`, кламп, Enter).

## Диагностика

- Ошибки сервера — stderr процесса (там же логи погоды/боя).
- 401/редирект — сессионная кука; войти заново.
- Пустые ответы API — часто старый кэш: Ctrl+F5.
- `database is locked` — остановить сервер и повторить массовую запись.
- Паника `index out of range` в PvP — вероятно `ActiveIdx == -1` до `lead_select`.
- После прогона сверить БД: HP/PP/статусы, `ability_id`, отсутствие дублей.

## После теста

```powershell
python -c "import sqlite3; c=sqlite3.connect(r'database/pokemon.db'); print(c.execute('PRAGMA integrity_check').fetchone()); c.close()"
```

- Тестовые изменения данных откатить из бэкапа или осознанно оставить (тогда записать в `TEST_<тема>.txt`).
- Удалить/не коммитить `database/pokemon.db` (ignored) и `*-journal`.
- Результат оформить в `TEST_<тема>.txt`: что проверено, что сломано, что осталось.
