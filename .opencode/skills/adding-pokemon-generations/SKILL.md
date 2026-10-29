---
name: adding-pokemon-generations
description: Use when adding a new Pokémon generation (gen4/gen5/gen6/gen7/gen8/gen9) to pokemon-game — PokeAPI import of species, stats, types, genders, evolutions, learnsets, abilities, Showdown sprites, MISSING_MOVES_genN and TEST_genN reports. Triggers: "добавить поколение", gen4, gen5, gen6, gen7, gen8, gen9, Sinnoh, Unova, Kalos, Alola, Galar, Paldea, PokeAPI import.
---

# Добавление нового поколения покемонов

Этот skill — пошаговый рецепт для gen4–gen9. Он повторяет проверенный
проход gen3 (#252–386). Данные живут в SQLite, Go-код для нового вида
обычно не нужен: движок, покедекс, админка и бой работают с любым id.

Перед стартом прочитай:
- skill `pokemon-game-core` — конвенции проекта;
- skill `db-data-migrations` — схема БД, бэкапы, проверки;
- `HANDOFF_STATUS.txt` — что уже сделано и какие хвосты открыты;
- `TEST_gen3.txt` — эталон отчёта и фактических чисел.

## 0. Сначала согласовать объём с владельцем

По умолчанию (как в gen3):
- все виды диапазона, включая легендарных/мифических;
- анимированные спрайты Showdown в `normal/` и `mini/`, без shiny;
- лёрнсеты — только атаки, которые уже есть в `attacks`; недостающие
  атаки добавляются как ДАННЫЕ, механики — отдельной задачей;
- способности назначаются виду, включая hidden (slot 3); недостающие
  строки каталога добавляются как данные с пустым `effect_key`;
- эволюции — только в пределах целевого поколения и кросс-ген
  эволюции ИЗ более старых поколений В целевое; эволюции в следующие
  поколения НЕ добавляются;
- предметы эволюции не заводятся — метод ставится без `item_id`,
  список уходит в `MISSING_MOVES_genN.txt`;
- локации дикой ловли не трогаются (владелец расставит в админке);
- формы (regional, Mega, Gigantamax, Rotom, Deoxys и т.п.) не заводятся
  отдельными видами, если владелец не попросил иное.

Если ответы владельца отличаются — зафиксируй их в `HANDOFF_genN.txt`
и в `TEST_genN.txt`.

## 1. Диапазоны и источники

| Поколение | Регион | Диапазон | Видов | Версия PokeAPI для лёрнсетов |
|---|---|---|---|---|
| gen4 | Sinnoh | #387–493 | 107 | `platinum` (fallback: `diamond-pearl`, `heartgold-soulsilver`) |
| gen5 | Unova | #494–649 | 156 | `black-2-white-2` (fallback: `black-white`) |
| gen6 | Kalos | #650–721 | 72 | `omega-ruby-alpha-sapphire` (fallback: `x-y`) |
| gen7 | Alola | #722–809 | 88 | `ultra-sun-ultra-moon` (fallback: `sun-moon`) |
| gen8 | Galar | #810–905 | 96 | `sword-shield` |
| gen9 | Paldea | #906–1025 | 120 | `scarlet-violet` |

PokeAPI (проверено, доступен):
- `https://pokeapi.co/api/v2/generation/{N}` — виды/атаки/способности поколения;
- `https://pokeapi.co/api/v2/pokemon/{id}` — статы, типы, способности, рост, вес, base_experience, moves;
- `https://pokeapi.co/api/v2/pokemon-species/{id}` — gender_rate, capture_rate, is_legendary, is_mythical, evolution_chain;
- `https://pokeapi.co/api/v2/evolution-chain/{id}` — родители/условия;
- `https://pokeapi.co/api/v2/move/{name}` — данные недостающей атаки.

Спрайты: `https://play.pokemonshowdown.com/sprites/ani/{slug}.gif`.
Slug — английское имя вида из PokeAPI (для базовой формы; у special
форм пробовать имя без суффикса). Если 404 — fallback
`sprites/gen5/{slug}.png` или `sprites/dex/{slug}.png`, но тогда
`image_url` должен указывать на `.png`.

Кэш ответов и скрипты держи ВНЕ репозитория (scratchpad), в git — только
итоговые данные, спрайты и отчёты. PokeAPI периодически отдаёт SSL EOF:
делай retry 5–8 раз с паузой и сохраняй каждый ответ в JSON-кэш сразу.

## 2. Подготовка БД

1. Убедись, что сервер не пишет в БД. Если рядом работает другой агент,
   дождись паузы или спроси владельца.
2. Сделай бэкап через sqlite3 backup API (не просто Copy-Item при
   активном сервере):

```python
import sqlite3, datetime, os
src = sqlite3.connect(r'D:\pokemon-game\database\pokemon.db')
stamp = datetime.datetime.now().strftime('%Y%m%d-%H%M%S')
dst = sqlite3.connect(rf'D:\pokemon-game\database\backups\pokemon.db.bak-before-genN-{stamp}')
src.backup(dst)
dst.close(); src.close()
```

3. Открой соединение с `timeout=120`, `PRAGMA busy_timeout=120000`,
   `PRAGMA foreign_keys=ON`.
4. Все вставки — в одной транзакции `BEGIN IMMEDIATE` … `COMMIT`,
   при ошибке `ROLLBACK`.

## 3. Виды: маппинг полей

Для каждого id диапазона:

| `pokemons` | Источник / правило |
|---|---|
| `id` | национальный номер = id PokeAPI |
| `name` | русская транслитерация, UNIQUE; сверь с существующими именами |
| `description` | НЕПУСТАЯ строка, иначе `/full` и `/search` ломаются |
| `height` / `weight` | PokeAPI `height/10`, `weight/10` |
| `base_*` | PokeAPI stats (hp/attack/defense/speed/special-attack/special-defense) |
| `category_power` | PokeAPI `base_experience` (иначе 0 опыта за победу) |
| `experience_group` | `3` (как у всех видов в БД) |
| `rarity` | `1` (как gen2/gen3) |
| `male_percentage` / `female_percentage` | `gender_rate`: -1→0/0, 0→100/0, 1→87/13, 2→75/25, 4→50/50, 6→25/75, 7→12/88, 8→0/100 |
| `primary_type_id` / `secondary_type_id` | по именам из `types`; secondary = NULL, если один тип |
| `catch_rate` | PokeAPI `capture_rate` |
| `image_url` | `/static/images/pokemons/normal/NNN.gif` (или `.png`, если gif нет) |
| `evolves_from_id` и методы | см. §4 |

Важно: БД использует СОВРЕМЕННЫЕ типы (Fairy уже есть у gen1–2), поэтому
бери текущий `types` из PokeAPI, а не «тип на момент поколения».

## 4. Эволюции

Построй карту `child -> parent + evolution_details` из всех
`evolution-chain` целевых видов.

Правила:
- `parent` и `child` должны существовать в БД. Для нового поколения
  `child <= max_id` целевого поколения; ссылки на более поздние
  поколения НЕ создавать.
- Для новых видов выстави `evolves_from_id`, `evolution_method_id`,
  `evolution_level`, `evolution_item_id`.
- Кросс-ген эволюции ИЗ старых поколений В целевое — ВХОДЯТ в задачу:
  обнови старые строки (`pokemon_id` родителя уже есть) только если
  новый вид реально добавлен.
- Беби-формы новых поколений — тоже: у старого вида появляется
  `evolves_from_id` на нового беби.
- Не перезаписывай чужие/более поздние эволюции; перед UPDATE
  проверь, что целевой вид существует и ссылка канонична.
- Методы: `1=level`, `2=trade`, `3=stone`, `4=friendship`, `5=item`.
  Для `use-item` камни → 3, прочие предметы → 5, но `evolution_item_id`
  оставляй NULL, если предмета нет в `items` (и запиши в MISSING).
- `level-up` с `min_level` → method 1 + level.
- `level-up` с `min_happiness` → method 4 без level.
- `level-up` с `min_beauty` / `min_affection` / `time_of_day` /
  `known_move` / `location` / `relative_physical_stats` — приближай
  разумным методом (обычно friendship) и ОБЯЗАТЕЛЬНО фиксируй
  ограничение в `MISSING_MOVES_genN.txt`.
- `trade` → method 2; held item не добавляй в items.
- `shed`/special → `evolves_from_id=NULL`, запись в MISSING.
- Развилки (Wurmple, Eevee, Tyrogue, Clamperl, Applin и т.п.): БД хранит
  одну строку на ребёнка; обе ветки можно задать, но движок level-up
  выберет первую по id. Зафиксируй это в MISSING.
- Проверь отсутствие циклов и «висящих» `evolves_from_id`.

## 5. Лёрнсеты

Для каждого вида выбери `version_group_details` по версии из §1
(приоритет — первая доступная из списка).

Маппинг методов:
- `level-up` → `learn_method_id=1`, уровень = минимальный уровень
  этого хода (level 0 → 1);
- `machine` → 2, уровень 0;
- `egg` → 3, уровень 0;
- `tutor` → 4, уровень 0.

Сопоставление с `attacks` — по `eng_name` с нормализацией:
`re.sub(r'[^a-z0-9]', '', name.lower())`. В БД встречаются расхождения:
`Solarbeam` vs `Solar Beam`, `Double-Edge` vs `Double Edge`,
`Will-o-wisp` vs `Will O Wisp` — нормализация обязательна.

Если атаки нет в `attacks`:
1. выпиши её в `MISSING_MOVES_genN.txt`;
2. если владелец разрешил (по умолчанию — да), добавь строку в `attacks`
   с каноничными `type_id/category_id/power/accuracy/pp/priority/target_id/contact`
   и следующим свободным id;
3. если эффект выражается существующими данными (`effect_status_id`,
   `attack_stat_changes`, `never_misses`, `crit_rate`, `hits_min/max`),
   заполни их; иначе оставь механику на отдельную задачу.

Гарантируй, что у КАЖДОГО вида есть хотя бы одна level-up урон-атака.
Если канон даёт только статусы (Silcoon/Cascoon/Wynaut-подобные), добавь
базовую атаку по типу и отметь фолбэк в TEST.

## 6. Способности

`abilities` не имеет `eng_name` — только русское имя и `effect_key`.
Поэтому:
1. собери все английские имена способностей видов из `/pokemon`;
2. смапь их на существующие русские строки по смыслу/описанию/effect_key
   (не по алфавиту id!);
3. отсутствующие заведи новыми строками: русское имя, описание,
   `effect_key=NULL` (механики — отдельная задача);
4. база → slot 1/2; `is_hidden=1` → slot 3;
5. дедуплицируй по `(pokemon_id, ability_id)`; hidden не должен
   дублировать базовую;
6. для видов из СТАРЫХ поколений, которым способности только теперь
   появились, забэкфиль `user_pokemons.ability_id/ability_slot`
   (случайная базовая, slot 1/2) — как `backfillInstanceAbilities`.

Проверь: у каждого вида диапазона есть хотя бы одна способность;
все английские имена смаплены; `slot=3` ⇔ `is_hidden=1`.

## 7. Спрайты

Скачай `ani/{slug}.gif`, проверь HTTP 200 и `Content-Type: image/gif`,
сохрани как `client/static/images/pokemons/normal/NNN.gif` и скопируй
тем же файлом в `mini/NNN.gif` (исторически папки дублируются).
Если gif нет — fallback PNG и `.png` в `image_url`.
Проверь, что файл не пустой и открывается как изображение.
Shiny (`shine/`) не трогай, если не просили.

## 8. Запись и идемпотентность

- Виды: `INSERT OR IGNORE` по `id`; не перезаписывай существующие
  строки (владелец мог править имя/описание).
- Атаки: ищи по нормализованному `eng_name`; если нет — вставляй с
  явным следующим id.
- Лёрнсеты: `INSERT OR IGNORE` по PK `(pokemon_id, attack_id, learn_method_id)`.
- Способности: `INSERT OR IGNORE` по PK `(pokemon_id, ability_id)`.
- Не делай `DELETE`/`UPDATE` чужих данных без явной необходимости.
- Если параллельно работает другой агент: короткая транзакция,
  `busy_timeout`, свежий бэкап, повторное чтение max(id) внутри транзакции.

## 9. Проверки (DoD)

Обязательно:
1. `go build ./...`, `go vet ./...`, `go test ./... -count=1`.
2. `node --check client/static/js/pokedex.js` (и `game.js`, если трогал).
3. `PRAGMA integrity_check = ok`.
4. Count видов диапазона = ожидаемому; min/max id верные.
5. У каждого вида: все base-статы > 0, типы валидны, пол задан,
   `description` непустой, `category_power > 0`, `catch_rate > 0`,
   `image_url` непустой и файл существует, есть способность,
   есть хотя бы одна урон-атака.
6. Нет дублей `pokemons.name`, `attacks.name`, `attacks.eng_name`.
7. Нет сирот в `pokemon_attacks`/`pokemon_abilities`.
8. Нет `evolves_from_id` на несуществующий вид и на поколение позже
   целевого.
9. Сверь выборку статов/типов/пола/роста/веса/catch/base_exp с PokeAPI —
   расхождений быть не должно.
10. Регрессия по бэкапу: строки старых поколений не изменились, кроме
    согласованных кросс-ген эволюций/способностей.
11. HTTP-смоук: `/api/pokedex/list` содержит последний id,
    `/api/pokedex/pokemon/{id}/full` отдаёт способности/атаки,
    спрайт отдаётся 200.
12. Живой браузерный прогон: покедекс + создание/выдача вида + бой
    (wild/trainer), если возможно. Если нет — честно написать в TEST.

Известный pre-existing дефект: `PRAGMA foreign_key_check` показывает
`attacks.effect_status_id=9` без строки в `statuses`; это НЕ дефект
нового поколения. Не «исправляй» его молча.

## 10. Отчёты и документация

- `MISSING_MOVES_genN.txt`: недостающие атаки/предметы/условия эволюции,
  механики, которые не реализованы.
- `TEST_genN.txt`: сколько видов/атак/способностей/лёрнсетов добавлено,
  источники, версия лёрнсетов, ограничения, проверки, бэкап.
- Обнови `HANDOFF_STATUS.txt`: поколение, числа, очередь, хвосты.
- Обнови `pokedex.js` fallback `maxPokemonId` на новый максимум.
- Если добавлял новые способности/атаки — обнови соответствующие
  справочные числа в skill `db-data-migrations`.
- Ничего не коммить без явной просьбы владельца.

## 11. Очередь задач

Файлы ТЗ на каждое поколение (по одной задаче на поколение):
`HANDOFF_gen4.txt`, `HANDOFF_gen5.txt`, `HANDOFF_gen6.txt`,
`HANDOFF_gen7.txt`, `HANDOFF_gen8.txt`, `HANDOFF_gen9.txt`.
Каждый файл ссылается на этот skill и содержит только специфику
поколения (диапазон, версия, кросс-ген эволюции, формы, особые условия).
