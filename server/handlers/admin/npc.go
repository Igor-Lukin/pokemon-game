package admin

import (
	"database/sql"
	"log"
	"net/http"
	"pokemon-game/server/db"
	"strconv"
	"text/template"
)

type Location struct {
	ID   int
	Name string
}

type NPC struct {
	ID           int
	Name         string
	LocationName string
}

// список НПС
func NonPlayerCharacterHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// JOIN с таблицей locations для получения имени локации
	rows, err := db.DB.Query(`
		SELECT npc.id, npc.name, locations.name 
		FROM npc 
		JOIN locations ON npc.location_id = locations.id 
		ORDER BY npc.id
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var nonPlayerCharacters []NPC

	for rows.Next() {
		var npc NPC
		if err := rows.Scan(&npc.ID, &npc.Name, &npc.LocationName); err != nil {
			continue
		}
		nonPlayerCharacters = append(nonPlayerCharacters, npc)
	}

	tmpl := template.Must(template.ParseFiles("./server/templates/admin/npc.html"))
	tmpl.Execute(w, nonPlayerCharacters)

}

// создание нового НПС (GET - форма, POST - сохранение)
func CreateNonPlayerCharacterHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if r.Method == http.MethodGet {

		// Получаем список всех локаций для выпадающего списка добавления выхода
		allRows, err := db.DB.Query("SELECT id, name FROM locations ORDER BY id")
		if err != nil {
			log.Printf("Error loading locations: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer allRows.Close()

		var allLocations []Location
		for allRows.Next() {
			var loc Location
			if err := allRows.Scan(&loc.ID, &loc.Name); err == nil {
				allLocations = append(allLocations, loc)
			}
		}

		data := struct {
			ID           int
			Name         string
			AllLocations []Location
		}{ID: 0, Name: "", AllLocations: allLocations}

		tmpl, err := template.ParseFiles("./server/templates/admin/npc_form.html")
		if err != nil {
			log.Printf("Template parse error: %v", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		err = tmpl.Execute(w, data)
		if err != nil {
			log.Printf("Template execute error: %v", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
		}
		return
	}

	if r.Method == http.MethodPost {
		name := r.FormValue("name")
		locationId := r.FormValue("location_id")
		if name == "" {
			http.Error(w, "Name is required", http.StatusBadRequest)
			return
		}

		_, err := db.DB.Exec("INSERT INTO npc (name, location_id) VALUES (?, ?)", name, locationId)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/admin/npc", http.StatusSeeOther)

	}
}

// удаление НПС
func DeleteNonPlayerCharacterHandler(w http.ResponseWriter, r *http.Request) {
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

	_, err = db.DB.Exec("DELETE FROM npc WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/npc", http.StatusSeeOther)
}

func EditNonPlayerCharacterHandler(w http.ResponseWriter, r *http.Request) {
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
		var name string
		var location_id int

		err := db.DB.QueryRow("SELECT name, location_id FROM npc WHERE id = ?", id).Scan(&name, &location_id)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "NPC not found", http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		// Получаем список всех локаций для выпадающего списка добавления выхода
		allRows, err := db.DB.Query("SELECT id, name FROM locations ORDER BY id")
		if err != nil {
			log.Printf("Error loading locations: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer allRows.Close()

		var allLocations []Location
		for allRows.Next() {
			var loc Location
			if err := allRows.Scan(&loc.ID, &loc.Name); err == nil {
				allLocations = append(allLocations, loc)
			}
		}

		data := struct {
			ID           int
			Name         string
			LocationID   int
			AllLocations []Location
		}{ID: id, Name: name, LocationID: location_id, AllLocations: allLocations}

		tmpl, err := template.ParseFiles("./server/templates/admin/npc_form.html")
		if err != nil {
			log.Printf("Template parse error: %v", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		err = tmpl.Execute(w, data)
		if err != nil {
			log.Printf("Template execute error: %v", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
		}
		return
	}

	if r.Method == http.MethodPost {
		name := r.FormValue("name")
		locationId := r.FormValue("location_id")
		if name == "" {
			http.Error(w, "Name is required", http.StatusBadRequest)
			return
		}

		_, err := db.DB.Exec("UPDATE npc SET name = ?, location_id = ? WHERE id = ?", name, locationId, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/admin/npc", http.StatusSeeOther)
	}
}
