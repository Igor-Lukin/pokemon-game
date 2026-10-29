package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"pokemon-game/server/db"
	"pokemon-game/server/utils"
)

// give_pokemon.go — выдача экземпляра покемона через единственную точку
// создания utils.CreateUserPokemon. Запускать из корня репозитория:
//
//   go run ./.opencode/skills/give-pokemon/scripts/give_pokemon.go \
//     -user ADMIN -pokemon 681 -level 50 -nature Непреклонный \
//     -iv max -ability "Королевское построение" -placement nursery
//
// IV передаются в порядке: HP,Atk,Def,Spd,SpA,SpD. "max" = все 31.
// Свободные EV всегда = (level-1)*3 + 100 (распределяются игроком позже).

func main() {
	userFlag := flag.String("user", "", "username или id игрока")
	speciesFlag := flag.String("pokemon", "", "имя или id вида")
	levelFlag := flag.Int("level", 0, "уровень 1..100")
	natureFlag := flag.String("nature", "", "характер (русское имя) или id")
	ivFlag := flag.String("iv", "", "max | число | HP,Atk,Def,Spd,SpA,SpD")
	abilityFlag := flag.String("ability", "", "способность: имя, id, slot:N или default")
	placementFlag := flag.String("placement", "nursery", "nursery (питомник) или team (команда)")
	genderFlag := flag.String("gender", "random", "random | M | F | N")
	happinessFlag := flag.Int("happiness", 0, "счастье (0..255)")
	flag.Parse()

	if *userFlag == "" || *speciesFlag == "" || *levelFlag == 0 || *natureFlag == "" || *ivFlag == "" {
		fail("нужны -user, -pokemon, -level, -nature и -iv")
	}
	if *levelFlag < 1 || *levelFlag > 100 {
		fail("уровень должен быть 1..100")
	}
	if *happinessFlag < 0 || *happinessFlag > 255 {
		fail("счастье должно быть 0..255")
	}
	placement := strings.ToLower(strings.TrimSpace(*placementFlag))
	if placement != "nursery" && placement != "team" {
		fail("-placement должен быть nursery или team")
	}

	backupPath := backupDB()
	db.InitDB()
	defer db.CloseDB()

	userID, username, err := resolveUser(*userFlag)
	if err != nil {
		fail("%v", err)
	}
	speciesID, speciesName, err := resolveSpecies(*speciesFlag)
	if err != nil {
		fail("%v", err)
	}
	natureID, natureName, err := resolveNature(*natureFlag)
	if err != nil {
		fail("%v", err)
	}
	iv, err := parseIV(*ivFlag)
	if err != nil {
		fail("IV: %v", err)
	}
	ability, err := resolveAbility(speciesID, *abilityFlag)
	if err != nil {
		fail("способность: %v", err)
	}
	gender, err := resolveGender(speciesID, *genderFlag)
	if err != nil {
		fail("пол: %v", err)
	}

	forceNursery := placement == "nursery"
	if !forceNursery {
		var used int
		if err := db.DB.QueryRow(
			`SELECT COUNT(*) FROM user_pokemons WHERE user_id = ? AND position BETWEEN 1 AND 6`,
			userID).Scan(&used); err != nil {
			fail("проверка команды: %v", err)
		}
		if used >= 6 {
			fail("у игрока %s уже 6 покемонов в команде — выбери питомник или освободи слот", username)
		}
	}

	id, err := utils.CreateUserPokemon(userID, speciesID, *levelFlag, iv, natureID, gender,
		0, *happinessFlag, forceNursery)
	if err != nil {
		fail("CreateUserPokemon: %v", err)
	}

	freeEV := (*levelFlag-1)*3 + 100
	if _, err := db.DB.Exec(`UPDATE user_pokemons
		SET available_ev = ?, ev_hp = 0, ev_attack = 0, ev_defense = 0,
		    ev_speed = 0, ev_special_attack = 0, ev_special_defense = 0
		WHERE id = ?`, freeEV, id); err != nil {
		fail("запись свободных EV: %v", err)
	}
	if ability != nil {
		if _, err := db.DB.Exec(
			`UPDATE user_pokemons SET ability_id = ?, ability_slot = ? WHERE id = ?`,
			ability.ID, ability.Slot, id); err != nil {
			fail("запись способности: %v", err)
		}
	}

	printResult(id, userID, username, speciesID, speciesName, natureName, ability,
		placement, freeEV, backupPath)
}

type abilityChoice struct {
	ID   int
	Slot int
	Name string
}

func resolveUser(value string) (int, string, error) {
	if id, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
		var username string
		err := db.DB.QueryRow(`SELECT username FROM users WHERE id = ?`, id).Scan(&username)
		if err != nil {
			return 0, "", fmt.Errorf("игрок id=%d не найден", id)
		}
		return id, username, nil
	}
	var id int
	var username string
	if err := db.DB.QueryRow(
		`SELECT id, username FROM users WHERE username = ? COLLATE NOCASE`, strings.TrimSpace(value),
	).Scan(&id, &username); err != nil {
		return 0, "", fmt.Errorf("игрок %q не найден", value)
	}
	return id, username, nil
}

func resolveSpecies(value string) (int, string, error) {
	value = strings.TrimSpace(value)
	if id, err := strconv.Atoi(value); err == nil {
		var name string
		if err := db.DB.QueryRow(`SELECT name FROM pokemons WHERE id = ?`, id).Scan(&name); err != nil {
			return 0, "", fmt.Errorf("покемон #%d не найден", id)
		}
		return id, name, nil
	}
	rows, err := db.DB.Query(`SELECT id, name FROM pokemons ORDER BY id`)
	if err != nil {
		return 0, "", err
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var name string
		if rows.Scan(&id, &name) == nil && strings.EqualFold(name, value) {
			return id, name, nil
		}
	}
	return 0, "", fmt.Errorf("покемон %q не найден", value)
}

func resolveNature(value string) (int, string, error) {
	value = strings.TrimSpace(value)
	if id, err := strconv.Atoi(value); err == nil {
		var name string
		if err := db.DB.QueryRow(`SELECT name FROM natures WHERE id = ?`, id).Scan(&name); err != nil {
			return 0, "", fmt.Errorf("характер id=%d не найден", id)
		}
		return id, name, nil
	}
	rows, err := db.DB.Query(`SELECT id, name FROM natures ORDER BY id`)
	if err != nil {
		return 0, "", err
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var name string
		if rows.Scan(&id, &name) == nil && strings.EqualFold(name, value) {
			return id, name, nil
		}
	}
	return 0, "", fmt.Errorf("характер %q не найден", value)
}

func parseIV(value string) ([6]int, error) {
	var iv [6]int
	text := strings.ToLower(strings.TrimSpace(value))
	if text == "max" || text == "макс" || text == "максимум" {
		return [6]int{31, 31, 31, 31, 31, 31}, nil
	}
	parts := strings.FieldsFunc(text, func(r rune) bool {
		return r == ',' || r == '/' || r == ';' || r == ' ' || r == '\t'
	})
	if len(parts) == 0 {
		return iv, fmt.Errorf("пустое значение")
	}
	if len(parts) == 1 {
		n, err := strconv.Atoi(parts[0])
		if err != nil {
			return iv, fmt.Errorf("%q не число", parts[0])
		}
		if n < 0 || n > 31 {
			return iv, fmt.Errorf("значение %d вне 0..31", n)
		}
		for i := range iv {
			iv[i] = n
		}
		return iv, nil
	}
	if len(parts) != 6 {
		return iv, fmt.Errorf("нужно 6 значений (HP,Atk,Def,Spd,SpA,SpD) или max")
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil {
			return iv, fmt.Errorf("%q не число", part)
		}
		if n < 0 || n > 31 {
			return iv, fmt.Errorf("значение %d вне 0..31", n)
		}
		iv[i] = n
	}
	return iv, nil
}

func resolveAbility(speciesID int, value string) (*abilityChoice, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "default") || strings.EqualFold(value, "авто") {
		return nil, nil
	}

	type row struct {
		ID   int
		Name string
		Slot int
	}
	rows, err := db.DB.Query(`
		SELECT a.id, a.name,
		       COALESCE(pa.slot, CASE WHEN pa.is_hidden = 1 THEN 3 ELSE 1 END)
		FROM pokemon_abilities pa
		JOIN abilities a ON a.id = pa.ability_id
		WHERE pa.pokemon_id = ?
		ORDER BY 3, a.id`, speciesID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []row
	for rows.Next() {
		var r row
		if rows.Scan(&r.ID, &r.Name, &r.Slot) == nil {
			list = append(list, r)
		}
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("у вида нет способностей в БД")
	}

	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "slot:") {
		slot, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(lower, "slot:")))
		if err != nil {
			return nil, fmt.Errorf("неверный slot:N")
		}
		for _, r := range list {
			if r.Slot == slot {
				return &abilityChoice{ID: r.ID, Slot: r.Slot, Name: r.Name}, nil
			}
		}
		return nil, fmt.Errorf("у вида нет способности в слоте %d", slot)
	}
	if id, err := strconv.Atoi(value); err == nil {
		for _, r := range list {
			if r.ID == id {
				return &abilityChoice{ID: r.ID, Slot: r.Slot, Name: r.Name}, nil
			}
		}
		return nil, fmt.Errorf("способность id=%d не принадлежит виду", id)
	}
	for _, r := range list {
		if strings.EqualFold(r.Name, value) {
			return &abilityChoice{ID: r.ID, Slot: r.Slot, Name: r.Name}, nil
		}
	}
	var names []string
	for _, r := range list {
		names = append(names, fmt.Sprintf("%s (id=%d, slot=%d)", r.Name, r.ID, r.Slot))
	}
	return nil, fmt.Errorf("%q не найдена; доступны: %s", value, strings.Join(names, "; "))
}

func resolveGender(speciesID int, value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" || value == "RANDOM" || value == "СЛУЧАЙНО" {
		return db.RollGender(speciesID)
	}
	if value != "M" && value != "F" && value != "N" {
		return "", fmt.Errorf("нужно random, M, F или N")
	}
	var male, female int
	if err := db.DB.QueryRow(
		`SELECT COALESCE(male_percentage,0), COALESCE(female_percentage,0)
		 FROM pokemons WHERE id = ?`, speciesID).Scan(&male, &female); err != nil {
		return "", err
	}
	if male == 0 && female == 0 {
		if value != "N" {
			return "", fmt.Errorf("вид бесполый — допустим только N")
		}
		return value, nil
	}
	if value == "N" {
		return "", fmt.Errorf("вид имеет пол — используй M, F или random")
	}
	return value, nil
}

func backupDB() string {
	src := filepath.Join("database", "pokemon.db")
	if _, err := os.Stat(src); err != nil {
		fail("не найден %s (запускай из корня репозитория)", src)
	}
	dir := filepath.Join("database", "backups")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail("бэкап: %v", err)
	}
	dst := filepath.Join(dir, "pokemon.db.bak-before-give-"+time.Now().Format("20060102-150405"))
	if err := copyFile(src, dst); err != nil {
		fail("бэкап: %v", err)
	}
	return dst
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func printResult(id, userID int, username string, speciesID int, speciesName, natureName string,
	ability *abilityChoice, placement string, freeEV int, backupPath string) {
	var (
		level, position, happiness      int
		abilityName, abilityKey, nature string
		genderDB                        string
		ivs                             string
		availableEV                     int
		hp, atk, def, spd, spAtk, spDef int
	)
	err := db.DB.QueryRow(`
		SELECT up.level, up.position, COALESCE(a.name,''), COALESCE(a.effect_key,''),
		       COALESCE(n.name,''), COALESCE(up.gender,''),
		       printf('%d/%d/%d/%d/%d/%d', up.iv_hp, up.iv_attack, up.iv_defense,
		              up.iv_speed, up.iv_special_attack, up.iv_special_defense),
		       COALESCE(up.available_ev,0), up.happiness,
		       up.max_hp, up.attack, up.defense, up.speed, up.special_attack, up.special_defense
		FROM user_pokemons up
		LEFT JOIN abilities a ON a.id = up.ability_id
		LEFT JOIN natures n ON n.id = up.nature_id
		WHERE up.id = ?`, id).Scan(&level, &position, &abilityName, &abilityKey,
		&nature, &genderDB, &ivs, &availableEV, &happiness,
		&hp, &atk, &def, &spd, &spAtk, &spDef)
	if err != nil {
		fail("проверка после выдачи: %v", err)
	}
	if availableEV != freeEV {
		fail("свободные EV не совпали: в БД %d, ожидалось %d", availableEV, freeEV)
	}
	if nature == "" {
		nature = natureName
	}
	placementText := "питомник"
	if placement == "team" {
		placementText = fmt.Sprintf("команда, слот %d", position)
	}
	fmt.Printf("OK user_pokemons.id=%d\n", id)
	fmt.Printf("Игрок: %s (id=%d)\n", username, userID)
	fmt.Printf("Покемон: %s (#%d) Lv.%d, %s\n", speciesName, speciesID, level, placementText)
	fmt.Printf("Характер: %s | Пол: %s | Счастье: %d\n", nature, genderDB, happiness)
	fmt.Printf("IV: %s | Свободные EV: %d\n", ivs, availableEV)
	if abilityName != "" {
		fmt.Printf("Способность: %s (%s)\n", abilityName, abilityKey)
	}
	if ability != nil {
		fmt.Printf("Способность задана вручную: %s (id=%d, slot=%d)\n", ability.Name, ability.ID, ability.Slot)
	}
	fmt.Printf("Статы: HP %d / Atk %d / Def %d / Spd %d / SpA %d / SpD %d\n",
		hp, atk, def, spd, spAtk, spDef)
	fmt.Printf("Бэкап БД: %s\n", backupPath)
}

func fail(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "ОШИБКА: "+format+"\n", args...)
	os.Exit(1)
}
