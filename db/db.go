package db

import (
	"database/sql"
	_ "github.com/lib/pq"
	"errors"
	"fmt"
)

type AppDb struct {
	Conn *sql.DB
}

type User struct {
	Id int
	InstituteAbr string
	GroupId int
	GroupName string
	Week int
}

var (
	ErrNoUser = errors.New("Пользователь не найден")
	ErrNoGroupId = errors.New("Группа пользователя не найдена")
	ErrDbNotInit = errors.New("БД не инициализирована")
)

func PostgresConnStr(user, password, host, port, name, params string) string {
	return fmt.Sprintf(
		"postgresql://%s:%s@%s:%s/%s?%s",
		user, password, host, port, name, params,
	)
}

func (u* User) scan(row *sql.Row) error {
	return row.Scan(&u.Id, &u.InstituteAbr, &u.GroupId, &u.GroupName)
}

func InitAppDb(name, connStr string) (db AppDb, err error) {
	db.Conn, err = sql.Open(name, connStr)
	if _, err = db.Conn.Query("select * from TgUsers limit 1"); err != nil {
		err = ErrDbNotInit
		return
	}
  return
}

func (db* AppDb) CreateUser(id int) (err error) {
	_, err = db.Conn.Exec("insert into TgUsers (Id) values ($1)", id)
	return
}

func (db* AppDb) GetUserById(id int) (user User, err error) {
	row := db.Conn.QueryRow("select Id, InstituteAbr, GroupId, GroupName from TgUsers where id = $1", id)
	err = user.scan(row)
	if err != nil {
		err = ErrNoUser
	}
	return
}

func (db* AppDb) SetUserInstitute(id int, abr string) (err error) {
	_, err = db.Conn.Exec("update TgUsers set InstituteAbr = $1 where id = $2", abr, id)
	return
}

func (db* AppDb) SetUserGroup(id int, group int, name string) (err error) {
	_, err = db.Conn.Exec("update TgUsers set GroupId = $1, GroupName = $2 where id = $3", group, name, id)
	return
}

func (db *AppDb) SetUserWeek(id int, week int) (err error) {
	_, err = db.Conn.Exec("update TgUsers set Week = $1 where id = $2", week, id)
	return
}
