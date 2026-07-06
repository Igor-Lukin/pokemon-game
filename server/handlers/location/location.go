package location

type Location struct {
	ID          int    `db:"id"`
	Name        string `db:"name"`
	Description string `db:"description"`
}
