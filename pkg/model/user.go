package model

import (
	"encoding/json"
	"time"
)

type User struct {
	ID        int        `json:"id" gorm:"autoIncrement;primaryKey"`
	Name      string     `json:"name" gorm:"type:varchar(50);uniqueIndex:name_auth_type;not null"`
	Password  string     `json:"-" gorm:"type:varchar(256);"`
	Email     string     `json:"email" gorm:"type:varchar(256);"`
	AuthType  string     `json:"authType" gorm:"type:varchar(256);uniqueIndex:name_auth_type;default:nil"`
	AuthId    string     `json:"authId" gorm:"type:varchar(256);"`
	Avatar    string     `json:"avatar" gorm:"type:varchar(256);"`
	CreatedAt time.Time  `json:"create_time"`
	UpdatedAt time.Time  `json:"update_time"`
	DeletedAt *time.Time `json:"-"` // soft delete
}

func (*User) TableName() string {
	return "users"
}

func (u *User) CacheKey() string {
	return u.TableName() + ":id"
}

func (u *User) MarshalBinary() ([]byte, error) {
	return json.Marshal(u)
}

func (u *User) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, u)
}

type CreatedUser struct {
	Name     string `json:"name"`
	Password string `json:"password"`
	Email    string `json:"email"`
	AuthType string `json:"authType"`
	AuthId   string `json:"authId"`
	Avatar   string `json:"avatar"`
}

func (u *CreatedUser) GetUser() *User {
	return &User{
		Name:     u.Name,
		Password: u.Password,
		Email:    u.Email,
		AuthType: u.AuthType,
		AuthId:   u.AuthId,
		Avatar:   u.Avatar,
	}
}

type UpdatedUser struct {
	Name     string `json:"name"`
	Password string `json:"password"`
	Email    string `json:"email"`
}

func (u *UpdatedUser) GetUser() *User {
	return &User{
		Name:     u.Name,
		Password: u.Password,
		Email:    u.Email,
	}
}

type AuthUser struct {
	Name      string `json:"name"`
	Password  string `json:"password"`
	SetCookie bool   `json:"setCookie"`
	AuthType  string `json:"authType"`
	AuthCode  string `json:"authCode"`
}

type Users []User

type UserService interface {
	List() (Users, error)
	Create(*User) (*User, error)
	Get(string) (*User, error)
	CreateOAuthUser(user *User) (*User, error)
	Update(string, *User) (*User, error)
	Delete(string) error
	Validate(*User) error
	Auth(*AuthUser) (*User, error)
	Default(*User)
}

type UserRepository interface {
	GetUserByID(int) (*User, error)
	GetUserByAuthID(authType, authID string) (*User, error)
	GetUserByName(string) (*User, error)
	List() (Users, error)
	Create(user *User) (*User, error)
	Update(user *User) (*User, error)
	Delete(user *User) error
	Migrate() error
}
