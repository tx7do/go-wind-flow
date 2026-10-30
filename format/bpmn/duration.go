package bpmn

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// isoDurationRe 匹配 ISO 8601 时长的 P[n]D T[n]H[n]M[n]S 子集。
// 年/月/周（Y/M/W）为日历相关时长、语义不固定，不进正则——显式拒绝（fail-closed）。
var isoDurationRe = regexp.MustCompile(
	`^P(?:(\d+(?:\.\d+)?)D)?(?:T(?:(\d+(?:\.\d+)?)H)?(?:(\d+(?:\.\d+)?)M)?(?:(\d+(?:\.\d+)?)S)?)?$`)

// ParseISODuration 解析 ISO 8601 时长（BPMN timeDuration 的标准形态，如
// "PT24H"、"P1D"、"PT30M"、"PT1.5S"）。
//
// 支持天/时/分/秒（含小数）；年/月/周日历相关时长显式拒绝；零与负时长非法。
func ParseISODuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("invalid ISO8601 duration: empty")
	}

	// 宽容大写；正则只认大写。
	m := isoDurationRe.FindStringSubmatch(strings.ToUpper(s))
	if m == nil {
		return 0, fmt.Errorf("invalid ISO8601 duration %q: supported form is P[n]D T[n]H[n]M[n]S (Y/M/W calendar units not supported)", s)
	}
	if m[1] == "" && m[2] == "" && m[3] == "" && m[4] == "" {
		return 0, fmt.Errorf("invalid ISO8601 duration %q: zero duration", s)
	}

	num := func(g string) (float64, error) {
		if g == "" {
			return 0, nil
		}
		return strconv.ParseFloat(g, 64)
	}
	days, err := num(m[1])
	if err != nil {
		return 0, fmt.Errorf("invalid ISO8601 duration %q: bad days: %v", s, err)
	}
	hours, err := num(m[2])
	if err != nil {
		return 0, fmt.Errorf("invalid ISO8601 duration %q: bad hours: %v", s, err)
	}
	mins, err := num(m[3])
	if err != nil {
		return 0, fmt.Errorf("invalid ISO8601 duration %q: bad minutes: %v", s, err)
	}
	secs, err := num(m[4])
	if err != nil {
		return 0, fmt.Errorf("invalid ISO8601 duration %q: bad seconds: %v", s, err)
	}

	total := time.Duration(days*24)*time.Hour +
		time.Duration(hours)*time.Hour +
		time.Duration(mins)*time.Minute +
		time.Duration(secs*float64(time.Second))
	if total <= 0 {
		return 0, fmt.Errorf("invalid ISO8601 duration %q: must be positive", s)
	}
	return total, nil
}

// WriteISODuration 反向编码为 ISO 8601（PTnHnMnS 形式，小时可超过 24）。
func WriteISODuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	h := int64(d / time.Hour)
	m := int64((d % time.Hour) / time.Minute)
	sec := (d % time.Minute).Seconds()
	var b strings.Builder
	b.WriteByte('P')
	b.WriteByte('T')
	if h > 0 {
		b.WriteString(strconv.FormatInt(h, 10))
		b.WriteByte('H')
	}
	if m > 0 || (h == 0 && sec == 0) {
		b.WriteString(strconv.FormatInt(m, 10))
		b.WriteByte('M')
	}
	if sec > 0 || (h == 0 && m == 0) {
		b.WriteString(strconv.FormatFloat(sec, 'f', -1, 64))
		b.WriteByte('S')
	}
	return b.String()
}
