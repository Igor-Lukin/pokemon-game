package db

import (
	"database/sql"
	"log"
)

type Item struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	CategoryID   int    `json:"category_id"`
	CategoryName string `json:"category_name"`
	Price        int    `json:"price"`
	EffectType   string `json:"effect_type"`
	EffectValue  int    `json:"effect_value"`
	ImageURL     string `json:"image_url"`
	// CatchMultiplier — множитель шанса поимки (x100): обычный=100, Great=150,
	// Ultra=200, Master=очень большой. Для не-болов игнорируется (фаза 1 ловли).
	CatchMultiplier int `json:"catch_multiplier"`
	// Условные болы (фаза 2). CatchCondition: '' (безусловный, множитель всегда),
	// 'type' (значение=type_id), 'speed_gt' (значение=порог скорости),
	// 'level_lt' (значение=порог уровня), 'opposite_gender' (противоположный
	// активному игроку пол). Если условие не выполнено — множитель = ×1.
	CatchCondition string `json:"catch_condition"`
	CatchCondValue int    `json:"catch_cond_value"`
	// Пост-эффекты на пойманного: бонус ко ВСЕМ генам (IV, кап 31), бонус к
	// счастью, принудительно в питомник (Трансбол).
	GeneBonus      int `json:"gene_bonus"`
	HappinessBonus int `json:"happiness_bonus"`
	ForceNursery   int `json:"force_nursery"`
	// TypeID — тип надеваемого (held) предмета для типового гейта. 0 = нет/не типовой.
	TypeID int `json:"type_id"`
}

type UserItem struct {
	Item
	Quantity int `json:"quantity"`
}

type Category struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func GetUserInventory(userID int) ([]UserItem, error) {
	rows, err := DB.Query(`
		SELECT ui.item_id, i.name, COALESCE(i.description, ''), i.category_id,
		       COALESCE(ic.name, ''), i.price,
		       COALESCE(i.effect_type, ''), COALESCE(i.effect_value, 0),
		       COALESCE(i.image_url, ''), COALESCE(i.catch_multiplier, 100),
		       COALESCE(i.catch_condition, ''), COALESCE(i.catch_cond_value, 0),
		       COALESCE(i.gene_bonus, 0), COALESCE(i.happiness_bonus, 0),
		       COALESCE(i.force_nursery, 0), COALESCE(i.type_id, 0), ui.quantity
		FROM user_items ui
		JOIN items i ON ui.item_id = i.id
		LEFT JOIN item_categories ic ON i.category_id = ic.id
		WHERE ui.user_id = ? AND ui.quantity > 0
		  AND COALESCE(i.is_award, 0) = 0
		ORDER BY i.category_id, i.id
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var inv []UserItem
	for rows.Next() {
		var ui UserItem
		if err := rows.Scan(&ui.ID, &ui.Name, &ui.Description, &ui.CategoryID,
			&ui.CategoryName, &ui.Price, &ui.EffectType, &ui.EffectValue,
			&ui.ImageURL, &ui.CatchMultiplier, &ui.CatchCondition, &ui.CatchCondValue,
			&ui.GeneBonus, &ui.HappinessBonus, &ui.ForceNursery, &ui.TypeID, &ui.Quantity); err != nil {
			log.Printf("GetUserInventory scan: %v", err)
			continue
		}
		inv = append(inv, ui)
	}
	if inv == nil {
		inv = []UserItem{}
	}
	return inv, nil
}

func AddItemToUser(userID, itemID, quantity int) error {
	_, err := DB.Exec(`
		INSERT INTO user_items (user_id, item_id, quantity)
		VALUES (?, ?, ?)
		ON CONFLICT(user_id, item_id) DO UPDATE SET quantity = quantity + ?
	`, userID, itemID, quantity, quantity)
	return err
}

func RemoveItemFromUser(userID, itemID, quantity int) (bool, error) {
	tx, err := DB.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var current int
	err = tx.QueryRow("SELECT quantity FROM user_items WHERE user_id = ? AND item_id = ?", userID, itemID).Scan(&current)
	if err != nil {
		return false, err
	}
	if current < quantity {
		return false, nil
	}

	newQty := current - quantity
	if newQty <= 0 {
		_, err = tx.Exec("DELETE FROM user_items WHERE user_id = ? AND item_id = ?", userID, itemID)
	} else {
		_, err = tx.Exec("UPDATE user_items SET quantity = ? WHERE user_id = ? AND item_id = ?", newQty, userID, itemID)
	}
	if err != nil {
		return false, err
	}
	return tx.Commit() == nil, nil
}

func GetItemByID(itemID int) (*Item, error) {
	var it Item
	err := DB.QueryRow(`
		SELECT i.id, i.name, COALESCE(i.description, ''), i.category_id,
		       COALESCE(ic.name, ''), i.price,
		       COALESCE(i.effect_type, ''), COALESCE(i.effect_value, 0),
		       COALESCE(i.image_url, ''), COALESCE(i.catch_multiplier, 100),
		       COALESCE(i.catch_condition, ''), COALESCE(i.catch_cond_value, 0),
		       COALESCE(i.gene_bonus, 0), COALESCE(i.happiness_bonus, 0),
		       COALESCE(i.force_nursery, 0), COALESCE(i.type_id, 0)
		FROM items i
		LEFT JOIN item_categories ic ON i.category_id = ic.id
		WHERE i.id = ?
	`, itemID).Scan(&it.ID, &it.Name, &it.Description, &it.CategoryID,
		&it.CategoryName, &it.Price, &it.EffectType, &it.EffectValue, &it.ImageURL, &it.CatchMultiplier,
		&it.CatchCondition, &it.CatchCondValue, &it.GeneBonus, &it.HappinessBonus, &it.ForceNursery, &it.TypeID)
	if err != nil {
		return nil, err
	}
	return &it, nil
}

func GetAllItems() ([]Item, error) {
	rows, err := DB.Query(`
		SELECT i.id, i.name, COALESCE(i.description, ''), i.category_id,
		       COALESCE(ic.name, ''), i.price,
		       COALESCE(i.effect_type, ''), COALESCE(i.effect_value, 0),
		       COALESCE(i.image_url, '')
		FROM items i
		LEFT JOIN item_categories ic ON i.category_id = ic.id
		ORDER BY i.category_id, i.id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.Name, &it.Description, &it.CategoryID,
			&it.CategoryName, &it.Price, &it.EffectType, &it.EffectValue, &it.ImageURL); err != nil {
			continue
		}
		items = append(items, it)
	}
	return items, nil
}

func GetAllCategories() ([]Category, error) {
	rows, err := DB.Query("SELECT id, name FROM item_categories ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cats []Category
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Name); err != nil {
			continue
		}
		cats = append(cats, c)
	}
	return cats, nil
}

func CreateCategory(name string) error {
	_, err := DB.Exec("INSERT INTO item_categories (name) VALUES (?)", name)
	return err
}

func UpdateCategory(id int, name string) error {
	_, err := DB.Exec("UPDATE item_categories SET name = ? WHERE id = ?", name, id)
	return err
}

func DeleteCategory(id int) error {
	_, err := DB.Exec("DELETE FROM item_categories WHERE id = ?", id)
	return err
}

func CreateItem(name, description string, categoryID, price int, effectType string, effectValue int, imageURL string, typeID int) error {
	var typeVal interface{}
	if typeID > 0 {
		typeVal = typeID
	}
	_, err := DB.Exec(`
		INSERT INTO items (name, description, category_id, price, effect_type, effect_value, image_url, type_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, name, description, categoryID, price, effectType, effectValue, imageURL, typeVal)
	return err
}

func UpdateItem(id int, name, description string, categoryID, price int, effectType string, effectValue int, imageURL string, typeID int) error {
	var typeVal interface{}
	if typeID > 0 {
		typeVal = typeID
	}
	_, err := DB.Exec(`
		UPDATE items SET name=?, description=?, category_id=?, price=?, effect_type=?, effect_value=?, image_url=?, type_id=?
		WHERE id=?
	`, name, description, categoryID, price, effectType, effectValue, imageURL, typeVal, id)
	return err
}

// GetAllTypes — список типов покемонов/предметов (id, name) для форм.
func GetAllTypes() ([]Category, error) {
	rows, err := DB.Query("SELECT id, name FROM types ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var types []Category
	for rows.Next() {
		var t Category
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			continue
		}
		types = append(types, t)
	}
	return types, nil
}

func DeleteItem(id int) error {
	_, err := DB.Exec("DELETE FROM items WHERE id = ?", id)
	return err
}

// CategoryHeld — id категории надеваемых (held) предметов.
const CategoryHeld = 9

// IsHeldItem — можно ли предмет надеть на покемона (категория held или type_boost).
func IsHeldItem(it *Item) bool {
	return it != nil && (it.CategoryID == CategoryHeld || it.EffectType == "type_boost")
}

// GetHeldItemID — id надетого на экземпляр предмета (0 = ничего) с проверкой владения.
func GetHeldItemID(userID, userPokemonID int) (int, error) {
	var held sql.NullInt64
	err := DB.QueryRow(
		"SELECT held_item_id FROM user_pokemons WHERE id = ? AND user_id = ?",
		userPokemonID, userID,
	).Scan(&held)
	if err != nil {
		return 0, err
	}
	if held.Valid {
		return int(held.Int64), nil
	}
	return 0, nil
}

// HoldItem надевает предмет itemID на покемона userPokemonID под транзакцией:
// списывает новый предмет из сумки, возвращает ранее надетый (если был) и
// проставляет held_item_id. Владение проверяется по user_id. При нехватке
// предмета в сумке возвращает false, nil.
func HoldItem(userID, userPokemonID, itemID int) (bool, error) {
	tx, err := DB.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	// Владение + текущий надетый.
	var curHeld sql.NullInt64
	err = tx.QueryRow(
		"SELECT held_item_id FROM user_pokemons WHERE id = ? AND user_id = ?",
		userPokemonID, userID,
	).Scan(&curHeld)
	if err != nil {
		return false, err // sql.ErrNoRows -> не его покемон
	}

	// Новый предмет должен быть в сумке.
	var qty int
	err = tx.QueryRow(
		"SELECT quantity FROM user_items WHERE user_id = ? AND item_id = ?",
		userID, itemID,
	).Scan(&qty)
	if err == sql.ErrNoRows || qty <= 0 {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	// Списать новый предмет (−1).
	if qty-1 <= 0 {
		_, err = tx.Exec("DELETE FROM user_items WHERE user_id = ? AND item_id = ?", userID, itemID)
	} else {
		_, err = tx.Exec("UPDATE user_items SET quantity = quantity - 1 WHERE user_id = ? AND item_id = ?", userID, itemID)
	}
	if err != nil {
		return false, err
	}

	// Вернуть в сумку ранее надетый предмет (+1).
	if curHeld.Valid {
		_, err = tx.Exec(`
			INSERT INTO user_items (user_id, item_id, quantity) VALUES (?, ?, 1)
			ON CONFLICT(user_id, item_id) DO UPDATE SET quantity = quantity + 1
		`, userID, int(curHeld.Int64))
		if err != nil {
			return false, err
		}
	}

	// Проставить новый надетый.
	if _, err = tx.Exec("UPDATE user_pokemons SET held_item_id = ? WHERE id = ?", itemID, userPokemonID); err != nil {
		return false, err
	}

	return tx.Commit() == nil, nil
}

// UnholdItem снимает надетый предмет с покемона под транзакцией: возвращает его
// в сумку (+1) и обнуляет held_item_id. Если ничего не надето — (false, nil).
func UnholdItem(userID, userPokemonID int) (bool, error) {
	tx, err := DB.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var curHeld sql.NullInt64
	err = tx.QueryRow(
		"SELECT held_item_id FROM user_pokemons WHERE id = ? AND user_id = ?",
		userPokemonID, userID,
	).Scan(&curHeld)
	if err != nil {
		return false, err
	}
	if !curHeld.Valid {
		return false, nil // ничего не надето
	}

	_, err = tx.Exec(`
		INSERT INTO user_items (user_id, item_id, quantity) VALUES (?, ?, 1)
		ON CONFLICT(user_id, item_id) DO UPDATE SET quantity = quantity + 1
	`, userID, int(curHeld.Int64))
	if err != nil {
		return false, err
	}

	if _, err = tx.Exec("UPDATE user_pokemons SET held_item_id = NULL WHERE id = ?", userPokemonID); err != nil {
		return false, err
	}

	return tx.Commit() == nil, nil
}
