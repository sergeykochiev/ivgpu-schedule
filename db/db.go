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
	ErrMigrateFailed = errors.New("Не удалось популяризировать БД")
)

const CreateQuery = `CREATE TABLE TgUsers ( Id SERIAL PRIMARY KEY, InstituteAbr VARCHAR(50) DEFAULT '', GroupId INT DEFAULT 0, GroupName VARCHAR(50) DEFAULT '', Week INT DEFAULT 0 );`

func (u* User) scan(rows *sql.Rows) error {
	return rows.Scan(&u.Id, &u.InstituteAbr, &u.GroupId, &u.GroupName)
}

func InitAppDb(name, connStr string) (db AppDb, err error) {
	db.Conn, err = sql.Open(name, connStr)
	if _, err = db.Conn.Query("select * from TgUsers limit 1"); err != nil {
		err = db.createTables()
		if err != nil {
			err = errors.Join(ErrMigrateFailed, err)
		}
	}
  return
}

func (db* AppDb) createTables() (err error) {
	_, err = db.Conn.Exec(CreateQuery)
	return
}

func (db* AppDb) CreateUser(id int) (err error) {
	_, err = db.Conn.Exec("insert into TgUsers (Id) values ($1)", id)
	fmt.Println(1)
	return
}

func userFromRows(rows *sql.Rows) (user User, err error) {
	nextRes := rows.Next()
	if !nextRes {
		err = rows.Err()
		if errors.Is(err, sql.ErrNoRows) {
			err = ErrNoUser
		}
		return
	}
	err = user.scan(rows)
	return
}

func (db* AppDb) GetUserById(id int) (user User, err error) {
	result, err := db.Conn.Query("select Id, InstituteAbr, GroupId, GroupName from TgUsers where id = $1", id)
	if err == nil {
		user, err = userFromRows(result)
	}
	defer result.Close()
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
