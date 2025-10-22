package moex

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type SessionInterval struct {
	Name  string
	Start time.Time
	End   time.Time
}

// GetBoardSessions загружает расписание сессий для указанного борда и даты.
func (c *Client) GetBoardSessions(ctx context.Context, boardID string, date time.Time) ([]SessionInterval, error) {
	endpoint := fmt.Sprintf("engines/stock/markets/shares/boards/%s/sessions.json", boardID)
	values := url.Values{}
	values.Set("date", date.Format("2006-01-02"))

	var response struct {
		Sessions struct {
			Columns []string        `json:"columns"`
			Data    [][]interface{} `json:"data"`
		} `json:"sessions"`
	}

	if err := c.getJSON(ctx, endpoint, values, &response); err != nil {
		return nil, err
	}

	idx := columnIndexInsensitive(response.Sessions.Columns)
	nameIdx, okName := idx["SESSION"]
	if !okName {
		nameIdx = idx["NAME"]
	}
	if nameIdx == 0 && !okName {
		nameIdx = -1
	}
	startIdx, okStart := idx["BEGIN"]
	if !okStart {
		startIdx = idx["START"]
	}
	endIdx, okEnd := idx["END"]
	if !okEnd {
		endIdx = idx["FINISH"]
	}

	intervals := make([]SessionInterval, 0, len(response.Sessions.Data))
	for _, row := range response.Sessions.Data {
		var name string
		if nameIdx >= 0 {
			name = stringFromRow(row, nameIdx)
		}
		beginStr := stringFromRow(row, startIdx)
		endStr := stringFromRow(row, endIdx)
		if beginStr == "" || endStr == "" {
			continue
		}
		begin, err := parseScheduleTime(beginStr, date)
		if err != nil {
			return nil, err
		}
		end, err := parseScheduleTime(endStr, date)
		if err != nil {
			return nil, err
		}
		intervals = append(intervals, SessionInterval{
			Name:  strings.ToLower(name),
			Start: begin,
			End:   end,
		})
	}

	return intervals, nil
}

func parseScheduleTime(value string, baseDate time.Time) (time.Time, error) {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		return time.Time{}, err
	}
	if strings.Contains(value, " ") {
		return time.ParseInLocation("2006-01-02 15:04:05", value, loc)
	}
	t, err := time.ParseInLocation("15:04:05", value, loc)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(baseDate.Year(), baseDate.Month(), baseDate.Day(), t.Hour(), t.Minute(), t.Second(), 0, loc), nil
}
