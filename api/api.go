package api

import (
	"time"
	"fmt"
	"math"
	"strconv"
	"slices"
	"net/http"
	"encoding/json"
	"github.com/sergeykochiev/ivgpu-schedule/common"
	"github.com/sergeykochiev/ivgpu-schedule/tg"
)

type GrouplistGroup struct {
	Id int `json:"id"`
	Title string `json:"title"`
	AlertsGroup []string `json:"alerts_group"`
	EduForm int `json:"eduForm"`
	MailruCalendar string `json:"mailru_calendar"`
}

type GrouplistGroupList []GrouplistGroup

type GrouplistInstitute struct {
	Abbreviate string `json:"abr"`
	Title string `json:"title"`
	Alerts []string `json:"alerts"`
	Groups GrouplistGroupList `json:"groups"`
}

type GrouplistResponse []GrouplistInstitute

// type LessonTimes struct {
// 	All string `json:"-1"`
// 	First string `json:"0"`
// 	Second string `json:"1"`
// 	Third string `json:"2"`
// 	Forth string `json:"3"`
// 	Fifth string `json:"4"`
// 	Sixth string `json:"5"`
// 	Seventh string `json:"6"`
// }

type LessonTimes map[string]string

type Lecturer struct {
	Id int `json:"id"`
	FIO string `json:"FIO"`
	FirstName string `json:"first_name"`
	SecondName string `json:"second_name"`
	MiddleName string `json:"middle_name"`
}

type LessonOnPeriod struct {
	LessonTitle string `json:"lesson_title"`
	Lecturers []Lecturer `json:"lecturers"`
	Room []string `json:"room"`
	Extra struct {} `json:"extra"`
	Remote bool `json:"remote"`
	WeekDay int `json:"week_day"`
	LessonTime int  `json:"lesson_time"`
	SubGroup int `json:"sub_group"`
	Week int `json:"week"`
	Dates []string `json:"dates"`
	Form string `json:"form"`
	AlterSubGroup int `json:"alter_sub_group"`
}

type LessonsOnPeriod []LessonOnPeriod

type GroupSchedule struct {
	EduForm string `json:"eduForm"`
	StartDate string `json:"startDate"`
	EndDate string `json:"endDate"`
	Session bool `json:"session"`
	WeekStart int `json:"week_start"`
	LastModify string `json:"last_modify"`
	Title string `json:"title"`
	LessonsOnPeriod LessonsOnPeriod `json:"lessons_on_period"`
}

type GroupResponse struct {
	Mode string `json:"mode"`
	LessonTimes LessonTimes `json:"lesson_times"`
	LessonShortTimes LessonTimes `json:"lesson_short_times"`
	RemoteDesc struct {
		RemoteLink string `json:"remote_link"`
		RemoteAbr string `json:"remote_abr"`
	} `json:"remote_descr"`
	Schedule []GroupSchedule `json:"rasp"`
}

const (
 	LessonTimeAll int = iota - 1
 	LessonTimeFirst
 	LessonTimeSecond
 	LessonTimeThird
 	LessonTimeForth
 	LessonTimeFifth
 	LessonTimeSixth
 	LessonTimeSeventh
)

const DateLayout = "2006-01-02"

const (
	EndpointGrouplist = "https://raspisanie.ivgpu.ru/api/grouplist"
	Endpoint = "https://raspisanie.ivgpu.ru/api/rasp/?group_id="
)

func totime(date string) (t time.Time) {
	t, _ = time.Parse(DateLayout, date)
	return
}

func fromtime(t time.Time) string {
	return t.Format(DateLayout);
}

func timeBetween(t time.Time, start time.Time, end time.Time) bool {
	return t.Compare(start) >= 0 && t.Compare(end) < 0
}

func (gr GroupResponse) Exams() string {
	var schedule GroupSchedule
	for _, schedule = range(gr.Schedule) {
		// TODO factor out into const
		if schedule.EduForm == "exam_och" {
			break
		}
	}
	return fmt.Sprintf(
		"Расписание экзаменов и консультаций\n\n%s",
		schedule.LessonsOnPeriod.Readable(gr.LessonTimes, true),
	)
}

func (gr GroupResponse) ByDate(t time.Time, userWeek int) string {
	var schedule GroupSchedule
	var start time.Time
	var end time.Time
	for _, schedule = range(gr.Schedule) {
		start = totime(schedule.StartDate)
		end = totime(schedule.EndDate)
		if timeBetween(t, start, end) {
			break
		}
	}
	weekDay := common.WeekdayToISO(t.Weekday())
	week := userWeek
	if userWeek == 0 {
		week = schedule.WeekStart
		if t.Compare(start.AddDate(0, 0, 7)) > 0 {
			if week == 1 {
				week = 2
			} else {
				week = 1
			}
		}
	}
	var lessons LessonsOnPeriod
	for _, lesson := range(schedule.LessonsOnPeriod) {
		if lesson.WeekDay != weekDay || lesson.Week != week {
			continue
		}
		if len(lesson.Dates) > 0 && !slices.Contains(lesson.Dates, fromtime(t)) {
			continue
		}
		lessons = append(lessons, lesson)
	}
	return fmt.Sprintf(
		"Расписание на %d.%d, %s\n\n%s",
		t.Day(),
		t.Month(),
		common.WeekdayNames[weekDay],
		lessons.Readable(gr.LessonTimes, false),
	)
}

func (l LessonOnPeriod) Readable(lt LessonTimes, withDate bool) (s string) {
	if withDate {
		t := totime(l.Dates[0])
		s += fmt.Sprintf("%d.%d, %s\n", t.Day(), t.Month(), common.WeekdayNames[l.WeekDay])
	}
	s += fmt.Sprintf(
		"[%s] \"%s\"\n",
		l.Form,
		l.LessonTitle,
	)
	for i, lr := range(l.Lecturers) {
		s += fmt.Sprintf(
			"%s %s %s\n",
			lr.SecondName,
			lr.FirstName,
			lr.MiddleName,
		)
		if i != len(l.Lecturers) - 1 {
			s += ", "
		}
	}
	s += lt[fmt.Sprintf("%d", l.LessonTime)]
	s += fmt.Sprintf(", %s\n\n", l.Room[0])
	return
}

func (ll LessonsOnPeriod) Readable(lt LessonTimes, withDate bool) (s string) {
	if len(ll) == 0 {
		return "Пар нет"
	}
	for _, l := range(ll) {
		s += l.Readable(lt, withDate)
	}
	return
}

func (gr GrouplistResponse) InlineButtons() (buttons tg.InlineKeyboardMarkup) {
	buttons.InlineKeyboard = make(
		[][]tg.InlineKeyboardButton,
		int(math.Ceil(float64(len(gr)) / 2)),
	)
	for i, g := range(gr) {
		idx := i / 2
		buttons.InlineKeyboard[idx] = append(
			buttons.InlineKeyboard[idx],
			tg.InlineKeyboardButton{
				Text: g.Abbreviate,
				CallbackData: common.CallbackData{
					Typ: common.CallbackQueryTypeInstitute,
					Data: g.Abbreviate,
				}.ToJson(),
			},
		)
	}
	return
}

func (gl GrouplistGroupList) InlineButtons(messageId int) (buttons tg.InlineKeyboardMarkup) {
	fmt.Println(len(gl))
	buttons.InlineKeyboard = make(
		[][]tg.InlineKeyboardButton,
		int(math.Ceil(float64(len(gl)) / 3)),
	)
	for i, g := range(gl) {
		idx := i / 3
		buttons.InlineKeyboard[idx] = append(
			buttons.InlineKeyboard[idx],
			tg.InlineKeyboardButton{
				Text: g.Title,
				CallbackData: common.CallbackData{
					MessageId: messageId,
					Typ: common.CallbackQueryTypeGroups,
					Data: strconv.Itoa(g.Id),
				}.ToJson(),
			},
		)
	}
	buttons.InlineKeyboard = append(buttons.InlineKeyboard, []tg.InlineKeyboardButton{
		{
			Text: "Назад",
			CallbackData: common.CallbackData{ Typ: common.CallbackQueryTypeChangeInstitute, MessageId: messageId }.ToJson(),
		},
	})
	return
}

func simpleGet[T any](url string) (output T, err error) {
	res, err := common.Req(http.MethodGet, url, nil);
	if err != nil {
		return
	}
	err = json.NewDecoder(res.Body).Decode(&output)
	return
}

func GetGrouplist() (GrouplistResponse, error) {
	return simpleGet[GrouplistResponse](EndpointGrouplist)
}

func GetGroup(id int) (GroupResponse, error) {
	return simpleGet[GroupResponse](common.Concat(Endpoint, id))
}

