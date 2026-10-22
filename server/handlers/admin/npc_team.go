package admin

import (
	"net/http"
	"pokemon-game/server/db"
	"strconv"
)

// Редактор фикс-ростера команды NPC (поток B). Все хендлеры POST, возвращают на
// страницу редактирования NPC. checkAdmin — как везде в admin.

// AddNPCPokemonHandler — POST /admin/npc/team/add.
func AddNPCPokemonHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	npcIDStr := r.FormValue("npc_id")
	npcID, err := strconv.Atoi(npcIDStr)
	if err != nil {
		http.Error(w, "Invalid npc_id", http.StatusBadRequest)
		return
	}
	pokemonID, err := strconv.Atoi(r.FormValue("pokemon_id"))
	if err != nil || pokemonID <= 0 {
		http.Error(w, "Invalid pokemon_id", http.StatusBadRequest)
		return
	}
	// Вид должен существовать.
	var exists int
	db.DB.QueryRow("SELECT COUNT(*) FROM pokemons WHERE id = ?", pokemonID).Scan(&exists)
	if exists == 0 {
		http.Error(w, "Нет такого вида покемона", http.StatusBadRequest)
		return
	}
	level := atoiDefault(r.FormValue("level"), 5)
	if level < 1 {
		level = 1
	}
	position := atoiDefault(r.FormValue("position"), 0)

	_, err = db.DB.Exec(
		"INSERT INTO npc_pokemons (npc_id, pokemon_id, level, position) VALUES (?, ?, ?, ?)",
		npcID, pokemonID, level, position)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	npcEditRedirect(w, r, npcIDStr)
}

// EditNPCPokemonHandler — POST /admin/npc/team/edit.
func EditNPCPokemonHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	npcIDStr := r.FormValue("npc_id")
	id, err := strconv.Atoi(r.FormValue("id"))
	if err != nil {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}
	pokemonID, err := strconv.Atoi(r.FormValue("pokemon_id"))
	if err != nil || pokemonID <= 0 {
		http.Error(w, "Invalid pokemon_id", http.StatusBadRequest)
		return
	}
	var exists int
	db.DB.QueryRow("SELECT COUNT(*) FROM pokemons WHERE id = ?", pokemonID).Scan(&exists)
	if exists == 0 {
		http.Error(w, "Нет такого вида покемона", http.StatusBadRequest)
		return
	}
	level := atoiDefault(r.FormValue("level"), 5)
	if level < 1 {
		level = 1
	}
	position := atoiDefault(r.FormValue("position"), 0)

	_, err = db.DB.Exec(
		"UPDATE npc_pokemons SET pokemon_id = ?, level = ?, position = ? WHERE id = ?",
		pokemonID, level, position, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	npcEditRedirect(w, r, npcIDStr)
}

// DeleteNPCPokemonHandler — POST /admin/npc/team/delete.
func DeleteNPCPokemonHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	npcIDStr := r.FormValue("npc_id")
	id, err := strconv.Atoi(r.FormValue("id"))
	if err != nil {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}
	db.DB.Exec("DELETE FROM npc_pokemons WHERE id = ?", id)
	npcEditRedirect(w, r, npcIDStr)
}
