package db

import "log"

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
		       COALESCE(i.image_url, ''), ui.quantity
		FROM user_items ui
		JOIN items i ON ui.item_id = i.id
		LEFT JOIN item_categories ic ON i.category_id = ic.id
		WHERE ui.user_id = ? AND ui.quantity > 0
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
			&ui.ImageURL, &ui.Quantity); err != nil {
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
		       COALESCE(i.image_url, '')
		FROM items i
		LEFT JOIN item_categories ic ON i.category_id = ic.id
		WHERE i.id = ?
	`, itemID).Scan(&it.ID, &it.Name, &it.Description, &it.CategoryID,
		&it.CategoryName, &it.Price, &it.EffectType, &it.EffectValue, &it.ImageURL)
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

func CreateItem(name, description string, categoryID, price int, effectType string, effectValue int, imageURL string) error {
	_, err := DB.Exec(`
		INSERT INTO items (name, description, category_id, price, effect_type, effect_value, image_url)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, name, description, categoryID, price, effectType, effectValue, imageURL)
	return err
}

func UpdateItem(id int, name, description string, categoryID, price int, effectType string, effectValue int, imageURL string) error {
	_, err := DB.Exec(`
		UPDATE items SET name=?, description=?, category_id=?, price=?, effect_type=?, effect_value=?, image_url=?
		WHERE id=?
	`, name, description, categoryID, price, effectType, effectValue, imageURL, id)
	return err
}

func DeleteItem(id int) error {
	_, err := DB.Exec("DELETE FROM items WHERE id = ?", id)
	return err
}
