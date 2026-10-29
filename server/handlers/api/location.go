package api

import (
	"encoding/json"
	"net/http"
	"pokemon-game/server/db"
	"strconv"

	"github.com/gorilla/mux"
)

type Location struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	RegionID    int     `json:"region_id"`
	RegionName  string  `json:"region_name"`
	HasNursery  bool    `json:"has_nursery"`
	HasHealing  bool    `json:"has_healing"`
	HasWild     bool    `json:"has_wild"`
	Exits       []Exit  `json:"exits"`
	NPCs        []NPC   `json:"npcs"`
	Spawns      []Spawn `json:"spawns"`
}

// Spawn — вид, который водится на локации (для блока «Здесь водятся»).
type Spawn struct {
	PokemonID int    `json:"pokemon_id"`
	Name      string `json:"name"`
	ImageURL  string `json:"image_url"`
	MinLevel  int    `json:"min_level"`
	MaxLevel  int    `json:"max_level"`
	Rarity    string `json:"rarity"`
}

type Exit struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type NPC struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	HasDialogue bool   `json:"has_dialogue"` // есть стартовый узел диалога (клиент подсветит/кликабельно)
}

func GetLocation(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid location id", http.StatusBadRequest)
		return
	}

	var loc Location
	var hasNursery, hasHealing, hasWild int
	err = db.DB.QueryRow(`
		SELECT l.id, l.name, l.description, l.has_nursery, l.has_healing,
		       EXISTS(SELECT 1 FROM pokemon_locations pl WHERE pl.location_id = l.id),
		       COALESCE(l.region_id, 0), COALESCE(r.name, '')
		FROM locations l
		LEFT JOIN regions r ON r.id = l.region_id
		WHERE l.id = ?`, id).
		Scan(&loc.ID, &loc.Name, &loc.Description, &hasNursery, &hasHealing, &hasWild,
			&loc.RegionID, &loc.RegionName)
	if err != nil {
		http.Error(w, "Location not found", http.StatusNotFound)
		return
	}
	loc.HasNursery = hasNursery == 1
	loc.HasHealing = hasHealing == 1
	loc.HasWild = hasWild == 1

	// Виды, которые водятся на локации (блок «Здесь водятся»).
	rowsSpawn, err := db.DB.Query(`
		SELECT pl.pokemon_id, p.name, COALESCE(p.image_url, ''),
		       COALESCE(pl.min_level, 1), COALESCE(pl.max_level, 1),
		       COALESCE(pl.rarity, '')
		FROM pokemon_locations pl
		JOIN pokemons p ON p.id = pl.pokemon_id
		WHERE pl.location_id = ?
		ORDER BY pl.min_level, p.id`, id)
	if err == nil {
		defer rowsSpawn.Close()
		for rowsSpawn.Next() {
			var s Spawn
			if err := rowsSpawn.Scan(&s.PokemonID, &s.Name, &s.ImageURL, &s.MinLevel, &s.MaxLevel, &s.Rarity); err != nil {
				continue
			}
			loc.Spawns = append(loc.Spawns, s)
		}
	}

	// Выходы
	rows, err := db.DB.Query("SELECT to_location, (SELECT name FROM locations WHERE id = to_location) FROM location_exits WHERE from_location = ?", id)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var e Exit
			rows.Scan(&e.ID, &e.Name)
			loc.Exits = append(loc.Exits, e)
		}
	}

	// NPC (+признак наличия стартового узла диалога).
	rowsNPC, err := db.DB.Query(`
		SELECT n.id, n.name,
		       EXISTS(SELECT 1 FROM dialogue_nodes d WHERE d.npc_id = n.id AND d.is_start = 1)
		FROM npc n WHERE n.location_id = ?`, id)
	if err == nil {
		defer rowsNPC.Close()
		for rowsNPC.Next() {
			var npc NPC
			var hasDialogue int
			rowsNPC.Scan(&npc.ID, &npc.Name, &hasDialogue)
			npc.HasDialogue = hasDialogue == 1
			loc.NPCs = append(loc.NPCs, npc)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(loc)
}
