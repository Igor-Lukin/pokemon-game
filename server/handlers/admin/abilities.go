package admin

import (
	"database/sql"
	"net/http"
	"pokemon-game/server/db"
	"strconv"
	"text/template"
)

type Ability struct {
	ID          int
	Name        string
	Description string
	EffectKey   string // машиночит. ключ эффекта для диспетчера боя (может быть пуст)
}

// Список способностей
func AbilitiesHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	rows, err := db.DB.Query("SELECT id, name, description, COALESCE(effect_key, '') FROM abilities ORDER BY id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var abilities []Ability
	for rows.Next() {
		var a Ability
		if err := rows.Scan(&a.ID, &a.Name, &a.Description, &a.EffectKey); err != nil {
			continue
		}
		abilities = append(abilities, a)
	}
	tmpl := template.Must(template.ParseFiles("./server/templates/admin/abilities.html"))
	tmpl.Execute(w, abilities)
}

// Создание способности
func CreateAbilityHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodGet {
		tmpl := template.Must(template.ParseFiles("./server/templates/admin/ability_form.html"))
		tmpl.Execute(w, nil)
		return
	}
	if r.Method == http.MethodPost {
		name := r.FormValue("name")
		description := r.FormValue("description")
		effectKey := r.FormValue("effect_key")
		if name == "" {
			http.Error(w, "Name is required", http.StatusBadRequest)
			return
		}
		_, err := db.DB.Exec("INSERT INTO abilities (name, description, effect_key) VALUES (?, ?, ?)", name, description, effectKey)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/admin/abilities", http.StatusSeeOther)
	}
}

// Редактирование способности
func EditAbilityHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		http.Error(w, "Missing id", http.StatusBadRequest)
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}
	if r.Method == http.MethodGet {
		var name, description, effectKey string
		err := db.DB.QueryRow("SELECT name, description, COALESCE(effect_key, '') FROM abilities WHERE id = ?", id).Scan(&name, &description, &effectKey)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "Ability not found", http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		data := struct {
			ID          int
			Name        string
			Description string
			EffectKey   string
		}{ID: id, Name: name, Description: description, EffectKey: effectKey}
		tmpl := template.Must(template.ParseFiles("./server/templates/admin/ability_form.html"))
		tmpl.Execute(w, data)
		return
	}
	if r.Method == http.MethodPost {
		name := r.FormValue("name")
		description := r.FormValue("description")
		effectKey := r.FormValue("effect_key")
		if name == "" {
			http.Error(w, "Name is required", http.StatusBadRequest)
			return
		}
		_, err := db.DB.Exec("UPDATE abilities SET name = ?, description = ?, effect_key = ? WHERE id = ?", name, description, effectKey, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/admin/abilities", http.StatusSeeOther)
	}
}

// Удаление способности
func DeleteAbilityHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.FormValue("id")
	if idStr == "" {
		http.Error(w, "Missing id", http.StatusBadRequest)
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}
	_, err = db.DB.Exec("DELETE FROM abilities WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/abilities", http.StatusSeeOther)
}
