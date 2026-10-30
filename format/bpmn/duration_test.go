package bpmn

import (
	"testing"
	"time"
)

func TestParseISODuration(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{"PT24H", 24 * time.Hour, true},
		{"P1D", 24 * time.Hour, true},
		{"PT30M", 30 * time.Minute, true},
		{"PT1.5S", 1500 * time.Millisecond, true},
		{"P2DT12H", 60 * time.Hour, true},
		{"pt1s", time.Second, true}, // 宽容小写
		{"P1M", 0, false},           // 月：日历相关，拒绝
		{"P1Y", 0, false},           // 年：拒绝
		{"P1W", 0, false},           // 周：拒绝
		{"PT0S", 0, false},          // 零时长
		{"", 0, false},
		{"24H", 0, false},   // 缺 P
		{"PTH", 0, false},   // 缺数值
		{"PT1H30", 0, false} /* 缺 designator */,
	}
	for _, c := range cases {
		got, err := ParseISODuration(c.in)
		if c.ok && (err != nil || got != c.want) {
			t.Errorf("ParseISODuration(%q) = %v, %v; want %v", c.in, got, err, c.want)
		}
		if !c.ok && err == nil {
			t.Errorf("ParseISODuration(%q) should fail", c.in)
		}
	}
}

func TestWriteISODurationRoundTrip(t *testing.T) {
	for _, d := range []time.Duration{
		time.Second, 90 * time.Second, time.Hour, 25 * time.Hour,
		90 * time.Minute, 36*time.Hour + 15*time.Minute + 10*time.Second,
	} {
		s := WriteISODuration(d)
		back, err := ParseISODuration(s)
		if err != nil || back != d {
			t.Errorf("round-trip %v → %q → %v, %v", d, s, back, err)
		}
	}
}
