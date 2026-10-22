package db

// Хелперы карточки тренера (поток [7]). Всё только на чтение; наполнение друзей/
// желаемых/наград — через БД и /admin/give_item.

// AwardInfo — награда игрока (предмет с items.is_award=1).
type AwardInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ImageURL    string `json:"image_url"`
}

// WishlistPokemon — желаемый покемон (только показ).
type WishlistPokemon struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	ImageURL string `json:"image_url"`
}

// FriendInfo — друг игрока с признаком онлайн (по last_seen).
type FriendInfo struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Online   bool   `json:"online"`
}

// онлайн = last_seen задан и не старше 5 минут (UTC, как CURRENT_TIMESTAMP).
const onlineExpr = "u.last_seen IS NOT NULL AND (strftime('%s','now') - strftime('%s', u.last_seen)) < 300"

// GetUserAwards — награды игрока (предметы-награды из его сумки).
func GetUserAwards(userID int) ([]AwardInfo, error) {
	rows, err := DB.Query(`
		SELECT i.name, COALESCE(i.description, ''), COALESCE(i.image_url, '')
		FROM user_items ui
		JOIN items i ON i.id = ui.item_id
		WHERE ui.user_id = ? AND ui.quantity > 0 AND COALESCE(i.is_award, 0) = 1
		ORDER BY i.id
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	awards := []AwardInfo{}
	for rows.Next() {
		var a AwardInfo
		if err := rows.Scan(&a.Name, &a.Description, &a.ImageURL); err != nil {
			continue
		}
		awards = append(awards, a)
	}
	return awards, nil
}

// GetUserWishlist — список желаемых покемонов игрока.
func GetUserWishlist(userID int) ([]WishlistPokemon, error) {
	rows, err := DB.Query(`
		SELECT p.id, p.name, COALESCE(p.image_url, '')
		FROM user_wishlist w
		JOIN pokemons p ON p.id = w.pokemon_id
		WHERE w.user_id = ?
		ORDER BY p.id
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []WishlistPokemon{}
	for rows.Next() {
		var p WishlistPokemon
		if err := rows.Scan(&p.ID, &p.Name, &p.ImageURL); err != nil {
			continue
		}
		list = append(list, p)
	}
	return list, nil
}

// GetUserFriends — друзья игрока с флагом онлайн.
func GetUserFriends(userID int) ([]FriendInfo, error) {
	rows, err := DB.Query(`
		SELECT u.id, u.username,
		       CASE WHEN `+onlineExpr+` THEN 1 ELSE 0 END AS online
		FROM user_friends f
		JOIN users u ON u.id = f.friend_id
		WHERE f.user_id = ?
		ORDER BY u.username
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	friends := []FriendInfo{}
	for rows.Next() {
		var f FriendInfo
		var online int
		if err := rows.Scan(&f.ID, &f.Username, &online); err != nil {
			continue
		}
		f.Online = online == 1
		friends = append(friends, f)
	}
	return friends, nil
}
