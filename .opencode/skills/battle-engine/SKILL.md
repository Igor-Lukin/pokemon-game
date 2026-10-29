---
name: battle-engine
description: Use when changing combat in pokemon-game - moves, damage, statuses, abilities, held items, weather, hazards, switches, wild/trainer/PvP battles. Covers battle.go, pvp.go, abilities_battle.go, processTurn, MoveHandler, SwitchHandler and the reusable battle primitives.
---

# Боевой движок pokemon-game

Один серверный stateful-движок обслуживает три режима. Не писать второй движок: PvP и тренерский бой переиспользуют примитивы дикого боя.

## Модель состояния

```go
// battle.go
type BattleState struct {
    PlayerTeam     []BattlePokemon
    ActiveIdx      int
    EnemyTeam      []BattlePokemon   // пусто для wild
    EnemyActiveIdx int               // -1 до выбора лида в PvP
    WildPokemon    *BattlePokemon    // ВСЕГДА указатель на активного врага
    BattleType     string            // "wild" | "trainer" | "pvp"
    EnemySmart     bool              // ИИ-замены/ходы NPC
    TrainerName    string
    // погода, экраны, hazards, комнаты, Water/Mud Sport, Gravity, Terrain ...
}
```

- Сессии: `battleSessions` + `battleMu` (одиночные), `pvpSessions` + `pvpMu` (PvP).
- `PvpBattle` держит `BS *BattleState`: сторона A = `PlayerTeam`, сторона B = `EnemyTeam` + `WildPokemon` (указательный трюк). `Phase`: `lead_select -> battling -> await_replace -> finished`.
- «Указательный трюк»: `WildPokemon` всегда указывает на активного врага. `EnemyTeam` строится один раз; смена врага — только индекс + переустановка указателя, никогда `append`.
- `BattleType`-гейты: ловля только `wild`, ИИ-замены только `trainer+EnemySmart`, опыт только `wild`, PvP без наград/опыта.

## Оркестраторы и входные точки

| Функция | Роль |
|---|---|
| `MoveHandler` (battle.go) | ход игрока в одиночном бою: атака + ответ врага |
| `SwitchHandler` (battle.go) | смена своего покемона + ответ врага, ловля-провал |
| `processTurn` | ИИ-оркестратор одиночного боя |
| `pvpResolveTurn` (pvp.go) | зеркало `processTurn` для двух людей, ход разрешается когда сдали оба |
| `pvpSwitchB` | смена стороны B, зеркало `performInBattleSwitch` |
| `pvpForceMove` | форс-состояния (заряд/перезарядка/фикс-ход) обеих сторон |
| `handleEnemyFaint` | падение врага: wild → победа/опыт, trainer → следующий живой |
| `buildPlayerTeam`, `playerPokemonToBattle` | сборка команды игрока из БД |
| `buildGenerateFor`, `wildToBattlePokemon` | генерация дикого и команды NPC (единое ядро) |
| `saveBattleState`, `savePvpState` | персист HP/статусов/PP/EV в `user_pokemons` |

## Переиспользуемые примитивы

Не копировать их, а вызывать:

- `executeDamagingMove` — нанесение урона + вторичные эффекты.
- `calculateDamage` — формула, STAB, типы, криты, held-множитель, ability-хуки.
- `checkHit` — точность/уклонение/способности.
- `applyEffectByID`, `applyStatChanges`, `applyHeal` — статусы, статы, лечение.
- `handleCustomStatusMove`, `applySideMove`, `handlePhazeMove`, `handleCallMove` — особые категории ходов.
- `applyEntryHazards` — единая точка входа на поле (stealth rock, spikes, ability onSwitchIn).
- `tickSideTimers` — конец хода (погода, экраны, hazards, `abilityOnEndTurn`).
- `setBattleWeather` — погода (в т.ч. из способностей).
- `sideFor` — получить сторону по атакующему/защищающемуся; в PvP до выбора лида `ActiveIdx == -1`, не звать бой до фазы `battling`.

Домены ходов разложены по файлам: `charge_moves.go`, `recharge`, `protect_moves.go`, `counter_moves.go`, `heal_moves.go`, `status_registry.go`, `screens.go`, `hazards.go`, `weather_effects.go`, `terrain_moves.go`, `room_moves.go`, `volatiles.go`, `gender_moves.go`, `formula_moves.go`, `copy_moves.go`, `transform_moves.go`, `call_moves.go`, `switch_moves.go`, `stat_swap_moves.go`, `stockpile_moves.go`, `future_moves.go`, `field_effects.go`, `fire_effects.go`, `block_moves.go`, `lock_moves.go`, `sacrifice_moves.go`, `special_dmg_moves.go`, `type_moves.go`, `sleep_moves.go`, `status_extra_moves.go`. Сначала искать существующий обработчик, потом писать новый.

## Добавление/правка атаки

1. Найти атаку по русскому названию в БД (`attacks`), проверить `category` (physical/special/status), `type_id`, `power`, `accuracy`, `effect_id`.
2. Проверить, нет ли уже обработчика в тематическом файле (см. список выше).
3. Если нужен новый эффект — добавить ветку в диспетчер (`applyEffectByID` / `handleCustomStatusMove` / специализированный файл), переиспользуя примитивы.
4. Проверить лёрнсет: `pokemon_attacks` (вид ↔ атака, level/tm/egg/tutor).
5. `go build ./... && go vet ./...`.
6. Прогнать в бою именно этот ход: wild + trainer; если ход затрагивает стороны/смену — ещё и PvP.

## Добавление/правка способности

1. `abilities.effect_key` — код эффекта (строка), `abilities.description` — русский текст.
2. Назначение видам — `pokemon_abilities` (`slot 1/2/3`, слот 3 ⇔ `is_hidden=1`).
3. Реализация — `switch` в `abilities_battle.go`. Существующие хуки:
   - `onSwitchIn` — в `applyEntryHazards` (и у лидов);
   - `modifyOutgoingDamage` / `modifyIncomingDamage` — в `calculateDamage`;
   - `blocksStatus` — в `applyEffectByID` перед наложением статуса;
   - `abilityAbsorbsMove` — поглощение урона по типу до расчёта;
   - `abilityAbsorbsStatusMove` — поглощение статус-атак по типу (self-target и погодные ходы исключены);
   - `abilityOnHitByMove` — ответ на контакт после урона;
   - `abilityAfterBeingHit` — реакция на любой физ. удар (Хрупкая броня);
   - `abilityOnEndTurn` — в `MoveHandler`, `SwitchHandler` и провале ловли после конца хода;
   - `abilityBlocksStatDrop` — в `applyStatChanges` (сейчас блокирует любые статы);
   - точность/крит: `abilityAccuracyMult`, `abilityIgnoresEvasion`, `abilityBlocksCrit`, уклонение в погоду;
   - формы по HP: `abilityUpdateForm` + таблица `pokemon_forms` (`db.FormFor`).
   - Хелперы: `abilityOpponentOf`, `abilityBumpStat`, `abilityHeal`, `abilityAbsorbType`.
   - Лог: `abilityLog(p, round, log, onceKey, msg)` → `[способность] Имя: msg`; `onceKey != ""` пишет один раз за бой (поле `abilityLogSeen`).
4. Новый хук помечать `// [ability hook]`, добавлять ровно в одну общую точку, а не в каждый обработчик.
5. Экземпляр получает способность в `utils.CreateUserPokemon`; при эволюции — `db.RemapAbilityOnEvolve` (два вызова: `evolution.go`, `levelup.go`).
6. Проверить: способность срабатывает у игрока, у дикого/NPC, и не ломает PvP.

## Известные грабли

- `ActiveIdx/EnemyActiveIdx == -1` до выбора лида в PvP — любой боевой примитив до `battling` паникует.
- `handlePhazeMove` (Рёв/Вихрь) в PvP не проверен: риск потерять указатель стороны B. При рассинхроне отключить для `BattleType == "pvp"`.
- Формы по HP (`schooling`/`shields_down`/`totem`) работают только если в `pokemon_forms` есть строки; для gen1–2 таблица пустая.
- При `effectiveness == 0` урон теперь 0 (раньше формула поднимала до 1 HP).
- Двойной обморок за ход → `need_switch`; карточка врага освежается при следующей смене.
- Победа над диким остаточным уроном в `SwitchHandler` — без опыта (сохранённое поведение, не регресс).
- `enemySwitched` — враг не должен бить после ИИ-замены при гибели от волатайла.
- При персисте обеих команд в PvP держать синхронными `pvpSwitchB` и `performInBattleSwitch` (очистка волатилей/выходов).
- `category_power` вида должен быть равен base_experience, иначе опыт за gen-N = 0.
- Не забыть `saveBattleState`/`savePvpState` после хода, иначе урон не сохранится.

## Матрица проверки

Любая правка движка:

1. wild: старт → ход → смена → победа/ловля/поражение.
2. trainer (random и fixed/smart): ИИ-ход, ИИ-замена, победа по всей команде, отсутствие опыта.
3. pvp: два браузера/инкогнито, выбор лида вслепую → ход/смена/замена → победа; урон сохраняется в реальные `user_pokemons`.
4. Регрессия: статусы, погода, hazards, held-предметы, способность игрока и врага.
