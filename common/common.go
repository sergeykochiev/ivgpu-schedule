package common

import (
	"time"
	"fmt"
	"net/http"
	"bytes"
	"errors"
	"encoding/json"
)


type CallbackData struct {
	MessageId int `json:"m"`
	Typ string `json:"t"`
	Data string `json:"d"`
}

var (
	ErrNoUser = errors.New("User not found: ")
	ErrNoGroupId = errors.New("User's group not found: ")
	ErrDbNotInit = errors.New("DB hasn't initialized yet: ")
	ErrNotOk = errors.New("Request status is not OK: ")
	ErrConnectDb = errors.New("Failed to connect to DB: ")
	ErrGetGroupList = errors.New("Failed to get group list: ")
	ErrCreateUser = errors.New("Failed to create new user: ")
	ErrGetUserById = errors.New("Failed to get user by ID: ")
	ErrSetGroup = errors.New("Failed to set user group: ")
	ErrEditMsg = errors.New("Failed to edit original message: ")
	ErrSetUserInst = errors.New("Failed to set user institute: ")
	ErrInitGroupChoice = errors.New("Failed to init group choice: ")
	ErrAcceptInstChoice = errors.New("Failed to accept institute choice: ")
	ErrAcceptGroupChoice = errors.New("Failed to accept group choice: ")
	ErrAcceptWeekChoice = errors.New("Failed to accept week choice: ")
	ErrInitWeekChoice = errors.New("Failed to init week choice: ")
	ErrInitInstChoice = errors.New("Failed to init institute choice: ")
	ErrGetSchedule = errors.New("Failed to get schedule: ")
	ErrWeekFromData = errors.New("Failed to parse week from query data: ")
	ErrSetWeek = errors.New("Failed to update user week: ")
	ErrCommand = errors.New("Failed to handle command: ")
	ErrGetExams = errors.New("Failed to get exams: ")
	ErrGetToday = errors.New("Failed to get today's schedule: ")
	ErrGetTomorrow = errors.New("Failed to get tomorrow's schedule: ")
	ErrGetMon = errors.New("Failed to get Monday's schedule: ")
	ErrGetTue = errors.New("Failed to get Tuesday's schedule: ")
	ErrGetWed = errors.New("Failed to get Wednesday's schedule: ")
	ErrGetThu = errors.New("Failed to get Thursday's schedule: ")
	ErrGetFri = errors.New("Failed to get Friday's schedule: ")
	ErrGetSat = errors.New("Failed to get Saturday's schedule: ")
	ErrGetUpdates = errors.New("Failed to get updates: ")
	ErrInit = errors.New("Failed to init main app: ")
	ErrHandleMessage = errors.New("Failed to handle message: ")
	ErrHandleQuery = errors.New("Failed to handle : ")
)

const (
	CallbackQueryTypeInstitute = "insttt"
	CallbackQueryTypeChangeInstitute = "cngint"
	CallbackQueryTypeWeek = "cngwek"
	CallbackQueryTypeGroups = "groups"
)

const (
	ReplyKeyboardButtonToday = "На сегодня"
	ReplyKeyboardButtonTomorrow = "На завтра"
	ReplyKeyboardButtonMonday = "Пн"
	ReplyKeyboardButtonTuesday = "Вт"
	ReplyKeyboardButtonWednesday = "Ср"
	ReplyKeyboardButtonThursday = "Чт"
	ReplyKeyboardButtonFriday = "Пт"
	ReplyKeyboardButtonSaturday = "Сб"
	ReplyKeyboardButtonChangeGroup = "Сменить группу"
	ReplyKeyboardButtonChangeWeek = "Сменить неделю"
	ReplyKeyboardButtonExams = "Все экзамены"
)

var WeekdayNames = [7]string{
	"Понедельник",
	"Вторник",
	"Среда",
	"Четверг",
	"Пятница",
	"Суббота",
	"Воскресение",
}
var Weeknames = [3]string{"Текущая", "Первая", "Вторая"}

func (cd CallbackData) ToJson() string {
	out, _ := json.Marshal(cd)
	return string(out)
}

func WeekdayToISO(weekday time.Weekday) int {
	if (weekday == 0) {
		return 6;
	}
	return int(weekday - 1);
}

func ParseCallbackData(data string) (query CallbackData) { 
	json.Unmarshal([]byte(data), &query)
	return
}

func Req(method string, url string, body []byte) (*http.Response, error) {
	req, err := http.NewRequest(method, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Add("content-type", "application/json")
	client := http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return res, err
	}
	if res.StatusCode != 200 {
		return res, errors.Join(
			ErrNotOk,
			fmt.Errorf("%s: %s %s", method, url, res.Status),
			errors.New(string(body)),
		)
	}
	return res, nil
}

func JsonReq[ResT any, ReqT any](url string, body ReqT) (jsonRes ResT, err error) {
	bytes, err := json.Marshal(body)
	if err != nil {
		return
	}
	res, err := Req(http.MethodPost, url, bytes)
	json.NewDecoder(res.Body).Decode(&jsonRes)
	return
}

func Concat(s string, i int) string {
	return fmt.Sprintf("%s%d", s, i)
}

func StartsWith(s string, frg string) bool {
	return len(s) >= len(frg) && s[:len(frg)] == frg
}

func StatefulButton(name string, state string) string {
	return fmt.Sprintf("%s (сейчас - %s)", name, state)
}
