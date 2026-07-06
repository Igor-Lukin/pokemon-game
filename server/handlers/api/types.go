package api

import (
	"encoding/json"
	"net/http"
	"pokemon-game/server/db"
)

type TypeInfo struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func GetAllTypes(w http.ResponseWriter, r *http.Request) {
	rows, err := db.DB.Query("SELECT id, name FROM types ORDER BY id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var types []TypeInfo
	for rows.Next() {
		var t TypeInfo
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			continue
		}
		types = append(types, t)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(types)
}
