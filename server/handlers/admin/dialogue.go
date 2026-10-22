package admin

import (
	"net/http"
	"pokemon-game/server/db"
	"strconv"
)

// Редактор диалогов NPC (поток A). Все хендлеры POST, возвращают на страницу
// редактирования NPC: /admin/npc/edit?id=<npc_id>. checkAdmin — как везде в admin.

// npcEditRedirect — общий возврат на страницу NPC после правки диалога.
func npcEditRedirect(w http.ResponseWriter, r *http.Request, npcID string) {
	http.Redirect(w, r, "/admin/npc/edit?id="+npcID, http.StatusSeeOther)
}

// clearStartFlag снимает is_start у всех узлов NPC (гвард «ровно один стартовый»).
func clearStartFlag(npcID int) {
	db.DB.Exec("UPDATE dialogue_nodes SET is_start = 0 WHERE npc_id = ?", npcID)
}

// AddDialogueNodeHandler — POST /admin/npc/dialogue/node/add.
func AddDialogueNodeHandler(w http.ResponseWriter, r *http.Request) {
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
	speaker := r.FormValue("speaker_text")
	if speaker == "" {
		http.Error(w, "speaker_text is required", http.StatusBadRequest)
		return
	}
	nodeKey := r.FormValue("node_key")
	isStart := r.FormValue("is_start") != ""

	if isStart {
		clearStartFlag(npcID)
	}
	startVal := 0
	if isStart {
		startVal = 1
	}
	_, err = db.DB.Exec(
		"INSERT INTO dialogue_nodes (npc_id, node_key, speaker_text, is_start) VALUES (?, ?, ?, ?)",
		npcID, nodeKey, speaker, startVal)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	npcEditRedirect(w, r, npcIDStr)
}

// EditDialogueNodeHandler — POST /admin/npc/dialogue/node/edit.
func EditDialogueNodeHandler(w http.ResponseWriter, r *http.Request) {
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
	nodeID, err := strconv.Atoi(r.FormValue("id"))
	if err != nil {
		http.Error(w, "Invalid node id", http.StatusBadRequest)
		return
	}
	speaker := r.FormValue("speaker_text")
	if speaker == "" {
		http.Error(w, "speaker_text is required", http.StatusBadRequest)
		return
	}
	nodeKey := r.FormValue("node_key")
	isStart := r.FormValue("is_start") != ""

	if isStart {
		clearStartFlag(npcID)
	}
	startVal := 0
	if isStart {
		startVal = 1
	}
	_, err = db.DB.Exec(
		"UPDATE dialogue_nodes SET node_key = ?, speaker_text = ?, is_start = ? WHERE id = ? AND npc_id = ?",
		nodeKey, speaker, startVal, nodeID, npcID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	npcEditRedirect(w, r, npcIDStr)
}

// DeleteDialogueNodeHandler — POST /admin/npc/dialogue/node/delete.
// Чистит опции узла и обнуляет ссылки next_node_id на него (целостность).
func DeleteDialogueNodeHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	npcIDStr := r.FormValue("npc_id")
	nodeID, err := strconv.Atoi(r.FormValue("id"))
	if err != nil {
		http.Error(w, "Invalid node id", http.StatusBadRequest)
		return
	}
	db.DB.Exec("DELETE FROM dialogue_options WHERE node_id = ?", nodeID)         // опции самого узла
	db.DB.Exec("UPDATE dialogue_options SET next_node_id = NULL WHERE next_node_id = ?", nodeID) // висячие ссылки
	if _, err := db.DB.Exec("DELETE FROM dialogue_nodes WHERE id = ?", nodeID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	npcEditRedirect(w, r, npcIDStr)
}

// nextNodeParam разбирает next_node_id и валидирует, что узел принадлежит тому же NPC.
// Возвращает (значение-или-nil, ok). ok=false -> ссылка на чужой/несуществующий узел.
func nextNodeParam(raw string, npcID int) (interface{}, bool) {
	if raw == "" {
		return nil, true // пусто = конец после действия
	}
	nid, err := strconv.Atoi(raw)
	if err != nil {
		return nil, false
	}
	var owner int
	if err := db.DB.QueryRow("SELECT npc_id FROM dialogue_nodes WHERE id = ?", nid).Scan(&owner); err != nil {
		return nil, false
	}
	if owner != npcID {
		return nil, false
	}
	return nid, true
}

// actionParam нормализует action_type: "" -> NULL, иначе 'battle'/'close'.
func actionParam(raw string) interface{} {
	if raw == "" {
		return nil
	}
	return raw
}

// AddDialogueOptionHandler — POST /admin/npc/dialogue/option/add.
func AddDialogueOptionHandler(w http.ResponseWriter, r *http.Request) {
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
	nodeID, err := strconv.Atoi(r.FormValue("node_id"))
	if err != nil {
		http.Error(w, "Invalid node_id", http.StatusBadRequest)
		return
	}
	text := r.FormValue("option_text")
	if text == "" {
		http.Error(w, "option_text is required", http.StatusBadRequest)
		return
	}
	nextNode, ok := nextNodeParam(r.FormValue("next_node_id"), npcID)
	if !ok {
		http.Error(w, "next_node_id must reference a node of the same NPC", http.StatusBadRequest)
		return
	}
	sortOrder, _ := strconv.Atoi(r.FormValue("sort_order"))
	_, err = db.DB.Exec(
		"INSERT INTO dialogue_options (node_id, sort_order, option_text, next_node_id, action_type) VALUES (?, ?, ?, ?, ?)",
		nodeID, sortOrder, text, nextNode, actionParam(r.FormValue("action_type")))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	npcEditRedirect(w, r, npcIDStr)
}

// EditDialogueOptionHandler — POST /admin/npc/dialogue/option/edit.
func EditDialogueOptionHandler(w http.ResponseWriter, r *http.Request) {
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
	optID, err := strconv.Atoi(r.FormValue("id"))
	if err != nil {
		http.Error(w, "Invalid option id", http.StatusBadRequest)
		return
	}
	text := r.FormValue("option_text")
	if text == "" {
		http.Error(w, "option_text is required", http.StatusBadRequest)
		return
	}
	nextNode, ok := nextNodeParam(r.FormValue("next_node_id"), npcID)
	if !ok {
		http.Error(w, "next_node_id must reference a node of the same NPC", http.StatusBadRequest)
		return
	}
	sortOrder, _ := strconv.Atoi(r.FormValue("sort_order"))
	_, err = db.DB.Exec(
		"UPDATE dialogue_options SET sort_order = ?, option_text = ?, next_node_id = ?, action_type = ? WHERE id = ?",
		sortOrder, text, nextNode, actionParam(r.FormValue("action_type")), optID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	npcEditRedirect(w, r, npcIDStr)
}

// DeleteDialogueOptionHandler — POST /admin/npc/dialogue/option/delete.
func DeleteDialogueOptionHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	npcIDStr := r.FormValue("npc_id")
	optID, err := strconv.Atoi(r.FormValue("id"))
	if err != nil {
		http.Error(w, "Invalid option id", http.StatusBadRequest)
		return
	}
	if _, err := db.DB.Exec("DELETE FROM dialogue_options WHERE id = ?", optID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	npcEditRedirect(w, r, npcIDStr)
}
