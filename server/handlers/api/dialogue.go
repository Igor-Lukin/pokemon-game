package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"pokemon-game/server/db"
	"strconv"

	"github.com/gorilla/mux"
)

// DialogueOption — кнопка-ответ, отдаваемая клиенту (только видимые поля).
type DialogueOption struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
}

// DialogueNodeResponse — стартовый узел NPC (ответ GET /api/npc/{id}/dialogue).
type DialogueNodeResponse struct {
	NpcID   int              `json:"npc_id"`
	NpcName string           `json:"npc_name"`
	NodeID  int              `json:"node_id"`
	Text    string           `json:"text"`
	Options []DialogueOption `json:"options"`
}

// DialogueOptionResult — результат выбора опции (ответ POST /api/npc/dialogue/option).
// kind: "node" | "battle" | "close".
type DialogueOptionResult struct {
	Kind    string           `json:"kind"`
	NpcID   int              `json:"npc_id,omitempty"`   // для kind=="battle" (заглушка для потока B)
	NodeID  int              `json:"node_id,omitempty"`  // для kind=="node"
	Text    string           `json:"text,omitempty"`     // для kind=="node"
	Options []DialogueOption `json:"options,omitempty"`  // для kind=="node"
}

// loadNodeOptions собирает видимые опции узла (condition_flag пока игнорируем — все видимы).
func loadNodeOptions(nodeID int) []DialogueOption {
	opts := []DialogueOption{}
	rows, err := db.DB.Query(
		"SELECT id, option_text FROM dialogue_options WHERE node_id = ? ORDER BY sort_order, id", nodeID)
	if err != nil {
		return opts
	}
	defer rows.Close()
	for rows.Next() {
		var o DialogueOption
		if err := rows.Scan(&o.ID, &o.Text); err == nil {
			opts = append(opts, o)
		}
	}
	return opts
}

// NpcDialogueStartHandler — GET /api/npc/{id}/dialogue: отдаёт стартовый узел NPC.
func NpcDialogueStartHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "Invalid npc id", http.StatusBadRequest)
		return
	}

	resp := DialogueNodeResponse{NpcID: id, Options: []DialogueOption{}}

	// Имя NPC (если NPC нет — 404).
	if err := db.DB.QueryRow("SELECT name FROM npc WHERE id = ?", id).Scan(&resp.NpcName); err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "NPC not found", http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Стартовый узел.
	var nodeID int
	var text string
	err = db.DB.QueryRow(
		"SELECT id, speaker_text FROM dialogue_nodes WHERE npc_id = ? AND is_start = 1 ORDER BY id LIMIT 1", id).
		Scan(&nodeID, &text)
	if err == sql.ErrNoRows {
		// Нет диалога — клиент покажет «…» и одну кнопку «Закрыть».
		resp.NodeID = 0
		resp.Text = "…"
		writeJSON(w, resp)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp.NodeID = nodeID
	resp.Text = text
	resp.Options = loadNodeOptions(nodeID)
	writeJSON(w, resp)
}

// NpcDialogueOptionHandler — POST /api/npc/dialogue/option {option_id}: разрешение выбора.
func NpcDialogueOptionHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OptionID int `json:"option_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.OptionID == 0 {
		http.Error(w, "Invalid option_id", http.StatusBadRequest)
		return
	}

	// Опция + npc_id через её узел.
	var (
		nextNode   sql.NullInt64
		actionType sql.NullString
		npcID      int
	)
	err := db.DB.QueryRow(`
		SELECT o.next_node_id, o.action_type, n.npc_id
		FROM dialogue_options o
		JOIN dialogue_nodes n ON o.node_id = n.id
		WHERE o.id = ?`, req.OptionID).Scan(&nextNode, &actionType, &npcID)
	if err == sql.ErrNoRows {
		http.Error(w, "Option not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Приоритет разрешения (§3): battle -> node -> close.
	if actionType.Valid && actionType.String == "battle" {
		writeJSON(w, DialogueOptionResult{Kind: "battle", NpcID: npcID})
		return
	}
	if nextNode.Valid {
		var text string
		if err := db.DB.QueryRow("SELECT speaker_text FROM dialogue_nodes WHERE id = ?", nextNode.Int64).Scan(&text); err != nil {
			// Битая ссылка на узел — трактуем как завершение.
			writeJSON(w, DialogueOptionResult{Kind: "close"})
			return
		}
		writeJSON(w, DialogueOptionResult{
			Kind:    "node",
			NodeID:  int(nextNode.Int64),
			Text:    text,
			Options: loadNodeOptions(int(nextNode.Int64)),
		})
		return
	}
	writeJSON(w, DialogueOptionResult{Kind: "close"})
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
