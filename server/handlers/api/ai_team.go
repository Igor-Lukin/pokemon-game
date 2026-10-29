package api

import (
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"

	"pokemon-game/server/db"
	"pokemon-game/server/utils"
)

// ai_team.go — тимбилдер NPC-бота (ai_level 1/2).
//
// Отличие от legacy buildSmartNPCTeam: бот выбирает не только виды, но и ходы,
// способность, характер, IV/EV и type_boost-предмет. Синергии кодируются
// тегами (hazard_set, screen_set, pivot, recovery, setup, immune:<type>), а не
// зашитыми архетипами. Погодные теги есть, но до отдельной задачи по погоде
// имеют нулевой вес.

type aiSpec struct {
	Tier             int
	MaxIV            int
	EVPerLevel       int
	MaxLegendaries   int
	RandomMoveChance int // % случайного легального хода (только L1)
	SearchDepth      int
}

func aiSpecForLevel(aiLevel int) aiSpec {
	switch aiLevel {
	case 1:
		return aiSpec{Tier: 1, MaxIV: 25, EVPerLevel: 2, MaxLegendaries: 0, RandomMoveChance: 20, SearchDepth: 1}
	case 2:
		return aiSpec{Tier: 2, MaxIV: 31, EVPerLevel: 3, MaxLegendaries: 1, RandomMoveChance: 0, SearchDepth: 2}
	case 3:
		return aiSpec{Tier: 2, MaxIV: 31, EVPerLevel: 3, MaxLegendaries: 4, RandomMoveChance: 0, SearchDepth: 2}
	case 4:
		return aiSpec{Tier: 2, MaxIV: 31, EVPerLevel: 3, MaxLegendaries: 6, RandomMoveChance: 0, SearchDepth: 2}
	default:
		// Неизвестный уровень не должен ломать бой — ведём себя как L2.
		return aiSpec{Tier: 2, MaxIV: 31, EVPerLevel: 3, MaxLegendaries: 1, RandomMoveChance: 0, SearchDepth: 2}
	}
}

type aiAbility struct {
	ID   int
	Name string
	Key  string
	Tags []string
}

type aiCandidate struct {
	smartCandidate
	moves     []GeneratedMove
	abilities []aiAbility
	tags      map[string]bool
}

type aiTeamState struct {
	teamState
	hazardSet  bool
	screenSet  bool
	pivot      bool
	recovery   bool
	setup      bool
	status     bool
	phaze      bool
	immunities map[int]bool
}

// implementedAbilityKeys — способности с реальным хуком в движке. Остальные
// бот не выбирает: иначе «лучшая» способность окажется пустышкой.
var implementedAbilityKeys = map[string]bool{
	"drought": true, "drizzle": true, "snow_warning": true,
	"intimidate": true, "speed_boost": true,
	"rain_dish": true, "ice_body": true, "shed_skin": true,
	"solar_power": true, "dry_skin": true,
	"clear_body": true, "keen_eye": true, "big_pecks": true,
	"flash_fire": true, "volt_absorb": true, "water_absorb": true,
	"motor_drive": true, "sap_sipper": true, "storm_drain": true,
	"rough_skin": true, "iron_barbs": true, "static": true,
	"flame_body": true, "poison_point": true, "gooey": true, "cute_charm": true,
	"sand_veil": true, "snow_cloak": true,
	"swarm": true, "overgrow": true, "blaze": true, "torrent": true,
	"pure_power": true, "huge_power": true, "reckless": true,
	"tough_claws": true, "thick_fat": true, "fur_coat": true,
	"fluffy": true, "filter": true, "sniper": true,
	"immunity": true, "limber": true, "insomnia": true, "vital_spirit": true,
	"levitate": true,
	// [batch6]
	"swift_swim": true, "chlorophyll": true, "sturdy": true, "inner_focus": true,
	"sheer_force": true, "pressure": true, "own_tempo": true, "oblivious": true,
	"infiltrator": true, "telepathy": true, "guts": true, "frisk": true,
	"hydration": true, "regenerator": true, "overcoat": true, "hustle": true,
	"leaf_guard": true, "technician": true, "mold_breaker": true, "damp": true,
	// [batch7]
	"lightning_rod": true, "tinted_lens": true, "synchronize": true, "water_defense": true,
	"flare_boost": true, "berserk": true, "aroma_veil": true, "water_bubble": true,
	"fairy_aura": true, "flower_gift": true, "pixilate": true, "stamina": true,
	"liquid_ooze": true, "aftermath": true, "defiant": true, "queenly_majesty": true,
	"long_reach": true, "simple": true, "friend_guard": true, "early_bird": true,
	"grass_pelt": true, "iron_fist": true, "liquid_voice": true, "download": true,
	"moxie": true, "steelworker": true, "ambush": true, "stall": true,
	"victory_star": true, "beast_boost": true, "soundproof": true, "aerilate": true,
	"stench": true, "serene_grace": true, "illusion": true, "forewarn": true, "anticipation": true,
	"power_spot": true, "ion_crown": true, "rock_head": true, "pickpocket": true, "coma": true,
	"competitive": true, "corrosion": true, "bad_dreams": true, "magma_armor": true,
	"sticky_hold": true, "magic_bounce": true, "magic_guard": true, "magnet_pull": true,
	"disguise": true, "skill_link": true, "moody": true, "misty_surge": true,
	"meteor_armor": true, "cloud_nine": true, "mercy": true, "protean": true,
	"water_veil": true, "refrigerate": true, "strong_jaw": true, "multitype": true,
	"mummy": true, "unaware": true, "slow_start": true, "heatproof": true,
	"prankster": true, "effect_spore": true, "aftermath_quarter": true, "defeatist": true,
	"plus": true, "mega_launcher": true, "sweet_veil": true, "triage": true,
	"primordial_armor": true, "marvel_scale": true, "sand_force": true, "aura_break": true,
	"anger_point": true, "justified": true, "natural_cure": true, "suction_cups": true,
	"adaptability": true, "arena_trap": true, "cursed_body": true, "truant": true,
	"normalize": true, "contrary": true, "bulletproof": true, "tangled_feet": true,
	"unburden": true, "klutz": true, "toxic_boost": true, "steadfast": true,
	"imposter": true, "analytic": true, "scrappy": true, "compassion": true,
	"type_item": true, "slush_rush": true, "rivalry": true, "no_guard": true,
	"super_luck": true, "dark_aura": true, "shadow_tag": true, "poison_touch": true,
	"light_metal": true, "grassy_surge": true, "heavy_metal": true, "hyper_cutter": true,
	"gale_wings": true, "rattled": true, "magician": true, "sand_stream": true,
	"color_change": true, "flower_veil": true, "healer": true, "shield_dust": true,
	"sand_rush": true, "quick_feet": true, "electric_surge": true, "trace": true,
	"wonder_guard": true, "multiscale": true, "wind_rider": true, "wonder_skin": true,
	"sharpness": true, "zen_mode": true, "galvanize": true, "surge_surfer": true,
	"battle_bond": true, "tangled_hair": true, "psychic_surge": true, "full_metal_body": true,
	"neuroforce": true, "chilling_neigh": true, "cotton_down": true, "dauntless_shield": true,
	"dragon_maw": true, "grim_neigh": true, "ice_face": true, "ice_scales": true,
	"intrepid_sword": true, "libero": true, "mirror_armor": true, "perish_body": true,
	"punk_rock": true, "sand_spit": true, "screen_cleaner": true, "steam_engine": true,
	"steely_spirit": true, "transistor": true, "unseen_fist": true, "wandering_spirit": true,
	"sticky_aroma": true, "seed_sower": true, "thermal_exchange": true, "rage_shell": true,
	"purifying_salt": true, "well_baked_body": true, "guard_dog": true, "rocky_payload": true,
	"wind_power": true, "zero_to_hero": true, "electromorphosis": true, "protosynthesis": true,
	"quark_drive": true, "good_as_gold": true, "vessel_of_ruin": true, "sword_of_ruin": true,
	"tablets_of_ruin": true, "beads_of_ruin": true, "orichalcum_pulse": true, "hadron_engine": true,
	"opportunist": true, "supreme_overlord": true, "toxic_debris": true, "armor_tail": true,
	"earth_eater": true, "mycelium_might": true, "sweet_syrup": true, "toxic_chain": true,
	"poison_puppeteer": true,
	// Формы, переключаемые ходом (Королевское построение).
	"stance_change": true,
}

// aiAbilityTags — синергетические теги способностей.
var aiAbilityTags = map[string][]string{
	"drizzle":      {"weather_set:rain"},
	"drought":      {"weather_set:sun"},
	"snow_warning": {"weather_set:hail"},
	"sand_stream":  {"weather_set:sand"},
	"rain_dish":    {"weather_benefit:rain"},
	"ice_body":     {"weather_benefit:hail"},
	"solar_power":  {"weather_benefit:sun", "offense"},
	"intimidate":   {"physical_control"},
	"speed_boost":  {"setup"},
	"water_absorb": {"immune:water"},
	"storm_drain":  {"immune:water"},
	"dry_skin":     {"immune:water", "weather_benefit:rain"},
	"volt_absorb":  {"immune:electric"},
	"motor_drive":  {"immune:electric"},
	"flash_fire":   {"immune:fire"},
	"sap_sipper":   {"immune:grass"},
	"levitate":     {"immune:ground"},
	"swarm":        {"pinch", "offense"},
	"overgrow":     {"pinch", "offense"},
	"blaze":        {"pinch", "offense"},
	"torrent":      {"pinch", "offense"},
	"pure_power":   {"offense"},
	"huge_power":   {"offense"},
	"reckless":     {"offense"},
	"tough_claws":  {"offense"},
	"sniper":       {"offense"},
	"thick_fat":    {"defense"},
	"fur_coat":     {"defense"},
	"fluffy":       {"defense"},
	"filter":       {"defense"},
	"clear_body":   {"defense"},
	"keen_eye":     {"defense"},
	"big_pecks":    {"defense"},
	"rough_skin":   {"contact_punish"},
	"iron_barbs":   {"contact_punish"},
	"static":       {"contact_punish"},
	"flame_body":   {"contact_punish"},
	"poison_point": {"contact_punish"},
	"gooey":        {"contact_punish"},
	"cute_charm":   {"contact_punish"},
	"sand_veil":    {"weather_benefit:sand"},
	"snow_cloak":   {"weather_benefit:hail"},
	"immunity":     {"status_immunity"},
	"limber":       {"status_immunity"},
	"insomnia":     {"status_immunity"},
	"vital_spirit": {"status_immunity"},
	"shed_skin":    {"status_immunity"},
	// [batch6]
	"swift_swim":   {"weather_benefit:rain", "speed"},
	"chlorophyll":  {"weather_benefit:sun", "speed"},
	"sturdy":       {"defense"},
	"inner_focus":  {"status_immunity"},
	"sheer_force":  {"offense"},
	"pressure":     {"defense"},
	"own_tempo":    {"status_immunity"},
	"oblivious":    {"status_immunity"},
	"infiltrator":  {"offense"},
	"telepathy":    {"defense"},
	"guts":         {"offense", "status_immunity"},
	"frisk":        {"defense"},
	"hydration":    {"weather_benefit:rain", "status_immunity"},
	"regenerator":  {"defense", "pivot"},
	"overcoat":     {"weather_benefit:sand", "weather_benefit:hail", "status_immunity"},
	"hustle":       {"offense"},
	"leaf_guard":   {"weather_benefit:sun", "status_immunity"},
	"technician":   {"offense"},
	"mold_breaker": {"offense"},
	"damp":         {"defense"},
}

// aiMoveTags — синергетические теги ходов по английскому имени (lowercase).
var aiMoveTags = map[string][]string{
	// hazards
	"stealth rock": {"hazard_set"}, "spikes": {"hazard_set"},
	"toxic spikes": {"hazard_set"}, "sticky web": {"hazard_set"},
	"defog": {"hazard_clear"}, "rapid spin": {"hazard_clear"},
	// screens / field
	"reflect": {"screen_set"}, "light screen": {"screen_set"},
	"aurora veil": {"screen_set"}, "safeguard": {"screen_set"},
	"mist": {"screen_set"}, "tailwind": {"tailwind"},
	"trick room": {"trick_room"},
	// pivots
	"u-turn": {"pivot"}, "volt switch": {"pivot"}, "baton pass": {"pivot"},
	"parting shot": {"pivot"}, "teleport": {"pivot"}, "flip turn": {"pivot"},
	// phazing
	"roar": {"phaze"}, "whirlwind": {"phaze"},
	"dragon tail": {"phaze"}, "circle throw": {"phaze"},
	// recovery
	"recover": {"recovery"}, "roost": {"recovery"}, "moonlight": {"recovery"},
	"morning sun": {"recovery"}, "synthesis": {"recovery"},
	"slack off": {"recovery"}, "soft-boiled": {"recovery"},
	"milk drink": {"recovery"}, "wish": {"recovery"}, "aqua ring": {"recovery"},
	"ingrain": {"recovery"}, "leech seed": {"recovery", "status"},
	"pain split": {"recovery"}, "rest": {"rest"},
	// setup
	"swords dance": {"setup"}, "nasty plot": {"setup"}, "calm mind": {"setup"},
	"dragon dance": {"setup"}, "quiver dance": {"setup"}, "bulk up": {"setup"},
	"iron defense": {"setup_defense"}, "agility": {"setup_speed"}, "rock polish": {"setup_speed"},
	"shell smash": {"setup"}, "coil": {"setup"}, "shift gear": {"setup"},
	"work up": {"setup"}, "hone claws": {"setup"}, "growth": {"setup"},
	"curse": {"setup"},
	// status
	"toxic": {"status"}, "thunder wave": {"status"}, "will-o-wisp": {"status"},
	"sleep powder": {"status"}, "spore": {"status"}, "hypnosis": {"status"},
	"glare": {"status"}, "yawn": {"status"}, "confuse ray": {"status"},
	// protect
	"protect": {"protect"}, "detect": {"protect"}, "endure": {"protect"},
	"king's shield": {"protect"}, "spiky shield": {"protect"},
	"baneful bunker": {"protect"}, "quick guard": {"protect"},
	"wide guard": {"protect"},
	// weather (вес 0 в v1)
	"rain dance": {"weather_set:rain"}, "sunny day": {"weather_set:sun"},
	"sandstorm": {"weather_set:sand"}, "hail": {"weather_set:hail"},
	"snowscape": {"weather_set:hail"}, "chilly reception": {"weather_set:hail"},
	// self-sacrifice
	"explosion": {"self_destruct"}, "self-destruct": {"self_destruct"},
	"final gambit": {"self_destruct"}, "memento": {"self_destruct"},
	"healing wish": {"self_destruct"}, "lunar dance": {"self_destruct"},
	"misty explosion": {"self_destruct"},
	// recharge
	"hyper beam": {"recharge"}, "blast burn": {"recharge"},
	"hydro cannon": {"recharge"}, "frenzy plant": {"recharge"},
	"giga impact": {"recharge"}, "roar of time": {"recharge"},
	"prismatic laser": {"recharge"},
}

func aiMoveHasTag(m GeneratedMove, tag string) bool {
	return aiMoveNameHasTag(m.EngName, tag)
}

func aiMoveNameHasTag(engName, tag string) bool {
	for _, t := range aiMoveTags[strings.ToLower(strings.TrimSpace(engName))] {
		if t == tag {
			return true
		}
	}
	return false
}

// loadAICandidates — финальные эволюции с ходами/способностями, загруженными
// пачкой (без N+1 запросов на каждый вид).
func loadAICandidates(level int) ([]aiCandidate, error) {
	base, err := loadSmartCandidates(level)
	if err != nil {
		return nil, err
	}
	if len(base) == 0 {
		return nil, fmt.Errorf("AI-пул кандидатов пуст")
	}

	ids := make([]int, len(base))
	for i := range base {
		ids[i] = base[i].id
	}
	movesBy, err := loadMovesForSpecies(ids, level)
	if err != nil {
		return nil, err
	}
	abilitiesBy, err := loadAbilitiesForSpecies(ids)
	if err != nil {
		return nil, err
	}

	out := make([]aiCandidate, 0, len(base))
	for _, c := range base {
		cand := aiCandidate{
			smartCandidate: c,
			moves:          movesBy[c.id],
			abilities:      abilitiesBy[c.id],
			tags:           map[string]bool{},
		}
		for _, m := range cand.moves {
			for _, tag := range aiMoveTags[strings.ToLower(strings.TrimSpace(m.EngName))] {
				cand.tags[tag] = true
			}
		}
		for _, a := range cand.abilities {
			for _, tag := range a.Tags {
				cand.tags[tag] = true
			}
		}
		out = append(out, cand)
	}
	return out, nil
}

func chunkInts(ids []int, size int) [][]int {
	if size < 1 {
		size = 1
	}
	var out [][]int
	for len(ids) > 0 {
		n := size
		if n > len(ids) {
			n = len(ids)
		}
		out = append(out, ids[:n])
		ids = ids[n:]
	}
	return out
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func loadMovesForSpecies(ids []int, level int) (map[int][]GeneratedMove, error) {
	out := map[int][]GeneratedMove{}
	for _, chunk := range chunkInts(ids, 400) {
		args := make([]interface{}, 0, len(chunk)+1)
		for _, id := range chunk {
			args = append(args, id)
		}
		args = append(args, level)
		q := fmt.Sprintf(`
			SELECT pa.pokemon_id, a.id, a.name, COALESCE(a.eng_name, ''), a.type_id,
			       a.category_id, a.power, a.accuracy, a.pp, a.priority,
			       COALESCE(a.target_id, 1), COALESCE(a.description, ''),
			       COALESCE(a.effect_status_id, 0), COALESCE(a.effect_chance, 0),
			       COALESCE(a.never_misses, 0), COALESCE(a.drain, 0),
			       COALESCE(a.crit_rate, 0), COALESCE(a.hits_min, 1),
			       COALESCE(a.hits_max, 1), COALESCE(a.disabled, 0),
			       COALESCE(a.heal_percent, 0), COALESCE(a.contact, 0),
			       COALESCE(a.sound, 0)
			FROM pokemon_attacks pa
			JOIN attacks a ON a.id = pa.attack_id
			WHERE pa.pokemon_id IN (%s)
			  AND ((pa.learn_method_id = 1 AND pa.level_learned_at <= ?)
			       OR pa.learn_method_id IN (2, 3, 4))
			GROUP BY pa.pokemon_id, a.id`, placeholders(len(chunk)))
		rows, err := db.DB.Query(q, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var speciesID int
			var m GeneratedMove
			var neverMisses, disabled, contact, sound int
			if err := rows.Scan(&speciesID, &m.ID, &m.Name, &m.EngName, &m.TypeID,
				&m.CategoryID, &m.Power, &m.Accuracy, &m.PP, &m.Priority,
				&m.TargetID, &m.Description, &m.EffectID, &m.EffectChance,
				&neverMisses, &m.Drain, &m.CritRate, &m.HitsMin, &m.HitsMax,
				&disabled, &m.HealPercent, &contact, &sound); err != nil {
				continue
			}
			m.NeverMisses = neverMisses != 0
			m.Disabled = disabled != 0
			m.Contact = contact != 0
			m.Sound = sound != 0
			out[speciesID] = append(out[speciesID], m)
		}
		rows.Close()
	}
	return out, nil
}

func loadAbilitiesForSpecies(ids []int) (map[int][]aiAbility, error) {
	out := map[int][]aiAbility{}
	for _, chunk := range chunkInts(ids, 400) {
		args := make([]interface{}, 0, len(chunk))
		for _, id := range chunk {
			args = append(args, id)
		}
		q := fmt.Sprintf(`
			SELECT pa.pokemon_id, ab.id, ab.name, COALESCE(ab.effect_key, '')
			FROM pokemon_abilities pa
			JOIN abilities ab ON ab.id = pa.ability_id
			WHERE pa.pokemon_id IN (%s)
			ORDER BY pa.pokemon_id, ab.id`, placeholders(len(chunk)))
		rows, err := db.DB.Query(q, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var speciesID int
			var a aiAbility
			if err := rows.Scan(&speciesID, &a.ID, &a.Name, &a.Key); err != nil {
				continue
			}
			a.Tags = aiAbilityTags[a.Key]
			out[speciesID] = append(out[speciesID], a)
		}
		rows.Close()
	}
	return out, nil
}

// buildAITeam — собрать команду для ai_level >= 1.
func buildAITeam(npcID, aiLevel, size, level int) ([]BattlePokemon, error) {
	_ = npcID
	if size < 1 {
		size = 1
	} else if size > 6 {
		size = 6
	}
	if level < 1 {
		level = 1
	}
	spec := aiSpecForLevel(aiLevel)

	// Сначала берём проверенный legacy-драфт (он лучше держит покрытие/роли/стенки),
	// затем пересобираем каждого покемона с AI-статами/ходами/способностью/предметом.
	legacyTeam, err := buildSmartNPCTeamWithLimit(size, level, spec.MaxLegendaries)
	if err != nil {
		return nil, err
	}
	team, err := buildAITeamFromLegacy(legacyTeam, aiLevel, level)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(team))
	for i := range team {
		names[i] = team[i].Name
	}
	log.Printf("[ai-team] L%d итог: %d покемонов ур.%d -> [%s]", aiLevel, len(team), level, strings.Join(names, ", "))
	return team, nil
}

// buildAITeamFromLegacy — пересобрать готовый legacy-драфт с AI-статами,
// способностью, характером, IV/EV и предметом. Виды и порядок слотов остаются
// legacy-драфта (он лучше держит покрытие/роли/стенки), меняется только начинка.
func buildAITeamFromLegacy(legacyTeam []BattlePokemon, aiLevel, level int) ([]BattlePokemon, error) {
	spec := aiSpecForLevel(aiLevel)
	pool, err := loadAICandidates(level)
	if err != nil {
		return nil, err
	}
	byID := make(map[int]aiCandidate, len(pool))
	for _, c := range pool {
		byID[c.id] = c
	}
	team := make([]BattlePokemon, 0, len(legacyTeam))
	for i := range legacyTeam {
		base := legacyTeam[i]
		c, ok := byID[base.PokemonID]
		if !ok {
			team = append(team, base)
			continue
		}
		bp, err := buildAIPokemon(c, spec, level)
		if err != nil {
			log.Printf("[ai-team] L%d #%d %s: fallback на legacy (%v)", aiLevel, base.PokemonID, base.Name, err)
			team = append(team, base)
			continue
		}
		team = append(team, *bp)
		log.Printf("[ai-team] L%d #%d %s ур.%d атаки=[%s] способность=%s характер=%d предмет=%s",
			aiLevel, bp.PokemonID, bp.Name, bp.Level, aiMoveNames(bp.Moves),
			bp.AbilityName, bp.NatureID, bp.HeldItemName)
	}
	alive := 0
	for i := range team {
		if !team[i].IsFainted && team[i].CurrentHP > 0 {
			alive++
		}
	}
	if len(team) == 0 || alive == 0 {
		return nil, fmt.Errorf("AI-команда пуста/без боеспособных")
	}
	// Порядок слотов legacy-драфта уже продуман (лид/стенки), не переставляем.
	return team, nil
}

func aiMoveNames(moves []BattleMove) string {
	names := make([]string, len(moves))
	for i := range moves {
		names[i] = moves[i].Name
	}
	return strings.Join(names, ", ")
}

// reorderAILead — лид выбирается ДО боя по своей команде (не под лид игрока):
// самый быстрый/сильный боец с хорошей урон-атакой. Это уменьшает соблазн
// ИИ менять лида на первом ходу и тратить ход.
func reorderAILead(team []BattlePokemon) {
	if len(team) < 2 {
		return
	}
	best := 0
	bestScore := -1e18
	for i := range team {
		p := &team[i]
		off := max(p.Attack, p.SpecialAttack)
		bestMove := 0
		for _, m := range p.Moves {
			if m.CategoryID != 3 && m.Power > bestMove {
				bestMove = m.Power
			}
		}
		score := float64(p.Speed)*0.6 + float64(off) + float64(p.MaxHP)*0.1 + float64(bestMove)*0.5
		if score > bestScore {
			bestScore = score
			best = i
		}
	}
	if best != 0 {
		team[0], team[best] = team[best], team[0]
	}
}

func aiSynergyBonus(c aiCandidate, st aiTeamState, spec aiSpec) float64 {
	_ = spec
	b := 0.0
	if c.tags["hazard_set"] && !st.hazardSet {
		b += 2.5
	}
	if c.tags["screen_set"] && !st.screenSet {
		b += 1.5
	}
	if c.tags["pivot"] && !st.pivot {
		b += 1.0
	}
	if c.tags["recovery"] && !st.recovery {
		b += 1.0
	}
	if c.tags["setup"] && !st.setup {
		b += 0.5
	}
	if c.tags["status"] && !st.status {
		b += 0.5
	}
	if c.tags["phaze"] && st.hazardSet {
		b += 1.0
	}
	for tag := range c.tags {
		if strings.HasPrefix(tag, "immune:") {
			if t, err := strconv.Atoi(strings.TrimPrefix(tag, "immune:")); err == nil && !st.immunities[t] {
				b += 0.5
			}
		}
	}
	// weather_set/weather_benefit — вес 0 до отдельной задачи по погоде.
	return b
}

func updateAITeamState(c aiCandidate, st *aiTeamState, eff map[int]map[int]float64, defenders map[int]bool, physThr, specThr float64) {
	st.roles[c.role]++
	if float64(c.baseDef) >= physThr {
		st.hasPhysWall = true
	}
	if float64(c.baseSpDef) >= specThr {
		st.hasSpecWall = true
	}
	for _, t := range c.stabTypes() {
		st.types[t] = true
	}
	for d := range defenders {
		if st.coverage[d] {
			continue
		}
		for _, s := range c.stabTypes() {
			if eff[s] != nil && eff[s][d] > 1 {
				st.coverage[d] = true
				break
			}
		}
	}
	st.hazardSet = st.hazardSet || c.tags["hazard_set"]
	st.screenSet = st.screenSet || c.tags["screen_set"]
	st.pivot = st.pivot || c.tags["pivot"]
	st.recovery = st.recovery || c.tags["recovery"]
	st.setup = st.setup || c.tags["setup"]
	st.status = st.status || c.tags["status"]
	st.phaze = st.phaze || c.tags["phaze"]
	for tag := range c.tags {
		if strings.HasPrefix(tag, "immune:") {
			if t, err := strconv.Atoi(strings.TrimPrefix(tag, "immune:")); err == nil {
				st.immunities[t] = true
			}
		}
	}
}

// aiSpeciesData — минимум для материализации бойца.
type aiSpeciesData struct {
	ID          int
	Name        string
	ImageURL    string
	PrimaryType int
	Secondary   *int
	Weight      float64
	Base        [6]int
}

func loadAISpeciesData(pokemonID int) (*aiSpeciesData, error) {
	var s aiSpeciesData
	var sec sql.NullInt64
	err := db.DB.QueryRow(`
		SELECT id, name, COALESCE(image_url, ''), primary_type_id, secondary_type_id,
		       COALESCE(weight, 0), base_hp, base_attack, base_defense,
		       base_speed, base_special_attack, base_special_defense
		FROM pokemons WHERE id = ?`, pokemonID).
		Scan(&s.ID, &s.Name, &s.ImageURL, &s.PrimaryType, &sec, &s.Weight,
			&s.Base[0], &s.Base[1], &s.Base[2], &s.Base[3], &s.Base[4], &s.Base[5])
	if err != nil {
		return nil, err
	}
	if sec.Valid {
		t := int(sec.Int64)
		s.Secondary = &t
	}
	return &s, nil
}

func buildAIPokemon(c aiCandidate, spec aiSpec, level int) (*BattlePokemon, error) {
	species, err := loadAISpeciesData(c.id)
	if err != nil {
		return nil, err
	}
	moves := chooseAIMoves(c, spec, level)
	if len(moves) == 0 {
		return nil, fmt.Errorf("нет урон-ходов")
	}
	physical := aiMovesPhysical(c, moves)
	natureID := chooseAINature(c, physical)
	natureFactors, err := utils.GetNatureFactors(natureID)
	if err != nil {
		natureFactors = [6]float64{1, 1, 1, 1, 1, 1}
	}
	iv := [6]int{spec.MaxIV, spec.MaxIV, spec.MaxIV, spec.MaxIV, spec.MaxIV, spec.MaxIV}
	ev := allocateAIEVs(c.role, physical, level, spec)
	train := [6]float64{1, 1, 1, 1, 1, 1}
	curHP, maxHP, atk, def, spd, spAtk, spDef := utils.CalculateStats(
		species.Base, iv, ev, level, natureFactors, train)

	ability := chooseAIAbility(c, c.role)
	itemID, itemType, itemMult, itemName, itemImage, itemDesc := chooseAIHeldItem(c, moves)
	gender, err := db.RollGender(c.id)
	if err != nil {
		gender = db.GenderNeutral
	}

	bp := &BattlePokemon{
		PokemonID:           c.id,
		Name:                species.Name,
		Level:               level,
		CurrentHP:           curHP,
		MaxHP:               maxHP,
		Attack:              atk,
		Defense:             def,
		Speed:               spd,
		SpecialAttack:       spAtk,
		SpecialDefense:      spDef,
		PrimaryTypeID:       species.PrimaryType,
		SecondaryTypeID:     species.Secondary,
		ImageURL:            species.ImageURL,
		Weight:              species.Weight,
		Gender:              gender,
		Moves:               generatedMovesToBattle(moves),
		IsFainted:           false,
		HeldAtkMult:         itemMult,
		HeldItemID:          itemID,
		HeldItemTypeID:      itemType,
		HeldItemName:        itemName,
		HeldItemImage:       itemImage,
		HeldItemDescription: itemDesc,
		NatureID:            natureID,
		IV:                  iv,
		EV:                  ev,
	}
	if ability.ID != 0 {
		bp.AbilityID = ability.ID
		bp.AbilityName = ability.Name
		bp.AbilityKey = ability.Key
	}
	snapshotBaseStats(bp)
	return bp, nil
}

func generatedMovesToBattle(moves []GeneratedMove) []BattleMove {
	out := make([]BattleMove, len(moves))
	for i, m := range moves {
		out[i] = BattleMove{
			ID:           m.ID,
			Name:         m.Name,
			EngName:      m.EngName,
			TypeID:       m.TypeID,
			CategoryID:   m.CategoryID,
			Power:        m.Power,
			Accuracy:     m.Accuracy,
			PP:           -1,
			MaxPP:        -1,
			Priority:     m.Priority,
			TargetID:     m.TargetID,
			Description:  m.Description,
			EffectID:     m.EffectID,
			EffectChance: m.EffectChance,
			NeverMisses:  m.NeverMisses,
			Drain:        m.Drain,
			HealPercent:  m.HealPercent,
			CritRate:     m.CritRate,
			HitsMin:      m.HitsMin,
			HitsMax:      m.HitsMax,
			Disabled:     m.Disabled,
			Contact:      m.Contact,
			Sound:        m.Sound,
			StatChanges:  loadStatChanges(m.ID),
		}
	}
	return out
}

func chooseAIMoves(c aiCandidate, spec aiSpec, level int) []GeneratedMove {
	_ = spec
	// База — проверенный legacy-набор: 4 урон-хода из всех методов обучения.
	base := generateSmartMoves(c.id, level, c.stabTypes())
	out := make([]GeneratedMove, 0, 4)
	seen := map[int]bool{}
	for _, m := range base {
		if seen[m.ID] {
			continue
		}
		seen[m.ID] = true
		out = append(out, m)
		if len(out) == 4 {
			break
		}
	}
	if len(out) == 0 {
		for _, m := range c.moves {
			if m.CategoryID == 3 || m.Power <= 0 {
				continue
			}
			out = append(out, m)
			if len(out) == 4 {
				break
			}
		}
	}
	return out
}

func aiStatusMoveScore(m GeneratedMove, role string) float64 {
	score := 0.0
	for _, tag := range aiMoveTags[strings.ToLower(strings.TrimSpace(m.EngName))] {
		switch tag {
		case "hazard_set":
			score += 10
		case "screen_set":
			score += 8
		case "recovery":
			if role == "bulky" {
				score += 9
			} else {
				score += 5
			}
		case "setup":
			if role == "physical" || role == "special" || role == "fast" {
				score += 9
			} else {
				score += 4
			}
		case "setup_speed":
			score += 4
		case "setup_defense":
			score += 3
		case "status":
			score += 8
		case "pivot":
			if role != "bulky" {
				score += 7
			} else {
				score += 3
			}
		case "phaze":
			score += 6
		case "protect":
			score += 3
		case "rest":
			score -= 2
		case "self_destruct":
			score -= 20
		case "recharge":
			score -= 5
		case "weather_set:rain", "weather_set:sun", "weather_set:hail", "weather_set:sand":
			// v1: погодные механики в отдельной задаче.
		}
	}
	if rechargeMoves[m.ID] {
		score -= 5
	}
	if m.HealPercent > 0 {
		score += float64(m.HealPercent) / 10.0
	}
	return score
}

func aiMovesPhysical(c aiCandidate, moves []GeneratedMove) bool {
	phys, spec := 0, 0
	for _, m := range moves {
		if m.CategoryID == 1 {
			phys += m.Power
		} else if m.CategoryID == 2 {
			spec += m.Power
		}
	}
	if phys == spec {
		return c.baseAtk >= c.baseSpAtk
	}
	return phys > spec
}

type aiNatureRow struct {
	ID  int
	Inc string
	Dec string
}

func loadAINatures() []aiNatureRow {
	rows, err := db.DB.Query(`
		SELECT id, COALESCE(increased_stat, ''), COALESCE(decreased_stat, '')
		FROM natures ORDER BY id`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []aiNatureRow
	for rows.Next() {
		var n aiNatureRow
		if rows.Scan(&n.ID, &n.Inc, &n.Dec) == nil {
			out = append(out, n)
		}
	}
	return out
}

func chooseAINature(c aiCandidate, physical bool) int {
	rows := loadAINatures()
	inc, dec := "", ""
	if c.role == "bulky" {
		if c.baseDef >= c.baseSpDef {
			inc = "defense"
		} else {
			inc = "special_defense"
		}
		if physical {
			dec = "special_attack"
		} else {
			dec = "attack"
		}
	} else if c.baseSpd >= 80 {
		inc = "speed"
		if physical {
			dec = "special_attack"
		} else {
			dec = "attack"
		}
	} else if physical {
		inc, dec = "attack", "special_attack"
	} else {
		inc, dec = "special_attack", "attack"
	}
	for _, n := range rows {
		if n.Inc == inc && n.Dec == dec {
			return n.ID
		}
	}
	for _, n := range rows {
		if n.Inc == inc && n.Dec != "" {
			return n.ID
		}
	}
	for _, n := range rows {
		if n.Inc == "" && n.Dec == "" {
			return n.ID
		}
	}
	return 13
}

func allocateAIEVs(role string, physical bool, level int, spec aiSpec) [6]int {
	var ev [6]int
	budget := spec.EVPerLevel * level
	if budget < 0 {
		budget = 0
	}
	type evWeight struct {
		idx int
		w   float64
	}
	var ws []evWeight
	switch role {
	case "physical":
		ws = []evWeight{{1, 0.4}, {3, 0.4}, {0, 0.2}}
	case "special":
		ws = []evWeight{{4, 0.4}, {3, 0.4}, {0, 0.2}}
	case "fast":
		if physical {
			ws = []evWeight{{3, 0.4}, {1, 0.4}, {0, 0.2}}
		} else {
			ws = []evWeight{{3, 0.4}, {4, 0.4}, {0, 0.2}}
		}
	case "bulky":
		ws = []evWeight{{0, 0.4}, {2, 0.3}, {5, 0.3}}
	default:
		ws = []evWeight{{0, 0.25}, {1, 0.15}, {2, 0.15}, {3, 0.15}, {4, 0.15}, {5, 0.15}}
	}
	for _, x := range ws {
		v := int(float64(budget) * x.w)
		if v > 126 {
			v = 126
		}
		ev[x.idx] = v
	}
	used := 0
	for _, v := range ev {
		used += v
	}
	rem := budget - used
	for rem > 0 {
		progressed := false
		for _, x := range ws {
			if rem <= 0 {
				break
			}
			if ev[x.idx] >= 126 {
				continue
			}
			add := rem
			if add > 126-ev[x.idx] {
				add = 126 - ev[x.idx]
			}
			ev[x.idx] += add
			rem -= add
			progressed = true
		}
		if !progressed {
			break
		}
	}
	return ev
}

func chooseAIAbility(c aiCandidate, role string) aiAbility {
	best := aiAbility{}
	bestScore := -1.0
	for _, a := range c.abilities {
		sc := 0.0
		if implementedAbilityKeys[a.Key] {
			sc += 1.0
		}
		for _, tag := range a.Tags {
			switch {
			case tag == "offense":
				if role == "physical" || role == "special" || role == "fast" {
					sc += 2.0
				}
			case tag == "defense":
				if role == "bulky" {
					sc += 2.0
				}
			case tag == "speed":
				sc += 1.5
			case tag == "physical_control":
				sc += 2.0
			case tag == "contact_punish":
				sc += 1.0
			case tag == "pinch":
				sc += 1.5
			case tag == "status_immunity":
				sc += 1.0
			case strings.HasPrefix(tag, "immune:"):
				sc += 1.5
			case strings.HasPrefix(tag, "weather_set:"):
				sc += 0.5
			case strings.HasPrefix(tag, "weather_benefit:"):
				sc += 0.3
			case tag == "setup":
				sc += 1.0
			}
		}
		if sc > bestScore {
			bestScore = sc
			best = a
		}
	}
	// Если ни одна способность вида не реализована — берём первую легальную
	// (косметически «способность есть», в бою она просто не имеет хука).
	if best.ID == 0 || !implementedAbilityKeys[best.Key] {
		if len(c.abilities) > 0 {
			return c.abilities[0]
		}
		return aiAbility{}
	}
	return best
}

func chooseAIHeldItem(c aiCandidate, moves []GeneratedMove) (id, typeID int, mult float64, name, image, desc string) {
	mult = 1.0
	powerByType := map[int]float64{}
	for _, m := range moves {
		if m.CategoryID != 3 && m.Power > 0 {
			powerByType[m.TypeID] += float64(m.Power)
		}
	}
	bestType := 0
	bestPower := -1.0
	// Предмет усиливает покемона, только если его тип совпадает с типом
	// покемона (calculateDamage: pokemonHasType), поэтому fallback на «тип
	// самой сильной атаки» недопустим.
	for _, t := range c.stabTypes() {
		if p := powerByType[t]; p > bestPower {
			bestPower, bestType = p, t
		}
	}
	if bestType == 0 {
		return
	}
	var value int
	err := db.DB.QueryRow(`
		SELECT id, name, COALESCE(image_url, ''), COALESCE(description, ''),
		       COALESCE(effect_value, 0)
		FROM items
		WHERE effect_type = 'type_boost' AND type_id = ?
		ORDER BY effect_value DESC, id LIMIT 1`, bestType).
		Scan(&id, &name, &image, &desc, &value)
	if err != nil {
		return
	}
	typeID = bestType
	if value > 0 {
		mult = float64(value) / 100.0
	}
	return
}
