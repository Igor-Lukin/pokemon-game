package user

type User struct {
	ID          int    `json:"id"`
	Username    string `json:"username"`
	Email       string `json:"email"`
	Location_id int    `json:"location_id"`
}
