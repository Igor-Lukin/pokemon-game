package admin

import (
	"database/sql"
	"log"
	"net/http"
	"pokemon-game/server/db"
	"strconv"
	"strings"
	"text/template"
)

// Структуры для справочников
type AttackType struct {
	ID   int
	Name string
}
type AttackCategory struct {
	ID   int
	Name string
}
type AttackTarget struct {
	ID   int
	Name string
}
type EffectStatus struct {
	ID   int
	Name string
}

// Структура атаки для отображения в списке и формах
type Attack struct {
	ID             int
	Name           string
	EngName        string
	TypeID         int
	TypeName       string
	Power          sql.NullInt64
	Accuracy       sql.NullInt64
	PP             sql.NullInt64
	CategoryID     int
	CategoryName   string
	TargetID       int
	TargetName     string
	Priority       int
	Description    string
	Contact        bool
	Sound          bool
	EffectStatusID sql.NullInt64
	EffectChance   int
	Drain          int
	CritRate       int
	HitsMin        int
	HitsMax        int
	NeverMisses  bool
	Disabled     bool
}

type StatChangeRow struct {
	StatID int
	Stages int
	Target int
	Chance int
}

func loadStatChangesForAttack(attackID int) []StatChangeRow {
	rows, err := db.DB.Query(`
		SELECT stat_id, stages, target, chance
		FROM attack_stat_changes
		WHERE attack_id = ?
	`, attackID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var scs []StatChangeRow
	for rows.Next() {
		var sc StatChangeRow
		if rows.Scan(&sc.StatID, &sc.Stages, &sc.Target, &sc.Chance) == nil {
			scs = append(scs, sc)
		}
	}
	return scs
}

func saveStatChangesForAttack(attackID int, scs []StatChangeRow) {
	db.DB.Exec("DELETE FROM attack_stat_changes WHERE attack_id = ?", attackID)
	for _, sc := range scs {
		db.DB.Exec(`
			INSERT INTO attack_stat_changes (attack_id, stat_id, stages, target, chance)
			VALUES (?, ?, ?, ?, ?)
		`, attackID, sc.StatID, sc.Stages, sc.Target, sc.Chance)
	}
}

// Список атак
func AttacksHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	query := `
        SELECT a.id, a.name, a.eng_name, a.type_id, t.name, a.power, a.accuracy, a.pp,
               a.category_id, c.name, a.target_id, tg.name, a.priority, a.description,
               a.contact, a.sound, a.effect_status_id, a.effect_chance, COALESCE(a.drain, 0),
               COALESCE(a.never_misses, 0), COALESCE(a.crit_rate, 0),
               COALESCE(a.hits_min, 1), COALESCE(a.hits_max, 1),
               COALESCE(a.disabled, 0)
         FROM attacks a
         JOIN types t ON a.type_id = t.id
         JOIN attack_categories c ON a.category_id = c.id
         JOIN attack_targets tg ON a.target_id = tg.id
         ORDER BY a.id
    `
	rows, err := db.DB.Query(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	defer rows.Close()

	var attacks []Attack

	for rows.Next() {
		var atk Attack
		if err := rows.Scan(&atk.ID, &atk.Name, &atk.EngName, &atk.TypeID, &atk.TypeName,
			&atk.Power, &atk.Accuracy, &atk.PP, &atk.CategoryID, &atk.CategoryName,
			&atk.TargetID, &atk.TargetName, &atk.Priority, &atk.Description,
			&atk.Contact, &atk.Sound, &atk.EffectStatusID, &atk.EffectChance, &atk.Drain, &atk.NeverMisses,
			&atk.CritRate, &atk.HitsMin, &atk.HitsMax, &atk.Disabled); err != nil {
			log.Println("Scan error:", err)
			continue
		}
		attacks = append(attacks, atk)
	}
	tmpl := template.Must(template.ParseFiles("./server/templates/admin/attacks.html"))
	log.Printf("Attacks count: %d", len(attacks))
	tmpl.Execute(w, attacks)
}

// Создание атаки
func CreateAttackHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodGet {
		types := loadAttackTypes()
		categories := loadAttackCategories()
		targets := loadAttackTargets()
		statuses := loadEffectStatuses()
		data := struct {
			Attack       Attack
			Types        []AttackType
			Categories   []AttackCategory
			Targets      []AttackTarget
			Statuses     []EffectStatus
			StatChanges  []StatChangeRow
		}{Attack{HitsMin: 1, HitsMax: 1}, types, categories, targets, statuses, nil}
		tmpl := template.Must(template.ParseFiles("./server/templates/admin/attack_form.html"))
		tmpl.Execute(w, data)
		return
	}
	if r.Method == http.MethodPost {
		name := r.FormValue("name")
		engName := r.FormValue("eng_name")
		typeID := r.FormValue("type_id")
		power := r.FormValue("power")
		accuracy := r.FormValue("accuracy")
		pp := r.FormValue("pp")
		categoryID := r.FormValue("category_id")
		targetID := r.FormValue("target_id")
		priority := r.FormValue("priority")
		description := r.FormValue("description")
		contact := r.FormValue("contact") == "on"
		sound := r.FormValue("sound") == "on"
		effectStatusID := r.FormValue("effect_status_id")
		effectChance := r.FormValue("effect_chance")

		if name == "" || typeID == "" || categoryID == "" || targetID == "" {
			http.Error(w, "Required fields missing", http.StatusBadRequest)
			return
		}

		var powerVal, accuracyVal, ppVal, priorityVal interface{}
		if power != "" {
			powerVal, _ = strconv.Atoi(power)
		} else {
			powerVal = nil
		}
		if accuracy != "" {
			accuracyVal, _ = strconv.Atoi(accuracy)
		} else {
			accuracyVal = nil
		}
		if pp != "" {
			ppVal, _ = strconv.Atoi(pp)
		} else {
			ppVal = nil
		}
		if priority != "" {
			priorityVal, _ = strconv.Atoi(priority)
		} else {
			priorityVal = 0
		}
		typeIDInt, _ := strconv.Atoi(typeID)
		categoryIDInt, _ := strconv.Atoi(categoryID)
		targetIDInt, _ := strconv.Atoi(targetID)
		effectChanceInt, _ := strconv.Atoi(effectChance)
		var effectStatusIDVal interface{}
		if effectStatusID != "" {
			id, _ := strconv.Atoi(effectStatusID)
			effectStatusIDVal = id
		} else {
			effectStatusIDVal = nil
		}

		drain, _ := strconv.Atoi(r.FormValue("drain"))
		neverMisses := r.FormValue("never_misses") == "on"
		critRate, _ := strconv.Atoi(r.FormValue("crit_rate"))
		hitsMin, _ := strconv.Atoi(r.FormValue("hits_min"))
		hitsMax, _ := strconv.Atoi(r.FormValue("hits_max"))

		res, err := db.DB.Exec(`
            INSERT INTO attacks (
                name, eng_name, type_id, power, accuracy, pp, category_id, target_id,
                priority, description, contact, sound, effect_status_id, effect_chance,
                drain, never_misses, crit_rate, hits_min, hits_max, disabled
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			name, engName, typeIDInt, powerVal, accuracyVal, ppVal, categoryIDInt, targetIDInt,
			priorityVal, description, contact, sound, effectStatusIDVal, effectChanceInt, drain, neverMisses,
			critRate, hitsMin, hitsMax, r.FormValue("disabled") == "on")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		attackID, _ := res.LastInsertId()

		// Сохраняем изменения статов
		scs := parseStatChanges(r)
		saveStatChangesForAttack(int(attackID), scs)

		http.Redirect(w, r, "/admin/attacks", http.StatusSeeOther)
	}
}

// Редактирование атаки
func EditAttackHandler(w http.ResponseWriter, r *http.Request) {
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
		var atk Attack
		var power, accuracy, pp sql.NullInt64
		err := db.DB.QueryRow(`
            SELECT id, name, eng_name, type_id, power, accuracy, pp, category_id, target_id,
                   priority, description, contact, sound, effect_status_id, effect_chance,
                   COALESCE(drain, 0), COALESCE(never_misses, 0),
                   COALESCE(crit_rate, 0), COALESCE(hits_min, 1), COALESCE(hits_max, 1),
                   COALESCE(disabled, 0)
            FROM attacks WHERE id = ?`, id).Scan(
			&atk.ID, &atk.Name, &atk.EngName, &atk.TypeID, &power, &accuracy, &pp,
			&atk.CategoryID, &atk.TargetID, &atk.Priority, &atk.Description,
			&atk.Contact, &atk.Sound, &atk.EffectStatusID, &atk.EffectChance, &atk.Drain, &atk.NeverMisses,
			&atk.CritRate, &atk.HitsMin, &atk.HitsMax, &atk.Disabled)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "Attack not found", http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		atk.Power = power
		atk.Accuracy = accuracy
		atk.PP = pp

		types := loadAttackTypes()
		categories := loadAttackCategories()
		targets := loadAttackTargets()
		statuses := loadEffectStatuses()
		statChanges := loadStatChangesForAttack(id)

		data := struct {
			Attack       Attack
			Types        []AttackType
			Categories   []AttackCategory
			Targets      []AttackTarget
			Statuses     []EffectStatus
			StatChanges  []StatChangeRow
		}{atk, types, categories, targets, statuses, statChanges}
		tmpl := template.Must(template.ParseFiles("./server/templates/admin/attack_form.html"))
		tmpl.Execute(w, data)
		return
	}
	if r.Method == http.MethodPost {
		name := r.FormValue("name")
		engName := r.FormValue("eng_name")
		typeID := r.FormValue("type_id")
		power := r.FormValue("power")
		accuracy := r.FormValue("accuracy")
		pp := r.FormValue("pp")
		categoryID := r.FormValue("category_id")
		targetID := r.FormValue("target_id")
		priority := r.FormValue("priority")
		description := r.FormValue("description")
		contact := r.FormValue("contact") == "on"
		sound := r.FormValue("sound") == "on"
		effectStatusID := r.FormValue("effect_status_id")
		effectChance := r.FormValue("effect_chance")

		if name == "" || typeID == "" || categoryID == "" || targetID == "" {
			http.Error(w, "Required fields missing", http.StatusBadRequest)
			return
		}

		var powerVal, accuracyVal, ppVal, priorityVal interface{}
		if power != "" {
			powerVal, _ = strconv.Atoi(power)
		} else {
			powerVal = nil
		}
		if accuracy != "" {
			accuracyVal, _ = strconv.Atoi(accuracy)
		} else {
			accuracyVal = nil
		}
		if pp != "" {
			ppVal, _ = strconv.Atoi(pp)
		} else {
			ppVal = nil
		}
		if priority != "" {
			priorityVal, _ = strconv.Atoi(priority)
		} else {
			priorityVal = 0
		}
		typeIDInt, _ := strconv.Atoi(typeID)
		categoryIDInt, _ := strconv.Atoi(categoryID)
		targetIDInt, _ := strconv.Atoi(targetID)
		effectChanceInt, _ := strconv.Atoi(effectChance)
		var effectStatusIDVal interface{}
		if effectStatusID != "" {
			id, _ := strconv.Atoi(effectStatusID)
			effectStatusIDVal = id
		} else {
			effectStatusIDVal = nil
		}

		drain, _ := strconv.Atoi(r.FormValue("drain"))
		neverMisses := r.FormValue("never_misses") == "on"
		critRate, _ := strconv.Atoi(r.FormValue("crit_rate"))
		hitsMin, _ := strconv.Atoi(r.FormValue("hits_min"))
		hitsMax, _ := strconv.Atoi(r.FormValue("hits_max"))

		_, err := db.DB.Exec(`
            UPDATE attacks SET
                name=?, eng_name=?, type_id=?, power=?, accuracy=?, pp=?, category_id=?, target_id=?,
                priority=?, description=?, contact=?, sound=?, effect_status_id=?, effect_chance=?,
                drain=?, never_misses=?, crit_rate=?, hits_min=?, hits_max=?, disabled=?
            WHERE id=?`,
			name, engName, typeIDInt, powerVal, accuracyVal, ppVal, categoryIDInt, targetIDInt,
			priorityVal, description, contact, sound, effectStatusIDVal, effectChanceInt, drain, neverMisses,
			critRate, hitsMin, hitsMax, r.FormValue("disabled") == "on", id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Сохраняем изменения статов
		scs := parseStatChanges(r)
		saveStatChangesForAttack(id, scs)

		http.Redirect(w, r, "/admin/attacks", http.StatusSeeOther)
	}
}

// Удаление атаки
func DeleteAttackHandler(w http.ResponseWriter, r *http.Request) {
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
	_, err = db.DB.Exec("DELETE FROM attacks WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/attacks", http.StatusSeeOther)
}

// ========== Вспомогательные функции ==========
func parseStatChanges(r *http.Request) []StatChangeRow {
	statIDs := r.Form["sc_stat_id"]
	stages := r.Form["sc_stages"]
	targets := r.Form["sc_target"]
	chances := r.Form["sc_chance"]
	var scs []StatChangeRow
	for i := 0; i < len(statIDs); i++ {
		sid := strings.TrimSpace(statIDs[i])
		if sid == "" {
			continue
		}
		sidInt, _ := strconv.Atoi(sid)
		stgInt, _ := strconv.Atoi(strings.TrimSpace(stages[i]))
		tgtInt, _ := strconv.Atoi(strings.TrimSpace(targets[i]))
		chInt, _ := strconv.Atoi(strings.TrimSpace(chances[i]))
		scs = append(scs, StatChangeRow{
			StatID: sidInt,
			Stages: stgInt,
			Target: tgtInt,
			Chance: chInt,
		})
	}
	return scs
}

func loadAttackTypes() []AttackType {
	rows, _ := db.DB.Query("SELECT id, name FROM types ORDER BY id")
	defer rows.Close()
	var types []AttackType
	for rows.Next() {
		var t AttackType
		rows.Scan(&t.ID, &t.Name)
		types = append(types, t)
	}
	return types
}

func loadAttackCategories() []AttackCategory {
	rows, _ := db.DB.Query("SELECT id, name FROM attack_categories ORDER BY id")
	defer rows.Close()
	var cats []AttackCategory
	for rows.Next() {
		var c AttackCategory
		rows.Scan(&c.ID, &c.Name)
		cats = append(cats, c)
	}
	return cats
}

func loadAttackTargets() []AttackTarget {
	rows, _ := db.DB.Query("SELECT id, name FROM attack_targets ORDER BY id")
	defer rows.Close()
	var tgts []AttackTarget
	for rows.Next() {
		var t AttackTarget
		rows.Scan(&t.ID, &t.Name)
		tgts = append(tgts, t)
	}
	return tgts
}

func loadEffectStatuses() []EffectStatus {
	rows, _ := db.DB.Query("SELECT id, name FROM statuses ORDER BY id")
	defer rows.Close()
	var stats []EffectStatus
	for rows.Next() {
		var s EffectStatus
		rows.Scan(&s.ID, &s.Name)
		stats = append(stats, s)
	}
	return stats
}
