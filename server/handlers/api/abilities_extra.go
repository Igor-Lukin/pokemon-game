package api

// abilities_extra.go — батчи 7+ : расширенные способности.
//
// Здесь собраны хуки, которых не было в abilities_battle.go (батчи 1–6):
// множители урона по тегам атак, иммунитеты к типам/звуку/снарядам, реакции
// на статы/крит/нокдаун, погодные и полевые ауры, контактные эффекты.
// Старые хуки вызывают эти функции как fallback, поэтому поведение пустого
// AbilityKey остаётся нейтральным (1.0 / false / no-op).

import (
	mathrand "math/rand"
	"strconv"
	"strings"
)

// Типы (остальные константы лежат в тематических файлах).
const (
	typeNormal   = 1
	typeWater    = 3
	typeIce      = 6
	typeFighting = 7
	typePsychic  = 11
	typeBug      = 12
	typeDragon   = 15
	typeDark     = 16
	typeFairy    = 18
)

// ---------------------------------------------------------------------------
// Классификация атак
// ---------------------------------------------------------------------------

func moveEngLower(m *BattleMove) string {
	if m == nil {
		return ""
	}
	return strings.ToLower(m.EngName)
}

func moveEngHas(m *BattleMove, subs ...string) bool {
	eng := moveEngLower(m)
	if eng == "" {
		return false
	}
	for _, s := range subs {
		if strings.Contains(eng, s) {
			return true
		}
	}
	return false
}

func moveIsSound(m *BattleMove) bool {
	if m == nil {
		return false
	}
	return m.Sound || moveEngHas(m, "sound", "boomburst", "hyper voice", "sing", "roar", "screech", "supersonic", "uproar", "snore", "bug buzz", "disarming voice", "echoed voice", "round", "confide", "noble roar", "overdrive", "torch song")
}

func moveIsPunch(m *BattleMove) bool {
	return moveEngHas(m, "punch", "fist")
}

func moveIsBite(m *BattleMove) bool {
	return moveEngHas(m, "bite", "fang", "jaw")
}

func moveIsSlice(m *BattleMove) bool {
	return moveEngHas(m, "slash", "cut", "blade", "scythe", "sword", "claw", "razor", "leaf blade", "sacred sword", "behemoth blade")
}

func moveIsPulse(m *BattleMove) bool {
	return moveEngHas(m, "pulse", "aura sphere", "origin pulse", "dragon energy")
}

func moveIsBall(m *BattleMove) bool {
	return moveEngHas(m, "ball", "bomb", "barrage", "bullet", "cannon", "shot", "seed", "orb", "sphere", "blast")
}

func moveIsWind(m *BattleMove) bool {
	return moveEngHas(m, "wind", "gust", "hurricane", "twister", "air cutter", "heat wave", "fairy wind", "bleakwind", "springtide", "sandsear", "wildbolt")
}

func moveIsDance(m *BattleMove) bool {
	return moveEngHas(m, "dance")
}

func moveIsHealing(m *BattleMove) bool {
	return m != nil && (m.HealPercent > 0 || m.Drain > 0)
}

// moveContactFor — контактность с учётом Длиннорукого.
func moveContactFor(attacker *BattlePokemon, m *BattleMove) bool {
	return m != nil && m.Contact && (attacker == nil || attacker.AbilityKey != "long_reach")
}

// battleHasAbility — есть ли способность у любого активного покемона.
func battleHasAbility(bs *BattleState, key string) bool {
	if bs == nil {
		return false
	}
	if bs.ActiveIdx >= 0 && bs.ActiveIdx < len(bs.PlayerTeam) &&
		bs.PlayerTeam[bs.ActiveIdx].AbilityKey == key {
		return true
	}
	return bs.WildPokemon != nil && bs.WildPokemon.AbilityKey == key
}

func abilityWeatherNegated(bs *BattleState) bool {
	return battleHasAbility(bs, "cloud_nine") || battleHasAbility(bs, "air_lock")
}

// sideForAbility — безопасный sideFor (не паникует на пустой команде в тестах).
func sideForAbility(bs *BattleState, p *BattlePokemon) *SideState {
	if bs == nil {
		return nil
	}
	if bs.ActiveIdx >= 0 && bs.ActiveIdx < len(bs.PlayerTeam) &&
		p == &bs.PlayerTeam[bs.ActiveIdx] {
		return &bs.PlayerSide
	}
	return &bs.WildSide
}

func abilityKeyIn(p *BattlePokemon, keys ...string) bool {
	if p == nil {
		return false
	}
	for _, k := range keys {
		if p.AbilityKey == k {
			return true
		}
	}
	return false
}

func abilityBlocksIndirect(p *BattlePokemon) bool {
	return p != nil && p.AbilityKey == "magic_guard"
}

func abilityHPBelowHalf(p *BattlePokemon) bool {
	return p != nil && p.MaxHP > 0 && p.CurrentHP*2 < p.MaxHP
}

func abilityFaintedAllies(bs *BattleState, self *BattlePokemon) int {
	if bs == nil {
		return 0
	}
	n := 0
	for i := range bs.PlayerTeam {
		if &bs.PlayerTeam[i] != self && bs.PlayerTeam[i].IsFainted {
			n++
		}
	}
	for i := range bs.EnemyTeam {
		if &bs.EnemyTeam[i] != self && bs.EnemyTeam[i].IsFainted {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// Скорость / приоритет / точность / криты / число ударов
// ---------------------------------------------------------------------------

func abilitySpeedMultExtra(bs *BattleState, p *BattlePokemon) float64 {
	if p == nil {
		return 1.0
	}
	mult := 1.0
	switch p.AbilityKey {
	case "sand_rush":
		if bs != nil && bs.BattleWeatherID == weatherSand {
			mult *= 2
		}
	case "slush_rush":
		if bs != nil && bs.BattleWeatherID == weatherHail {
			mult *= 2
		}
	case "surge_surfer":
		if bs != nil && bs.TerrainID == terrainElectric && bs.TerrainTurns > 0 {
			mult *= 2
		}
	case "quick_feet":
		if p.StatusID > 1 {
			mult *= 1.5
		}
	case "slow_start":
		if bs != nil && bs.Turn <= 5 {
			mult *= 0.5
		}
	case "unburden":
		if p.HeldItemID == 0 {
			mult *= 2
		}
	}
	return mult
}

// abilityPriorityExtra — дополнительный приоритет хода.
func abilityPriorityExtra(bs *BattleState, p *BattlePokemon, m *BattleMove) int {
	if p == nil || m == nil {
		return 0
	}
	pr := 0
	switch p.AbilityKey {
	case "prankster":
		if m.CategoryID == 3 {
			pr++
		}
	case "gale_wings":
		if m.TypeID == typeFlying && p.CurrentHP == p.MaxHP {
			pr++
		}
	case "triage":
		if moveIsHealing(m) {
			pr += 3
		}
	case "stall":
		return -10
	}
	return pr
}

func abilityMovePriority(bs *BattleState, p *BattlePokemon, m *BattleMove) int {
	if m == nil {
		return 0
	}
	return m.Priority + abilityPriorityExtra(bs, p, m)
}

// abilityBlocksPriorityMove — Дивный шарм/Проворство блокируют приоритетные атаки.
func abilityBlocksPriorityMove(defender *BattlePokemon, m *BattleMove) bool {
	if defender == nil || m == nil || m.Priority <= 0 {
		return false
	}
	return abilityKeyIn(defender, "queenly_majesty", "dazzling")
}

func abilityAlwaysHits(attacker, defender *BattlePokemon) bool {
	return abilityKeyIn(attacker, "no_guard") || abilityKeyIn(defender, "no_guard")
}

func abilityAccuracyMultExtra(attacker, defender *BattlePokemon, m *BattleMove) float64 {
	mult := 1.0
	if m == nil {
		return mult
	}
	if attacker != nil && attacker.AbilityKey == "victory_star" {
		mult *= 1.1
	}
	if defender != nil && defender.AbilityKey == "wonder_skin" && m.CategoryID == 3 {
		mult *= 0.5
	}
	return mult
}

func abilityCritStageExtra(attacker *BattlePokemon) int {
	if attacker != nil && attacker.AbilityKey == "super_luck" {
		return 1
	}
	return 0
}

func abilityAlwaysCrit(attacker *BattlePokemon) bool {
	return attacker != nil && attacker.AbilityKey == "mercy"
}

func abilityMaxHits(attacker *BattlePokemon) bool {
	return attacker != nil && attacker.AbilityKey == "skill_link"
}

// ---------------------------------------------------------------------------
// Исходящий / входящий урон
// ---------------------------------------------------------------------------

func abilityOutgoingMultExtra(bs *BattleState, attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) float64 {
	if attacker == nil || move == nil || move.CategoryID == 3 {
		return 1.0
	}
	mult := 1.0
	key := attacker.AbilityKey

	switch key {
	case "pixilate", "aerilate", "refrigerate", "galvanize", "normalize":
		// +20% уже заложен преобразованием типа/мощности атаки.
	case "water_bubble":
		if move.TypeID == typeWater {
			mult *= 2
		}
	case "protosynthesis":
		if bs != nil && bs.BattleWeatherID == weatherSun {
			mult *= 1.3
		}
	case "quark_drive":
		if bs != nil && bs.TerrainID == terrainElectric && bs.TerrainTurns > 0 {
			mult *= 1.3
		}
	case "orichalcum_pulse":
		if bs != nil && bs.BattleWeatherID == weatherSun && move.CategoryID == 1 {
			mult *= 1.33
		}
	case "hadron_engine":
		if bs != nil && bs.TerrainID == terrainElectric && bs.TerrainTurns > 0 && move.CategoryID == 2 {
			mult *= 1.33
		}
	case "steelworker":
		if move.TypeID == typeSteel {
			mult *= 1.5
		}
	case "dragon_maw":
		if move.TypeID == typeDragon {
			mult *= 1.5
		}
	case "transistor":
		if move.TypeID == typeElectric {
			mult *= 1.5
		}
	case "rocky_payload":
		if move.TypeID == typeRock {
			mult *= 1.5
		}
	case "flare_boost":
		if attacker.StatusID == 3 && move.CategoryID == 2 {
			mult *= 1.5
		}
	case "toxic_boost":
		if (attacker.StatusID == 6 || attacker.StatusID == 7) && move.CategoryID == 1 {
			mult *= 1.5
		}
	case "iron_fist":
		if moveIsPunch(move) {
			mult *= 1.2
		}
	case "strong_jaw":
		if moveIsBite(move) {
			mult *= 1.5
		}
	case "sharpness":
		if moveIsSlice(move) {
			mult *= 1.5
		}
	case "mega_launcher":
		if moveIsPulse(move) {
			mult *= 1.5
		}
	case "punk_rock":
		if moveIsSound(move) {
			mult *= 1.3
		}
	case "tinted_lens":
		if getTypeEffectiveness(move.TypeID, getPokemonTypes(defender)) < 1.0 {
			mult *= 2
		}
	case "neuroforce":
		if getTypeEffectiveness(move.TypeID, getPokemonTypes(defender)) > 1.0 {
			mult *= 1.25
		}
	case "analytic":
		if attacker.MovedSecond {
			mult *= 1.3
		}
	case "ambush":
		if bs != nil && defender.SwitchedInTurn == bs.Turn {
			mult *= 2
		}
	case "rivalry":
		if attacker.Gender != "" && defender.Gender != "" && attacker.Gender != "N" && defender.Gender != "N" {
			if attacker.Gender == defender.Gender {
				mult *= 1.25
			} else {
				mult *= 0.75
			}
		}
	case "slow_start":
		if bs != nil && bs.Turn <= 5 && move.CategoryID == 1 {
			mult *= 0.5
		}
	case "defeatist":
		if abilityHPBelowHalf(attacker) {
			mult *= 0.5
		}
	case "soul_heart", "compassion":
		mult *= 1.0 + 0.1*float64(abilityFaintedAllies(bs, attacker))
	case "supreme_overlord":
		mult *= 1.0 + 0.1*float64(abilityFaintedAllies(bs, attacker))
	case "swarm", "overgrow", "blaze", "torrent", "solar_power", "technician", "sheer_force", "reckless", "tough_claws", "flash_fire", "pure_power", "huge_power":
		// Уже обработано в abilities_battle.go.
	}

	// Полевые ауры (действуют на всех активных).
	if move.TypeID == typeFairy && (battleHasAbility(bs, "fairy_aura") || key == "fairy_aura") {
		if battleHasAbility(bs, "aura_break") || key == "aura_break" {
			mult *= 0.75
		} else {
			mult *= 1.33
		}
	}
	if move.TypeID == typeDark && (battleHasAbility(bs, "dark_aura") || key == "dark_aura") {
		if battleHasAbility(bs, "aura_break") || key == "aura_break" {
			mult *= 0.75
		} else {
			mult *= 1.33
		}
	}
	// Союзные ауры в одиночном бою усиливают самого носителя.
	if key == "power_spot" {
		mult *= 1.3
	}
	if move.TypeID == typeSteel && key == "steely_spirit" {
		mult *= 1.5
	}
	if move.CategoryID == 2 && key == "ion_crown" {
		mult *= 1.3
	}
	return mult
}

func abilityIncomingMultExtra(bs *BattleState, attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) float64 {
	if defender == nil || move == nil || move.CategoryID == 3 {
		return 1.0
	}
	mult := 1.0
	if bs != nil && sideForAbility(bs, defender).FriendGuardTurns > 0 {
		mult *= 0.75
	}
	switch defender.AbilityKey {
	case "water_bubble":
		if move.TypeID == typeFire {
			mult *= 0.5
		}
	case "heatproof":
		if move.TypeID == typeFire {
			mult *= 0.5
		}
	case "ice_scales":
		if move.CategoryID == 2 {
			mult *= 0.5
		}
	case "multiscale", "primordial_armor":
		if defender.CurrentHP == defender.MaxHP {
			mult *= 0.5
		}
	case "marvel_scale":
		if defender.StatusID > 1 && move.CategoryID == 1 {
			mult *= 0.67
		}
	case "grass_pelt":
		if bs != nil && bs.TerrainID == terrainGrassy && bs.TerrainTurns > 0 && move.CategoryID == 1 {
			mult *= 0.67
		}
	case "purifying_salt":
		if move.TypeID == typeGhost {
			mult *= 0.5
		}
	case "punk_rock":
		if moveIsSound(move) {
			mult *= 0.5
		}
	case "vessel_of_ruin":
		if move.CategoryID == 2 {
			mult *= 0.8
		}

	case "sword_of_ruin":
		// Меч разрушения снижает Защиту окружающих: владелец сам получает
		// физический урон как обычно, эффект учтён на исходящей стороне.
	}
	// Таблички/Бусы разрушения у атакующего уже учтены как исходящий множитель.
	if attacker != nil && attacker.AbilityKey == "sword_of_ruin" && move.CategoryID == 1 {
		mult *= 1.25
	}
	if attacker != nil && attacker.AbilityKey == "vessel_of_ruin" && move.CategoryID == 2 {
		mult *= 1.25
	}
	if defender.AbilityKey == "tablets_of_ruin" && move.CategoryID == 1 {
		mult *= 0.8
	}
	return mult
}

// ---------------------------------------------------------------------------
// Поглощение атак и иммунитеты
// ---------------------------------------------------------------------------

func abilityAbsorbsMoveExtra(bs *BattleState, attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) bool {
	if defender == nil || move == nil || abilityIgnoredBy(attacker) {
		return false
	}
	key := defender.AbilityKey
	switch key {
	case "lightning_rod":
		if move.TypeID == typeElectric {
			abilityLog(defender, round, log, "", "заземляет электричество — Сп.Атака +1")
			abilityBumpStat(defender, 4, 1, round, log)
			return true
		}
	case "water_defense":
		if move.TypeID == typeWater {
			abilityLog(defender, round, log, "", "поглощает воду — Защита +2")
			abilityBumpStat(defender, 2, 2, round, log)
			return true
		}
	case "earth_eater":
		if move.TypeID == typeGround {
			abilityHealMsg(defender, defender.MaxHP/4, round, log, "поглощает землю и восстанавливает 1/4 HP")
			return true
		}
	case "well_baked_body":
		if move.TypeID == typeFire {
			abilityLog(defender, round, log, "", "поглощает огонь — Защита +2")
			abilityBumpStat(defender, 2, 2, round, log)
			return true
		}
	case "wind_rider":
		if moveIsWind(move) {
			abilityLog(defender, round, log, "", "оседлал ветер — Атака +1")
			abilityBumpStat(defender, 1, 1, round, log)
			return true
		}
	case "bulletproof":
		if moveIsBall(move) {
			abilityLog(defender, round, log, "", "отражает снаряд бронёй")
			return true
		}
	case "soundproof":
		if moveIsSound(move) {
			abilityLog(defender, round, log, "", "блокирует звуковую атаку")
			return true
		}
	case "wonder_guard", "ephemeral":
		eff := getTypeEffectiveness(move.TypeID, getPokemonTypes(defender))
		if eff <= 1.0 && move.CategoryID != 3 {
			abilityLog(defender, round, log, "", "блокирует неэффективную атаку")
			return true
		}
	}
	return false
}

func abilityBlocksStatusMoveExtra(bs *BattleState, attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) bool {
	if defender == nil || move == nil || move.CategoryID != 3 {
		return false
	}
	if defender.AbilityKey == "good_as_gold" {
		abilityLog(defender, round, log, "", "не поддаётся статусным атакам")
		return true
	}
	if defender.AbilityKey == "aroma_veil" {
		switch move.ID {
		case moveTaunt, moveTorment, moveEncore, moveDisable, 515, 518, 373, 379: // mental/block moves
			abilityLog(defender, round, log, "", "защищён ароматом от воздействия на сознание")
			return true
		}
	}
	if attacker != nil && attacker.AbilityKey == "prankster" {
		if pokemonHasType(defender, typeDark) {
			abilityLog(attacker, round, log, "", "Озорник не действует на Тёмных")
			return true
		}
	}
	return false
}

func abilityBlocksStatusExtra(bs *BattleState, target *BattlePokemon, statusID int) bool {
	if target == nil {
		return false
	}
	switch target.AbilityKey {
	case "water_veil", "water_bubble":
		return statusID == 3
	case "magma_armor":
		return statusID == 4
	case "purifying_salt":
		return statusID >= 2 && statusID <= 7
	case "meteor_armor":
		return target.CurrentHP*2 >= target.MaxHP && statusID >= 2 && statusID <= 7
	case "sweet_veil":
		// Сторона с Патокой не засыпает (см. SideState).
		if bs == nil {
			return false
		}
		return statusID == 5 && sideForAbility(bs, target).SweetVeilTurns > 0
	}
	return false
}

func abilityBlocksStatDropExtra(target *BattlePokemon, statIdx int) bool {
	if target == nil {
		return false
	}
	switch target.AbilityKey {
	case "hyper_cutter":
		return statIdx == 1
	case "full_metal_body":
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Смена, конец хода, контакт, нокдаун
// ---------------------------------------------------------------------------

func abilityExtraSwitchIn(bs *BattleState, p *BattlePokemon, log *[]BattleLogEntry) {
	if p == nil || bs == nil {
		return
	}
	switch p.AbilityKey {
	case "sand_stream":
		abilityLog(p, bs.Turn, log, "", "вызывает песчаную бурю")
		setBattleWeather(bs, weatherSand, log)
	case "orichalcum_pulse":
		abilityLog(p, bs.Turn, log, "", "вызывает яркое солнце")
		setBattleWeather(bs, weatherSun, log)
	case "hadron_engine":
		abilityLog(p, bs.Turn, log, "", "создаёт Электрическую арену")
		setTerrain(bs, terrainElectric, bs.Turn, log)
	case "grassy_surge":
		setTerrain(bs, terrainGrassy, bs.Turn, log)
	case "misty_surge":
		setTerrain(bs, terrainMisty, bs.Turn, log)
	case "psychic_surge":
		setTerrain(bs, terrainPsychic, bs.Turn, log)
	case "electric_surge":
		setTerrain(bs, terrainElectric, bs.Turn, log)
	case "screen_cleaner":
		bs.PlayerSide.ReflectTurns, bs.PlayerSide.LightScreenTurns = 0, 0
		bs.WildSide.ReflectTurns, bs.WildSide.LightScreenTurns = 0, 0
		abilityLog(p, bs.Turn, log, "", "очищает экраны на поле")
	case "armor_tail":
		side := sideForAbility(bs, p)
		side.LightScreenTurns = 5
		side.LightScreenSetTurn = bs.Turn
		abilityLog(p, bs.Turn, log, "", "поднимает Экран света")
	case "intrepid_sword":
		abilityLog(p, bs.Turn, log, "", "повышает Атаку на 1")
		abilityBumpStat(p, 1, 1, bs.Turn, log)
	case "dauntless_shield":
		abilityLog(p, bs.Turn, log, "", "повышает Защиту на 1")
		abilityBumpStat(p, 2, 1, bs.Turn, log)
	case "steely_spirit":
		abilityLog(p, bs.Turn, log, "", "повышает Атаку команды на 1")
		abilityBumpStat(p, 1, 1, bs.Turn, log)
	case "download":
		if opp := abilityOpponentOf(bs, p); opp != nil {
			if opp.Defense < opp.SpecialDefense {
				abilityBumpStat(p, 1, 1, bs.Turn, log)
			} else {
				abilityBumpStat(p, 4, 1, bs.Turn, log)
			}
		}
	case "anticipation", "forewarn":
		if opp := abilityOpponentOf(bs, p); opp != nil {
			best := ""
			bestPow := 0
			for i := range opp.Moves {
				m := &opp.Moves[i]
				pw := m.Power
				if m.CategoryID != 3 && getTypeEffectiveness(m.TypeID, getPokemonTypes(p)) > 1.0 {
					pw += 100
				}
				if pw > bestPow {
					bestPow, best = pw, m.Name
				}
			}
			if best != "" {
				abilityLog(p, bs.Turn, log, "", "предчувствует опасную атаку: "+best)
			}
		}
	case "trace":
		if opp := abilityOpponentOf(bs, p); opp != nil {
			p.AbilityKey, p.AbilityName = opp.AbilityKey, opp.AbilityName
			abilityLog(p, bs.Turn, log, "", "копирует способность противника")
		}
	case "imposter":
		if opp := abilityOpponentOf(bs, p); opp != nil {
			handleTransformMove(p, opp, &BattleMove{ID: moveTransform}, bs.Turn, log)
		}
	case "illusion":
		// Копируем облик последнего живого члена команды.
		p.illusionName, p.illusionImage = p.Name, p.ImageURL
		for i := len(bs.PlayerTeam) - 1; i >= 0; i-- {
			if &bs.PlayerTeam[i] != p && !bs.PlayerTeam[i].IsFainted {
				p.Name, p.ImageURL = bs.PlayerTeam[i].Name, bs.PlayerTeam[i].ImageURL
				break
			}
		}
	case "multitype", "type_item":
		if p.HeldItemTypeID > 0 {
			p.BattleTypes = []int{p.HeldItemTypeID}
		}
	case "wind_rider":
		if sideForAbility(bs, p).TailwindTurns > 0 {
			abilityBumpStat(p, 1, 1, bs.Turn, log)
		}
	case "flower_gift":
		if bs.BattleWeatherID == weatherSun {
			abilityBumpStat(p, 1, 1, bs.Turn, log)
			abilityBumpStat(p, 4, 1, bs.Turn, log)
		}
	case "coma":
		if p.StatusID <= 1 {
			p.StatusID = 5
			p.SleepTurns = 99
		}
	case "zero_to_hero":
		if p.abilityLogSeen == nil {
			p.abilityLogSeen = map[string]bool{}
		}
		if !p.abilityLogSeen["zero_hero"] {
			p.abilityLogSeen["zero_hero"] = true
			abilityBumpStat(p, 1, 1, bs.Turn, log)
			abilityBumpStat(p, 4, 1, bs.Turn, log)
			abilityBumpStat(p, 3, 1, bs.Turn, log)
		}
	case "plus", "positive_charge":
		abilityBumpStat(p, 4, 1, bs.Turn, log)
	case "sweet_syrup":
		if opp := abilityOpponentOf(bs, p); opp != nil {
			abilityBumpStat(opp, 3, -1, bs.Turn, log)
		}
	case "guard_dog":
		if opp := abilityOpponentOf(bs, p); opp != nil {
			abilityBumpStat(opp, 1, -1, bs.Turn, log)
			opp.NoEscape = true
		}
	case "magnet_pull":
		if opp := abilityOpponentOf(bs, p); opp != nil && pokemonHasType(opp, typeSteel) {
			opp.NoEscape = true
		}
	case "arena_trap":
		if opp := abilityOpponentOf(bs, p); opp != nil && !hasTypeID(getPokemonTypes(opp), typeFlying) {
			opp.NoEscape = true
		}
	case "shadow_tag":
		if opp := abilityOpponentOf(bs, p); opp != nil {
			opp.NoEscape = true
		}
	case "sweet_veil":
		side := sideForAbility(bs, p)
		side.SweetVeilTurns = 5
		side.SweetVeilSetTurn = bs.Turn
	case "flower_veil":
		side := sideForAbility(bs, p)
		side.FlowerVeilTurns = 5
		side.FlowerVeilSetTurn = bs.Turn
	case "friend_guard":
		side := sideForAbility(bs, p)
		side.FriendGuardTurns = 5
		side.FriendGuardSetTurn = bs.Turn
	}
}

func clampStage(v int) int {
	if v > 6 {
		return 6
	}
	if v < -6 {
		return -6
	}
	return v
}

func abilityExtraSwitchOut(bs *BattleState, p *BattlePokemon, round int, log *[]BattleLogEntry) {
	if p == nil {
		return
	}
	switch p.AbilityKey {
	case "natural_cure":
		if p.StatusID > 1 {
			p.StatusID = 1
			p.SleepTurns, p.PoisonTurns = 0, 0
			abilityLog(p, round, log, "", "избавляется от статуса при замене")
		}
	case "magnet_pull", "arena_trap", "shadow_tag", "guard_dog":
		if opp := abilityOpponentOf(bs, p); opp != nil {
			opp.NoEscape = false
		}
	case "illusion":
		if p.illusionName != "" {
			p.Name, p.ImageURL = p.illusionName, p.illusionImage
			p.illusionName, p.illusionImage = "", ""
		}
	}
}

func abilityExtraEndTurn(bs *BattleState, p *BattlePokemon, round int, log *[]BattleLogEntry) {
	if p == nil || p.IsFainted {
		return
	}
	switch p.AbilityKey {
	case "bad_dreams":
		if opp := abilityOpponentOf(bs, p); opp != nil && !opp.IsFainted && opp.StatusID == 5 {
			abilityWeatherHP(opp, -opp.MaxHP/8, "страдает от кошмаров — −1/8 HP", round, log)
		}
	case "moody":
		up := mathrand.Intn(7) + 1
		down := mathrand.Intn(7) + 1
		abilityBumpStat(p, up, 2, round, log)
		abilityBumpStat(p, down, -1, round, log)
	case "healer":
		if mathrand.Intn(100) < 30 && p.StatusID > 1 {
			p.StatusID = 1
			p.SleepTurns, p.PoisonTurns = 0, 0
			abilityLog(p, round, log, "", "излечивает статус союзника")
		}
	case "ice_face":
		if bs != nil && bs.BattleWeatherID == weatherHail {
			p.DisguiseBroken = false
		}
	case "rage_shell":
		if abilityHPBelowHalf(p) && !p.abilityLogSeen["rage_shell"] {
			if p.abilityLogSeen == nil {
				p.abilityLogSeen = map[string]bool{}
			}
			p.abilityLogSeen["rage_shell"] = true
			abilityBumpStat(p, 2, -1, round, log)
			abilityBumpStat(p, 5, -1, round, log)
			abilityBumpStat(p, 1, 1, round, log)
			abilityBumpStat(p, 4, 1, round, log)
			abilityBumpStat(p, 3, 1, round, log)
		}
	case "zen_mode":
		if abilityHPBelowHalf(p) && !p.abilityLogSeen["zen_mode"] {
			if p.abilityLogSeen == nil {
				p.abilityLogSeen = map[string]bool{}
			}
			p.abilityLogSeen["zen_mode"] = true
			abilityBumpStat(p, 1, 1, round, log)
			abilityBumpStat(p, 3, 1, round, log)
			abilityBumpStat(p, 4, -1, round, log)
			abilityBumpStat(p, 5, -1, round, log)
		}
	}
}

func abilityExtraAfterHit(bs *BattleState, attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) {
	if defender == nil || defender.IsFainted {
		return
	}
	switch defender.AbilityKey {
	case "stamina":
		abilityLog(defender, round, log, "stamina", "повышает Защиту на 1")
		abilityBumpStat(defender, 2, 1, round, log)
	case "berserk":
		if abilityHPBelowHalf(defender) && !defender.abilityLogSeen["berserk"] {
			if defender.abilityLogSeen == nil {
				defender.abilityLogSeen = map[string]bool{}
			}
			defender.abilityLogSeen["berserk"] = true
			abilityLog(defender, round, log, "berserk", "впадает в ярость — Сп.Атака +1")
			abilityBumpStat(defender, 4, 1, round, log)
		}
	case "anger_point":
		// Вызывается только при критическом ударе (см. executeDamagingMove).
		if attacker != nil && attacker.AbilityKey == "mercy" {
			return
		}
	case "justified":
		if move != nil && move.TypeID == typeDark {
			abilityLog(defender, round, log, "", "гневается — Атака +1")
			abilityBumpStat(defender, 1, 1, round, log)
		}
	case "rattled":
		if move != nil && (move.TypeID == typeBug || move.TypeID == typeGhost || move.TypeID == typeDark) {
			abilityLog(defender, round, log, "", "пугается — Скорость +1")
			abilityBumpStat(defender, 3, 1, round, log)
		}
	case "thermal_exchange":
		if move != nil && move.TypeID == typeFire {
			abilityBumpStat(defender, 1, 1, round, log)
		} else if move != nil && move.TypeID == typeIce {
			abilityBumpStat(defender, 3, 1, round, log)
		}
	case "steam_engine":
		if move != nil && (move.TypeID == typeFire || move.TypeID == typeWater) {
			abilityLog(defender, round, log, "", "разгоняется паром — Скорость +6")
			defender.StatStages[3] = 6
		}
	case "wind_power":
		if move != nil && moveIsWind(move) {
			defender.ChargedElectric = true
			abilityLog(defender, round, log, "", "заряжается от ветра")
		}

	case "weak_armor":
		// Уже обработано в abilities_battle.go.
	}
}

func abilityExtraContact(bs *BattleState, attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) {
	if attacker == nil || defender == nil || attacker.IsFainted {
		return
	}
	chance := func(pct int) bool { return mathrand.Intn(100) < pct }
	status := func(id int) {
		if abilityBlocksStatus(bs, attacker, id) {
			return
		}
		inflictMajorStatus(attacker, id, round, log)
	}
	switch defender.AbilityKey {
	case "effect_spore":
		if chance(30) {
			status([]int{2, 6, 5}[mathrand.Intn(3)])
		}
	case "poison_touch":
		if chance(30) {
			status(6)
		}
	case "cursed_body":
		if chance(30) {
			attacker.DisabledMoveID = attacker.LastUsedMoveID
			attacker.DisableTurns = 4
			abilityLog(defender, round, log, "", "запрещает повторно использовать атаку")
		}
	case "mummy":
		if attacker.AbilityKey != "mummy" {
			attacker.AbilityKey, attacker.AbilityName = "mummy", "Мумификация"
			abilityLog(defender, round, log, "", "меняет способность атакующего на Мумификацию")
		}
	case "wandering_spirit":
		attacker.AbilityKey, defender.AbilityKey = defender.AbilityKey, attacker.AbilityKey
		attacker.AbilityName, defender.AbilityName = defender.AbilityName, attacker.AbilityName
		abilityLog(defender, round, log, "", "обменивается способностями с атакующим")
	case "pickpocket":
		stealHeldItem(attacker, defender, round, log)
	case "sticky_aroma":
		abilityBumpStat(attacker, 3, -1, round, log)
	case "tangled_hair", "cotton_down":
		abilityBumpStat(attacker, 3, -1, round, log)
	case "perish_body":
		attacker.PerishCount, defender.PerishCount = 3, 3
		abilityLog(defender, round, log, "", "оба обречены")
	case "sand_spit":
		if bs != nil {
			setBattleWeather(bs, weatherSand, log)
		}
	case "toxic_debris":
		if side := sideForAbility(bs, attacker); side != nil {
			side.ToxicSpikesLayers = 2
			abilityLog(defender, round, log, "", "разбрасывает токсичные шипы")
		}
	case "seed_sower":
		if bs != nil {
			setTerrain(bs, terrainGrassy, bs.Turn, log)
		}
	}
}

// abilityBlocksFirstHit — Маскировка/Ледяное лицо: первый удар не наносит урона.
func abilityBlocksFirstHit(defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) bool {
	if defender == nil || defender.DisguiseBroken {
		return false
	}
	switch defender.AbilityKey {
	case "disguise":
		defender.DisguiseBroken = true
		abilityLog(defender, round, log, "", "маскировка поглощает первый удар")
		return true
	case "ice_face":
		if move != nil && move.CategoryID != 1 {
			return false
		}
		defender.DisguiseBroken = true
		abilityLog(defender, round, log, "", "ледяное лицо выдерживает первый удар")
		return true
	}
	return false
}

// abilityLiquidOoze — Гниющая кожа: дренаж вредит атакующему вместо лечения.
// hpBeforeDrain — HP атакующего ДО applyDrain (нужно, чтобы отменить лечение и
// уйти в минус на величину дренажа, даже если HP был у максимума).
func abilityLiquidOoze(attacker, defender *BattlePokemon, move *BattleMove, dmg, hpBeforeDrain, round int, log *[]BattleLogEntry) {
	if attacker == nil || defender == nil || move == nil || move.Drain <= 0 || dmg <= 0 {
		return
	}
	if defender.AbilityKey != "liquid_ooze" {
		return
	}
	heal := dmg * move.Drain / 100
	if heal < 1 {
		heal = 1
	}
	actualGain := attacker.CurrentHP - hpBeforeDrain
	// Отменяем лечение и наносим урон на величину дренажа.
	attacker.CurrentHP -= actualGain + heal
	if attacker.CurrentHP < 0 {
		attacker.CurrentHP = 0
	}
	abilityLog(defender, round, log, "", "отравляет дренаж — атакующий теряет "+strconv.Itoa(heal)+" HP")
	if attacker.CurrentHP <= 0 {
		attacker.IsFainted = true
	}
}

// abilityExtraOnHit — эффекты атакующего после успешного урона.
func abilityExtraOnHit(bs *BattleState, attacker, defender *BattlePokemon, move *BattleMove, round int, log *[]BattleLogEntry) {
	if attacker == nil || defender == nil || defender.IsFainted || move == nil {
		return
	}
	switch attacker.AbilityKey {
	case "toxic_chain":
		if mathrand.Intn(100) < 30 && !abilityBlocksStatus(bs, defender, 7) {
			inflictMajorStatus(defender, 7, round, log)
		}
	case "poison_puppeteer":
		if defender.StatusID == 6 || defender.StatusID == 7 {
			inflictConfusion(defender, round, log)
		}
	case "stench":
		if mathrand.Intn(100) < 10 {
			inflictFlinch(defender, round, log)
		}
	case "magician":
		stealHeldItem(defender, attacker, round, log)
	}
}

func stealHeldItem(from, thief *BattlePokemon, round int, log *[]BattleLogEntry) {
	if from == nil || thief == nil || from.HeldItemID == 0 || thief.HeldItemID != 0 {
		return
	}
	if abilityKeyIn(from, "sticky_hold") {
		abilityLog(from, round, log, "", "удерживает предмет — украсть нельзя")
		return
	}
	thief.HeldItemID, thief.HeldItemName, thief.HeldItemImage = from.HeldItemID, from.HeldItemName, from.HeldItemImage
	thief.HeldItemTypeID, thief.HeldItemEffect = from.HeldItemTypeID, from.HeldItemEffect
	thief.HeldAtkMult = from.HeldAtkMult
	thief.HeldItemDescription = from.HeldItemDescription
	from.HeldItemID, from.HeldItemName, from.HeldItemImage = 0, "", ""
	from.HeldItemTypeID, from.HeldItemEffect, from.HeldAtkMult, from.HeldItemDescription = 0, "", 1, ""
	abilityLog(thief, round, log, "", "крадёт предмет противника")
}

func abilityExtraOnKO(bs *BattleState, attacker, defender *BattlePokemon, round int, log *[]BattleLogEntry) {
	if attacker == nil || attacker.IsFainted {
		return
	}
	switch attacker.AbilityKey {
	case "moxie", "chilling_neigh":
		abilityBumpStat(attacker, 1, 1, round, log)
	case "grim_neigh":
		abilityBumpStat(attacker, 4, 1, round, log)
	case "beast_boost":
		best, bestVal := 1, attacker.Attack
		for _, i := range []int{2, 3, 4, 5} {
			v := []int{0, attacker.Attack, attacker.Defense, attacker.Speed, attacker.SpecialAttack, attacker.SpecialDefense}[i]
			if v > bestVal {
				best, bestVal = i, v
			}
		}
		abilityBumpStat(attacker, best, 1, round, log)
	case "battle_bond":
		abilityBumpStat(attacker, 1, 1, round, log)
	}
}

// abilityStatReact — реакция на изменение стата в applyStatChanges.
// Возвращает (новые ступени, новый target, skip).
func abilityStatReact(attacker, defender, target *BattlePokemon, statIdx, stages, round int, log *[]BattleLogEntry, defSide *SideState) (int, *BattlePokemon, bool) {
	if target == nil || stages == 0 {
		return stages, target, false
	}
	// Простота: любые изменения статов удваиваются.
	if target.AbilityKey == "simple" {
		stages *= 2
	}
	// Противоположность: знак меняется.
	if target.AbilityKey == "contrary" {
		stages = -stages
	}
	// Зеркальная броня: снижение от противника отражается на атакующего.
	if target == defender && target.AbilityKey == "mirror_armor" && stages < 0 && attacker != nil && attacker != target {
		abilityLog(target, round, log, "", "отражает изменение статов обратно")
		return stages, attacker, false
	}
	// Цветокос: сторона защищена от снижения статов.
	if stages < 0 && defSide != nil && defSide.FlowerVeilTurns > 0 && target == defender {
		abilityLog(target, round, log, "flower_veil", "аромат цветов защищает статы")
		return stages, target, true
	}
	// Неповиновение/Конкуренция: противник понизил стат — буст.
	if target == defender && stages < 0 && attacker != nil && attacker != target {
		switch target.AbilityKey {
		case "defiant":
			abilityBumpStat(target, 1, 2, round, log)
		case "competitive":
			abilityBumpStat(target, 4, 2, round, log)
		}
	}
	// Оппортунист: противник повысил свой стат — копируем.
	if target == attacker && stages > 0 && defender != nil && defender.AbilityKey == "opportunist" {
		abilityBumpStat(defender, statIdx, stages, round, log)
	}
	return stages, target, false
}

// abilityAdjustMove — преобразование типа/контактности атаки способностью.
func abilityAdjustMove(attacker *BattlePokemon, move *BattleMove) *BattleMove {
	if attacker == nil || move == nil {
		return move
	}
	m := *move
	changed := false
	switch attacker.AbilityKey {
	case "normalize":
		m.TypeID = typeNormal
		if m.Power > 0 {
			m.Power = m.Power * 6 / 5
		}
		changed = true
	case "pixilate":
		if m.TypeID == typeNormal {
			m.TypeID = typeFairy
			if m.Power > 0 {
				m.Power = m.Power * 6 / 5
			}
			changed = true
		}
	case "aerilate":
		if m.TypeID == typeNormal {
			m.TypeID = typeFlying
			if m.Power > 0 {
				m.Power = m.Power * 6 / 5
			}
			changed = true
		}
	case "refrigerate":
		if m.TypeID == typeNormal {
			m.TypeID = typeIce
			if m.Power > 0 {
				m.Power = m.Power * 6 / 5
			}
			changed = true
		}
	case "galvanize":
		if m.TypeID == typeNormal {
			m.TypeID = typeElectric
			if m.Power > 0 {
				m.Power = m.Power * 6 / 5
			}
			changed = true
		}
	case "liquid_voice":
		if moveIsSound(&m) {
			m.TypeID = typeWater
			changed = true
		}
	}
	if attacker.AbilityKey == "long_reach" {
		m.Contact = false
		changed = true
	}
	if changed {
		return &m
	}
	return move
}

// abilityOnMoveTypeChange — Протеан/Либеро: тип носителя = тип атаки.
func abilityOnMoveTypeChange(attacker *BattlePokemon, move *BattleMove) {
	if attacker == nil || move == nil || move.CategoryID == 3 {
		return
	}
	if attacker.AbilityKey == "protean" || attacker.AbilityKey == "libero" {
		attacker.BattleTypes = []int{move.TypeID}
	}
}

// abilityColorChange — Хамелеон: тип = тип последней полученной атаки.
func abilityColorChange(defender *BattlePokemon, move *BattleMove) {
	if defender == nil || move == nil || defender.AbilityKey != "color_change" {
		return
	}
	defender.BattleTypes = []int{move.TypeID}
}

// abilityOnFaintRevenge — предсмертная контратака (Декомпрессия/Отмщение).
func abilityOnFaintRevenge(attacker, defender *BattlePokemon, move *BattleMove, hpBefore, round int, log *[]BattleLogEntry) {
	if attacker == nil || defender == nil || attacker.IsFainted {
		return
	}
	dmg := 0
	switch defender.AbilityKey {
	case "aftermath": // Декомпрессия: урон = HP до смертельного удара
		dmg = hpBefore
	case "aftermath_quarter": // Отмщение: 1/4 макс. HP, только контакт
		if move == nil || !move.Contact {
			return
		}
		dmg = attacker.MaxHP / 4
	}
	if dmg < 1 {
		return
	}
	attacker.CurrentHP -= dmg
	if attacker.CurrentHP < 0 {
		attacker.CurrentHP = 0
	}
	abilityLog(defender, round, log, "", "контратакует — "+attacker.Name+" теряет "+strconv.Itoa(dmg)+" HP")
	if attacker.CurrentHP <= 0 {
		attacker.IsFainted = true
	}
}

// breakIllusion — первая же наносящая урон атака раскрывает Иллюзию.
func breakIllusion(p *BattlePokemon, round int, log *[]BattleLogEntry) {
	if p == nil || p.AbilityKey != "illusion" || p.illusionName == "" {
		return
	}
	p.Name, p.ImageURL = p.illusionName, p.illusionImage
	p.illusionName, p.illusionImage = "", ""
	abilityLog(p, round, log, "", "иллюзия разрушена")
}

// abilityOnCritical — реакции на полученный крит.
func abilityOnCritical(defender *BattlePokemon, round int, log *[]BattleLogEntry) {
	if defender == nil {
		return
	}
	if defender.AbilityKey == "anger_point" {
		abilityLog(defender, round, log, "", "впадает в ярость — Атака максимальна")
		defender.StatStages[1] = 6
	}
}

// abilityEffectChance — Изящество удваивает шанс вторичного эффекта.
func abilityEffectChance(attacker *BattlePokemon, chance int) int {
	if attacker != nil {
		switch attacker.AbilityKey {
		case "serene_grace":
			chance *= 2
		case "mycelium_might":
			chance = chance * 13 / 10
		}
	}
	if chance > 100 {
		chance = 100
	}
	return chance
}

// abilityBlocksSecondary — Чешуекрылое блокирует вторичные эффекты атак.
func abilityBlocksSecondary(defender *BattlePokemon) bool {
	return defender != nil && defender.AbilityKey == "shield_dust"
}

func abilityStageTextMark(n int) string {
	return strconv.Itoa(n)
}
