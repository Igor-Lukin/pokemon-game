package api

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"

	"pokemon-game/server/db"
	"pokemon-game/server/utils"
)

// ai_team_store.go — сохранение/загрузка AI-команд в npc_ai_teams.
// Храним полный снимок бойца (вид, ходы, способность, характер, IV/EV, предмет),
// чтобы бой воспроизводился точно и не зависел от будущих правок лёрнсетов.

type aiTeamMember struct {
	PokemonID  int    `json:"pokemon_id"`
	Level      int    `json:"level"`
	MoveIDs    []int  `json:"move_ids"`
	AbilityID  int    `json:"ability_id,omitempty"`
	NatureID   int    `json:"nature_id"`
	IV         [6]int `json:"iv"`
	EV         [6]int `json:"ev"`
	HeldItemID int    `json:"held_item_id,omitempty"`
	Gender     string `json:"gender,omitempty"`
}

type aiTeamJSON struct {
	Members []aiTeamMember `json:"members"`
}

func battleTeamToJSON(team []BattlePokemon) (string, error) {
	payload := aiTeamJSON{Members: make([]aiTeamMember, 0, len(team))}
	for i := range team {
		p := &team[i]
		moves := make([]int, 0, len(p.Moves))
		for _, m := range p.Moves {
			moves = append(moves, m.ID)
		}
		payload.Members = append(payload.Members, aiTeamMember{
			PokemonID:  p.PokemonID,
			Level:      p.Level,
			MoveIDs:    moves,
			AbilityID:  p.AbilityID,
			NatureID:   p.NatureID,
			IV:         p.IV,
			EV:         p.EV,
			HeldItemID: p.HeldItemID,
			Gender:     p.Gender,
		})
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// saveAITeam сохраняет команду в пул. source: manual | auto | draft.
// Авто-команды держим в топ-5 по score, ручные не автоматически не удаляем.
func saveAITeam(npcID, aiLevel, level int, team []BattlePokemon, score float64, source string) error {
	if source == "" {
		source = "draft"
	}
	raw, err := battleTeamToJSON(team)
	if err != nil {
		return err
	}
	_, err = db.DB.Exec(`
		INSERT INTO npc_ai_teams (npc_id, ai_level, level, team_json, score, source)
		VALUES (?, ?, ?, ?, ?, ?)`,
		npcID, aiLevel, level, raw, score, source)
	if err != nil {
		return err
	}
	pruneAutoAITeams(npcID, aiLevel, level)
	return nil
}

func pruneAutoAITeams(npcID, aiLevel, level int) {
	_, err := db.DB.Exec(`
		DELETE FROM npc_ai_teams
		WHERE npc_id = ? AND ai_level = ? AND level = ? AND source = 'auto'
		  AND id NOT IN (
			SELECT id FROM npc_ai_teams
			WHERE npc_id = ? AND ai_level = ? AND level = ? AND source = 'auto'
			ORDER BY score DESC, id DESC LIMIT 5
		  )`,
		npcID, aiLevel, level, npcID, aiLevel, level)
	if err != nil {
		log.Printf("[ai-team] prune auto: %v", err)
	}
}

// loadSavedAITeam берёт случайную сохранённую команду из пула.
func loadSavedAITeam(npcID, aiLevel, level int) ([]BattlePokemon, bool) {
	var raw string
	err := db.DB.QueryRow(`
		SELECT team_json FROM npc_ai_teams
		WHERE npc_id = ? AND ai_level = ? AND level = ?
		ORDER BY RANDOM() LIMIT 1`, npcID, aiLevel, level).Scan(&raw)
	if err != nil {
		return nil, false
	}
	var payload aiTeamJSON
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		log.Printf("[ai-team] битый team_json npc=%d L%d: %v", npcID, aiLevel, err)
		return nil, false
	}
	if len(payload.Members) == 0 {
		return nil, false
	}
	team := make([]BattlePokemon, 0, len(payload.Members))
	for _, m := range payload.Members {
		p, err := buildPokemonFromAIMember(m)
		if err != nil {
			log.Printf("[ai-team] сохранённая команда npc=%d L%d: %v", npcID, aiLevel, err)
			return nil, false
		}
		team = append(team, *p)
	}
	alive := 0
	for i := range team {
		if !team[i].IsFainted && team[i].CurrentHP > 0 {
			alive++
		}
	}
	if alive == 0 {
		return nil, false
	}
	log.Printf("[ai-team] npc=%d L%d: загружена сохранённая команда (%d)", npcID, aiLevel, len(team))
	return team, true
}

func buildPokemonFromAIMember(m aiTeamMember) (*BattlePokemon, error) {
	species, err := loadAISpeciesData(m.PokemonID)
	if err != nil {
		return nil, err
	}
	if m.Level < 1 {
		m.Level = 1
	}
	moves, err := loadBattleMovesByIDs(m.MoveIDs)
	if err != nil {
		return nil, err
	}
	if len(moves) == 0 {
		return nil, fmt.Errorf("у вида #%d нет сохранённых ходов", m.PokemonID)
	}
	natureID := m.NatureID
	if natureID == 0 {
		natureID = 13
	}
	natureFactors, err := utils.GetNatureFactors(natureID)
	if err != nil {
		natureFactors = [6]float64{1, 1, 1, 1, 1, 1}
	}
	train := [6]float64{1, 1, 1, 1, 1, 1}
	curHP, maxHP, atk, def, spd, spAtk, spDef := utils.CalculateStats(
		species.Base, m.IV, m.EV, m.Level, natureFactors, train)

	gender := m.Gender
	if gender != db.GenderMale && gender != db.GenderFemale {
		if g, err := db.RollGender(m.PokemonID); err == nil {
			gender = g
		} else {
			gender = db.GenderNeutral
		}
	}

	bp := &BattlePokemon{
		PokemonID:       m.PokemonID,
		Name:            species.Name,
		Level:           m.Level,
		CurrentHP:       curHP,
		MaxHP:           maxHP,
		Attack:          atk,
		Defense:         def,
		Speed:           spd,
		SpecialAttack:   spAtk,
		SpecialDefense:  spDef,
		PrimaryTypeID:   species.PrimaryType,
		SecondaryTypeID: species.Secondary,
		ImageURL:        species.ImageURL,
		Weight:          species.Weight,
		Gender:          gender,
		Moves:           moves,
		IsFainted:       false,
		HeldAtkMult:     1.0,
		NatureID:        natureID,
		IV:              m.IV,
		EV:              m.EV,
	}
	if m.AbilityID != 0 {
		bp.AbilityID = m.AbilityID
		bp.AbilityName, bp.AbilityKey = db.AbilityNameKey(m.AbilityID)
	}
	if m.HeldItemID != 0 {
		var effectType, name, image, desc string
		var effectValue, typeID int
		err := db.DB.QueryRow(`
			SELECT COALESCE(effect_type, ''), COALESCE(effect_value, 0),
			       COALESCE(type_id, 0), name, COALESCE(image_url, ''),
			       COALESCE(description, '')
			FROM items WHERE id = ?`, m.HeldItemID).
			Scan(&effectType, &effectValue, &typeID, &name, &image, &desc)
		if err == nil {
			bp.HeldItemID = m.HeldItemID
			bp.HeldItemName = name
			bp.HeldItemImage = image
			bp.HeldItemDescription = desc
			bp.HeldItemEffect = effectType
			if effectType == "type_boost" && effectValue > 0 {
				bp.HeldItemTypeID = typeID
				bp.HeldAtkMult = float64(effectValue) / 100.0
			}
		}
	}
	snapshotBaseStats(bp)
	return bp, nil
}

func loadBattleMovesByIDs(ids []int) ([]BattleMove, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	byID := map[int]BattleMove{}
	for _, chunk := range chunkInts(ids, 400) {
		args := make([]interface{}, 0, len(chunk))
		for _, id := range chunk {
			args = append(args, id)
		}
		q := fmt.Sprintf(`
			SELECT a.id, a.name, COALESCE(a.eng_name, ''), a.type_id, a.category_id,
			       a.power, a.accuracy, a.pp, a.priority,
			       COALESCE(a.target_id, 1), COALESCE(a.description, ''),
			       COALESCE(a.effect_status_id, 0), COALESCE(a.effect_chance, 0),
			       COALESCE(a.never_misses, 0), COALESCE(a.drain, 0),
			       COALESCE(a.crit_rate, 0), COALESCE(a.hits_min, 1),
			       COALESCE(a.hits_max, 1), COALESCE(a.disabled, 0),
			       COALESCE(a.heal_percent, 0), COALESCE(a.contact, 0),
			       COALESCE(a.sound, 0)
			FROM attacks a WHERE a.id IN (%s)`, placeholders(len(chunk)))
		rows, err := db.DB.Query(q, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var m BattleMove
			var neverMisses, disabled, contact, sound int
			if err := rows.Scan(&m.ID, &m.Name, &m.EngName, &m.TypeID, &m.CategoryID,
				&m.Power, &m.Accuracy, &m.PP, &m.Priority, &m.TargetID, &m.Description,
				&m.EffectID, &m.EffectChance, &neverMisses, &m.Drain, &m.CritRate,
				&m.HitsMin, &m.HitsMax, &disabled, &m.HealPercent, &contact, &sound); err != nil {
				continue
			}
			m.NeverMisses = neverMisses != 0
			m.Disabled = disabled != 0
			m.Contact = contact != 0
			m.Sound = sound != 0
			m.PP, m.MaxPP = -1, -1
			m.StatChanges = loadStatChanges(m.ID)
			byID[m.ID] = m
		}
		rows.Close()
	}
	out := make([]BattleMove, 0, len(ids))
	seen := map[int]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		if m, ok := byID[id]; ok {
			out = append(out, m)
			seen[id] = true
		}
	}
	return out, nil
}

// AITeamMeta — краткая информация для админки.
type AITeamMeta struct {
	ID        int
	AILevel   int
	Level     int
	Score     float64
	Source    string
	CreatedAt string
	Size      int
	Names     string
}

func listSavedAITeams(npcID int) []AITeamMeta {
	rows, err := db.DB.Query(`
		SELECT id, ai_level, level, COALESCE(score, 0), COALESCE(source, ''),
		       COALESCE(created_at, ''), team_json
		FROM npc_ai_teams WHERE npc_id = ?
		ORDER BY ai_level, level, score DESC, id DESC`, npcID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []AITeamMeta
	for rows.Next() {
		var meta AITeamMeta
		var raw string
		if rows.Scan(&meta.ID, &meta.AILevel, &meta.Level, &meta.Score,
			&meta.Source, &meta.CreatedAt, &raw) != nil {
			continue
		}
		var payload aiTeamJSON
		if json.Unmarshal([]byte(raw), &payload) == nil {
			meta.Size = len(payload.Members)
			names := make([]string, 0, len(payload.Members))
			for _, m := range payload.Members {
				var name string
				db.DB.QueryRow("SELECT name FROM pokemons WHERE id = ?", m.PokemonID).Scan(&name)
				if name != "" {
					names = append(names, name+" ур."+strconv.Itoa(m.Level))
				}
			}
			meta.Names = strings.Join(names, ", ")
		}
		out = append(out, meta)
	}
	return out
}

func deleteSavedAITeam(id int) error {
	_, err := db.DB.Exec("DELETE FROM npc_ai_teams WHERE id = ?", id)
	return err
}

// ListSavedAITeamsForNPC — публичная обёртка для админки.
func ListSavedAITeamsForNPC(npcID int) []AITeamMeta {
	return listSavedAITeams(npcID)
}

// DeleteSavedAITeam — публичная обёртка для админки.
func DeleteSavedAITeam(id int) error {
	return deleteSavedAITeam(id)
}

// AITeamPreviewMember — строка предпросмотра AI-команды в админке.
type AITeamPreviewMember struct {
	PokemonID int
	Name      string
	Level     int
	Ability   string
	Nature    string
	Item      string
	Moves     []string
	HP        int
	Atk       int
	Def       int
	Spd       int
	SpAtk     int
	SpDef     int
	Legendary bool
}

// AITeamPreview — сгенерированная команда + JSON для сохранения.
type AITeamPreview struct {
	NPCID    int
	NPCName  string
	AILevel  int
	Level    int
	TeamJSON string
	Members  []AITeamPreviewMember
}

// GenerateAITeamForNPC — сгенерировать AI-команду по настройкам NPC для
// предпросмотра/ручного сохранения из админки.
func GenerateAITeamForNPC(npcID int) (*AITeamPreview, error) {
	var name string
	var aiLevel, size, level int
	err := db.DB.QueryRow(`
		SELECT name, COALESCE(ai_level, 0), COALESCE(team_size, 6), COALESCE(min_level, 5)
		FROM npc WHERE id = ?`, npcID).Scan(&name, &aiLevel, &size, &level)
	if err != nil {
		return nil, fmt.Errorf("NPC не найден")
	}
	if aiLevel < 1 {
		return nil, fmt.Errorf("сначала выставьте AI-уровень 1 или 2")
	}
	team, err := buildAITeam(npcID, aiLevel, size, level)
	if err != nil {
		return nil, err
	}
	raw, err := battleTeamToJSON(team)
	if err != nil {
		return nil, err
	}
	preview := &AITeamPreview{
		NPCID: npcID, NPCName: name, AILevel: aiLevel, Level: level, TeamJSON: raw,
	}
	for i := range team {
		p := &team[i]
		moves := make([]string, len(p.Moves))
		for j := range p.Moves {
			moves[j] = p.Moves[j].Name
		}
		var legendary int
		db.DB.QueryRow("SELECT COALESCE(is_legendary,0) FROM pokemons WHERE id = ?", p.PokemonID).Scan(&legendary)
		preview.Members = append(preview.Members, AITeamPreviewMember{
			PokemonID: p.PokemonID, Name: p.Name, Level: p.Level,
			Ability: p.AbilityName, Nature: natureNameByID(p.NatureID),
			Item: p.HeldItemName, Moves: moves,
			HP: p.MaxHP, Atk: p.Attack, Def: p.Defense, Spd: p.Speed,
			SpAtk: p.SpecialAttack, SpDef: p.SpecialDefense,
			Legendary: legendary != 0,
		})
	}
	return preview, nil
}

// SaveAITeamForNPC — сохранить предпросмотр в пул npc_ai_teams (source=manual).
func SaveAITeamForNPC(npcID int, teamJSON string) error {
	var aiLevel, level int
	err := db.DB.QueryRow(`
		SELECT COALESCE(ai_level, 0), COALESCE(min_level, 5)
		FROM npc WHERE id = ?`, npcID).Scan(&aiLevel, &level)
	if err != nil {
		return fmt.Errorf("NPC не найден")
	}
	if aiLevel < 1 {
		return fmt.Errorf("сначала выставьте AI-уровень 1 или 2")
	}
	var payload aiTeamJSON
	if err := json.Unmarshal([]byte(teamJSON), &payload); err != nil || len(payload.Members) == 0 {
		return fmt.Errorf("битая AI-команда")
	}
	team := make([]BattlePokemon, 0, len(payload.Members))
	for _, m := range payload.Members {
		p, err := buildPokemonFromAIMember(m)
		if err != nil {
			return err
		}
		team = append(team, *p)
	}
	return saveAITeam(npcID, aiLevel, level, team, 0, "manual")
}

func natureNameByID(id int) string {
	var name string
	if err := db.DB.QueryRow("SELECT name FROM natures WHERE id = ?", id).Scan(&name); err != nil {
		return strconv.Itoa(id)
	}
	return name
}

// savedTeamByID — для предпросмотра/повторного сохранения.
func savedTeamByID(id int) ([]BattlePokemon, bool) {
	var raw string
	if err := db.DB.QueryRow("SELECT team_json FROM npc_ai_teams WHERE id = ?", id).Scan(&raw); err != nil {
		return nil, false
	}
	var payload aiTeamJSON
	if json.Unmarshal([]byte(raw), &payload) != nil || len(payload.Members) == 0 {
		return nil, false
	}
	team := make([]BattlePokemon, 0, len(payload.Members))
	for _, m := range payload.Members {
		p, err := buildPokemonFromAIMember(m)
		if err != nil {
			return nil, false
		}
		team = append(team, *p)
	}
	return team, true
}
