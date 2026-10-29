package admin

import (
	"database/sql"
	"log"
	"net/http"
	"pokemon-game/server/db"
	"pokemon-game/server/handlers/api"
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

// --- Представления для формы редактирования NPC (команда + диалог) ---

type DialogueOptionView struct {
	ID           int
	SortOrder    int
	OptionText   string
	NextNodeID   int // 0 = нет перехода
	NextNodeText string
	ActionType   string // "" | "battle" | "close"
}

type DialogueNodeView struct {
	ID          int
	NodeKey     string
	SpeakerText string
	IsStart     bool
	Options     []DialogueOptionView
}

// NPCFormData — единые данные для npc_form.html (Create и Edit).
type NPCFormData struct {
	ID           int
	Name         string
	LocationID   int
	AllLocations []Location
	// поля команды для боя (§3.1, использует поток B)
	TeamMode string
	TeamSize int
	MinLevel int
	MaxLevel int
	AILevel  int // 0 = старый AI, 1/2 = бот, 3/4 = задел
	// диалог
	Nodes []DialogueNodeView
	// команда (fixed-ростер, поток B)
	Roster      []RosterRowView
	AllPokemons []PokemonOption
	// пул сохранённых AI-команд
	AITeams []api.AITeamMeta
}

// RosterRowView — строка фикс-ростера команды NPC (npc_pokemons).
type RosterRowView struct {
	ID         int
	PokemonID  int
	PokemonName string
	Level      int
	Position   int
}

// PokemonOption — вид покемона для выпадающего списка ростера.
type PokemonOption struct {
	ID   int
	Name string
}

// loadNPCRoster — строки фикс-ростера NPC (для админ-редактора команды).
func loadNPCRoster(npcID int) []RosterRowView {
	rows, err := db.DB.Query(`
		SELECT np.id, np.pokemon_id, COALESCE(p.name,''), np.level, COALESCE(np.position,0)
		FROM npc_pokemons np
		LEFT JOIN pokemons p ON p.id = np.pokemon_id
		WHERE np.npc_id = ? ORDER BY np.position, np.id`, npcID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []RosterRowView
	for rows.Next() {
		var rrow RosterRowView
		if rows.Scan(&rrow.ID, &rrow.PokemonID, &rrow.PokemonName, &rrow.Level, &rrow.Position) == nil {
			out = append(out, rrow)
		}
	}
	return out
}

// loadAllPokemons — список видов для select ростера.
func loadAllPokemons() []PokemonOption {
	rows, err := db.DB.Query("SELECT id, name FROM pokemons ORDER BY id")
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []PokemonOption
	for rows.Next() {
		var o PokemonOption
		if rows.Scan(&o.ID, &o.Name) == nil {
			out = append(out, o)
		}
	}
	return out
}

// loadNPCDialogue собирает узлы NPC с их опциями (для админ-редактора).
func loadNPCDialogue(npcID int) []DialogueNodeView {
	nodes := []DialogueNodeView{}
	rows, err := db.DB.Query(
		"SELECT id, COALESCE(node_key,''), speaker_text, is_start FROM dialogue_nodes WHERE npc_id = ? ORDER BY is_start DESC, id", npcID)
	if err != nil {
		return nodes
	}
	defer rows.Close()
	for rows.Next() {
		var n DialogueNodeView
		var isStart int
		if err := rows.Scan(&n.ID, &n.NodeKey, &n.SpeakerText, &isStart); err != nil {
			continue
		}
		n.IsStart = isStart == 1
		nodes = append(nodes, n)
	}
	// Опции каждого узла (+текст целевого узла для наглядности).
	for i := range nodes {
		orows, err := db.DB.Query(`
			SELECT o.id, o.sort_order, o.option_text,
			       COALESCE(o.next_node_id, 0), COALESCE(t.speaker_text, ''), COALESCE(o.action_type, '')
			FROM dialogue_options o
			LEFT JOIN dialogue_nodes t ON o.next_node_id = t.id
			WHERE o.node_id = ? ORDER BY o.sort_order, o.id`, nodes[i].ID)
		if err != nil {
			continue
		}
		for orows.Next() {
			var o DialogueOptionView
			if err := orows.Scan(&o.ID, &o.SortOrder, &o.OptionText, &o.NextNodeID, &o.NextNodeText, &o.ActionType); err == nil {
				nodes[i].Options = append(nodes[i].Options, o)
			}
		}
		orows.Close()
	}
	return nodes
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

		data := NPCFormData{
			ID: 0, Name: "", AllLocations: allLocations,
			TeamMode: "smart", TeamSize: 6, MinLevel: 5, MaxLevel: 5, AILevel: 0,
		}

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

	// Каскад вручную: опции всех узлов NPC, узлы, ростер команды (поток B), сам NPC.
	db.DB.Exec("DELETE FROM dialogue_options WHERE node_id IN (SELECT id FROM dialogue_nodes WHERE npc_id = ?)", id)
	db.DB.Exec("DELETE FROM dialogue_nodes WHERE npc_id = ?", id)
	db.DB.Exec("DELETE FROM npc_pokemons WHERE npc_id = ?", id)

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
		var teamMode string
		var teamSize, minLevel, maxLevel, aiLevel int

		err := db.DB.QueryRow(
			"SELECT name, location_id, COALESCE(team_mode,'random'), COALESCE(team_size,6), COALESCE(min_level,5), COALESCE(max_level,10), COALESCE(ai_level,0) FROM npc WHERE id = ?", id).
			Scan(&name, &location_id, &teamMode, &teamSize, &minLevel, &maxLevel, &aiLevel)
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

		data := NPCFormData{
			ID: id, Name: name, LocationID: location_id, AllLocations: allLocations,
			TeamMode: teamMode, TeamSize: teamSize, MinLevel: minLevel, MaxLevel: maxLevel,
			AILevel:     aiLevel,
			Nodes:       loadNPCDialogue(id),
			Roster:      loadNPCRoster(id),
			AllPokemons: loadAllPokemons(),
			AITeams:     api.ListSavedAITeamsForNPC(id),
		}

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

		// Поля команды для боя. Режимы random/fixed убраны — тренер ВСЕГДА smart
		// (умный драфт, поток [12]). Уровень фиксированный: поле «Уровень» -> min_level,
		// max_level = min_level.
		teamMode := "smart"
		teamSize := atoiDefault(r.FormValue("team_size"), 6)
		if teamSize < 1 {
			teamSize = 1
		} else if teamSize > 6 {
			teamSize = 6
		}
		minLevel := atoiDefault(r.FormValue("min_level"), 5)
		if minLevel < 1 {
			minLevel = 1
		}
		maxLevel := minLevel
		aiLevel := atoiDefault(r.FormValue("ai_level"), 0)
		if aiLevel < 0 {
			aiLevel = 0
		} else if aiLevel > 4 {
			aiLevel = 4
		}

		_, err := db.DB.Exec(
			"UPDATE npc SET name = ?, location_id = ?, team_mode = ?, team_size = ?, min_level = ?, max_level = ?, ai_level = ? WHERE id = ?",
			name, locationId, teamMode, teamSize, minLevel, maxLevel, aiLevel, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// После сохранения остаёмся на странице редактирования (диалог правится тут же).
		http.Redirect(w, r, "/admin/npc/edit?id="+idStr, http.StatusSeeOther)
	}
}

// atoiDefault — целое из строки формы с дефолтом при пустой/битой строке.
func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}
