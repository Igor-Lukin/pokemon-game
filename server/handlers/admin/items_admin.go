package admin

import (
	"html/template"
	"net/http"
	"pokemon-game/server/db"
	"strconv"
)

// ====================== КАТЕГОРИИ ПРЕДМЕТОВ ======================

func ItemsCategoriesHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	cats, err := db.GetAllCategories()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl := template.Must(template.ParseFiles("./server/templates/admin/item_categories.html"))
	tmpl.Execute(w, cats)
}

func CreateItemCategoryHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodGet {
		tmpl := template.Must(template.ParseFiles("./server/templates/admin/item_category_form.html"))
		tmpl.Execute(w, nil)
		return
	}
	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}
	if err := db.CreateCategory(name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/items/categories", http.StatusSeeOther)
}

func EditItemCategoryHandler(w http.ResponseWriter, r *http.Request) {
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
		cats, err := db.GetAllCategories()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var cat *db.Category
		for i, c := range cats {
			if c.ID == id {
				cat = &cats[i]
				break
			}
		}
		if cat == nil {
			http.Error(w, "Category not found", http.StatusNotFound)
			return
		}
		tmpl := template.Must(template.ParseFiles("./server/templates/admin/item_category_form.html"))
		tmpl.Execute(w, cat)
		return
	}

	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}
	if err := db.UpdateCategory(id, name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/items/categories", http.StatusSeeOther)
}

func DeleteItemCategoryHandler(w http.ResponseWriter, r *http.Request) {
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
	if err := db.DeleteCategory(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/items/categories", http.StatusSeeOther)
}

// ====================== ПРЕДМЕТЫ ======================

type attackOption struct {
	ID   int
	Name string
}

func getAttackOptions() ([]attackOption, error) {
	rows, err := db.DB.Query("SELECT id, name FROM attacks WHERE COALESCE(disabled, 0) = 0 ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []attackOption
	for rows.Next() {
		var a attackOption
		if err := rows.Scan(&a.ID, &a.Name); err != nil {
			continue
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

func ItemsListHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	items, err := db.GetAllItems()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl := template.Must(template.ParseFiles("./server/templates/admin/items.html"))
	tmpl.Execute(w, items)
}

func CreateItemHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodGet {
		cats, err := db.GetAllCategories()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		types, err := db.GetAllTypes()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		attacks, err := getAttackOptions()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tmpl := template.Must(template.ParseFiles("./server/templates/admin/item_form.html"))
		tmpl.Execute(w, struct {
			Categories []db.Category
			Types      []db.Category
			Attacks    []attackOption
			Item       *db.Item
		}{cats, types, attacks, nil})
		return
	}

	name := r.FormValue("name")
	description := r.FormValue("description")
	categoryID, _ := strconv.Atoi(r.FormValue("category_id"))
	price, _ := strconv.Atoi(r.FormValue("price"))
	effectType := r.FormValue("effect_type")
	effectValue, _ := strconv.Atoi(r.FormValue("effect_value"))
	imageURL := r.FormValue("image_url")
	typeID, _ := strconv.Atoi(r.FormValue("type_id"))
	attackID, _ := strconv.Atoi(r.FormValue("attack_id"))

	if name == "" || categoryID == 0 {
		http.Error(w, "Name and category are required", http.StatusBadRequest)
		return
	}
	if err := db.CreateItem(name, description, categoryID, price, effectType, effectValue, imageURL, typeID, attackID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/items", http.StatusSeeOther)
}

func EditItemHandler(w http.ResponseWriter, r *http.Request) {
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
		item, err := db.GetItemByID(id)
		if err != nil {
			http.Error(w, "Item not found", http.StatusNotFound)
			return
		}
		cats, err := db.GetAllCategories()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		types, err := db.GetAllTypes()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		attacks, err := getAttackOptions()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tmpl := template.Must(template.ParseFiles("./server/templates/admin/item_form.html"))
		tmpl.Execute(w, struct {
			Categories []db.Category
			Types      []db.Category
			Attacks    []attackOption
			Item       *db.Item
		}{cats, types, attacks, item})
		return
	}

	name := r.FormValue("name")
	description := r.FormValue("description")
	categoryID, _ := strconv.Atoi(r.FormValue("category_id"))
	price, _ := strconv.Atoi(r.FormValue("price"))
	effectType := r.FormValue("effect_type")
	effectValue, _ := strconv.Atoi(r.FormValue("effect_value"))
	imageURL := r.FormValue("image_url")
	typeID, _ := strconv.Atoi(r.FormValue("type_id"))
	attackID, _ := strconv.Atoi(r.FormValue("attack_id"))

	if name == "" || categoryID == 0 {
		http.Error(w, "Name and category are required", http.StatusBadRequest)
		return
	}
	if err := db.UpdateItem(id, name, description, categoryID, price, effectType, effectValue, imageURL, typeID, attackID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/items", http.StatusSeeOther)
}

func DeleteItemHandler(w http.ResponseWriter, r *http.Request) {
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
	if err := db.DeleteItem(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/items", http.StatusSeeOther)
}

// ====================== ВЫДАЧА ПРЕДМЕТА ======================

func GiveItemHandler(w http.ResponseWriter, r *http.Request) {
	if !checkAdmin(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodGet {
		usersRows, err := db.DB.Query("SELECT id, username FROM users ORDER BY id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer usersRows.Close()
		type User struct {
			ID   int
			Name string
		}
		var users []User
		for usersRows.Next() {
			var u User
			if err := usersRows.Scan(&u.ID, &u.Name); err == nil {
				users = append(users, u)
			}
		}

		items, err := db.GetAllItems()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		data := struct {
			Users []User
			Items []db.Item
		}{users, items}
		tmpl := template.Must(template.ParseFiles("./server/templates/admin/give_item.html"))
		tmpl.Execute(w, data)
		return
	}

	if r.Method == http.MethodPost {
		userID, _ := strconv.Atoi(r.FormValue("user_id"))
		itemID, _ := strconv.Atoi(r.FormValue("item_id"))
		quantity, _ := strconv.Atoi(r.FormValue("quantity"))
		if userID == 0 || itemID == 0 || quantity <= 0 {
			http.Error(w, "Invalid parameters", http.StatusBadRequest)
			return
		}
		if err := db.AddItemToUser(userID, itemID, quantity); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/admin/give_item", http.StatusSeeOther)
	}
}
