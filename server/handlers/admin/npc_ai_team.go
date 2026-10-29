package admin

import (
	htmltemplate "html/template"
	"net/http"
	"strconv"

	"pokemon-game/server/handlers/api"
)

// npc_ai_team.go — предпросмотр/ручное сохранение AI-команды NPC.

// GenerateNPCAITeamHandler — POST /admin/npc/ai-team/generate.
func GenerateNPCAITeamHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	npcID, err := strconv.Atoi(r.FormValue("npc_id"))
	if err != nil {
		http.Error(w, "Invalid npc_id", http.StatusBadRequest)
		return
	}
	preview, err := api.GenerateAITeamForNPC(npcID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	tmpl, err := htmltemplate.ParseFiles("./server/templates/admin/npc_ai_team.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmpl.Execute(w, preview); err != nil {
		http.Error(w, "Template execute error: "+err.Error(), http.StatusInternalServerError)
	}
}

// SaveNPCAITeamHandler — POST /admin/npc/ai-team/save.
func SaveNPCAITeamHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	npcID, err := strconv.Atoi(r.FormValue("npc_id"))
	if err != nil {
		http.Error(w, "Invalid npc_id", http.StatusBadRequest)
		return
	}
	if err := api.SaveAITeamForNPC(npcID, r.FormValue("team_json")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/admin/npc/edit?id="+strconv.Itoa(npcID), http.StatusSeeOther)
}

// DeleteNPCAITeamHandler — POST /admin/npc/ai-team/delete.
func DeleteNPCAITeamHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.Atoi(r.FormValue("id"))
	if err != nil {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}
	npcID := atoiDefault(r.FormValue("npc_id"), 0)
	if err := api.DeleteSavedAITeam(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if npcID > 0 {
		http.Redirect(w, r, "/admin/npc/edit?id="+strconv.Itoa(npcID), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/admin/npc", http.StatusSeeOther)
}
