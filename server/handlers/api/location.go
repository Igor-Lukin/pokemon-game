package api

import (
	"encoding/json"
	"net/http"
	"pokemon-game/server/db"
	"strconv"

	"github.com/gorilla/mux"
)

type Location struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	HasNursery  bool   `json:"has_nursery"`
	HasHealing  bool   `json:"has_healing"`
	Exits       []Exit `json:"exits"`
	NPCs        []NPC  `json:"npcs"`
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
	var hasNursery, hasHealing int
	err = db.DB.QueryRow("SELECT id, name, description, has_nursery, has_healing FROM locations WHERE id = ?", id).
		Scan(&loc.ID, &loc.Name, &loc.Description, &hasNursery, &hasHealing)
	if err != nil {
		http.Error(w, "Location not found", http.StatusNotFound)
		return
	}
	loc.HasNursery = hasNursery == 1
	loc.HasHealing = hasHealing == 1

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
